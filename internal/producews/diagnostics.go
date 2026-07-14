package producews

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cs2-highlight-tool-v2/internal/logging"
)

const (
	diagnosticRingLimit      = 500
	diagnosticQueueLimit     = 1024
	diagnosticLogMaxBytes    = 2 * 1024 * 1024
	diagnosticLogBackupCount = 3
	diagnosticPluginTailSize = 64 * 1024
)

// DiagnosticEvent is the sanitized, bounded event shape retained for a
// production WebSocket session. Dynamic values are kept in Meta so reports can
// be filtered without parsing free-form messages.
type DiagnosticEvent struct {
	Time      string            `json:"time"`
	Level     string            `json:"level"`
	Component string            `json:"component"`
	Stage     string            `json:"stage"`
	Action    string            `json:"action"`
	Message   string            `json:"message"`
	Error     string            `json:"error,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// DiagnosticsSnapshot is safe to export after the service has stopped or when
// the log directory cannot be written.
type DiagnosticsSnapshot struct {
	Events        []DiagnosticEvent `json:"events"`
	DroppedEvents uint64            `json:"dropped_events"`
	WriteFailures uint64            `json:"write_failures"`
}

// Diagnostics writes the durable diagnostic trail asynchronously. It is
// deliberately independent from Service.mu: Record only updates its own ring
// and makes a non-blocking enqueue; the worker owns all filesystem I/O.
type Diagnostics struct {
	logDir        string
	logPath       string
	pluginLogPath string

	mu            sync.Mutex
	events        []DiagnosticEvent
	droppedEvents uint64
	writeFailures uint64
	reportSeq     uint64
	queue         chan DiagnosticEvent
	stop          chan struct{}
	done          chan struct{}
	started       bool
}

// NewDiagnostics creates a stopped diagnostic sink rooted in dataDir/logs.
// Call Start/Stop with the owning Service lifecycle.
func NewDiagnostics(dataDir string) *Diagnostics {
	logDir := filepath.Join(strings.TrimSpace(dataDir), "logs")
	return &Diagnostics{
		logDir:        logDir,
		logPath:       filepath.Join(logDir, "producews.log"),
		pluginLogPath: filepath.Join(logDir, "cs2-server-plugin.log"),
	}
}

func (d *Diagnostics) Start() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return
	}
	d.queue = make(chan DiagnosticEvent, diagnosticQueueLimit)
	d.stop = make(chan struct{})
	d.done = make(chan struct{})
	d.started = true
	queue := d.queue
	stop := d.stop
	done := d.done
	d.mu.Unlock()

	go d.run(queue, stop, done)
}

func (d *Diagnostics) Stop() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if !d.started {
		d.mu.Unlock()
		return
	}
	stop := d.stop
	done := d.done
	d.started = false
	d.stop = nil
	d.done = nil
	d.queue = nil
	d.mu.Unlock()

	close(stop)
	<-done
}

// Record accepts an event without ever waiting for disk I/O. When the writer
// cannot keep up, the in-memory ring remains complete while only file events
// may be dropped; the export includes the drop counter.
func (d *Diagnostics) Record(event DiagnosticEvent) {
	if d == nil {
		return
	}
	event = sanitizeDiagnosticEvent(event)
	d.mu.Lock()
	d.events = append(d.events, event)
	if len(d.events) > diagnosticRingLimit {
		d.events = append([]DiagnosticEvent(nil), d.events[len(d.events)-diagnosticRingLimit:]...)
	}
	queue := d.queue
	started := d.started
	d.mu.Unlock()

	if !started || queue == nil {
		return
	}
	select {
	case queue <- event:
	default:
		d.mu.Lock()
		d.droppedEvents++
		d.mu.Unlock()
	}
}

func (d *Diagnostics) Snapshot() DiagnosticsSnapshot {
	if d == nil {
		return DiagnosticsSnapshot{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	items := make([]DiagnosticEvent, 0, len(d.events))
	for _, event := range d.events {
		items = append(items, cloneDiagnosticEvent(event))
	}
	return DiagnosticsSnapshot{
		Events:        items,
		DroppedEvents: d.droppedEvents,
		WriteFailures: d.writeFailures,
	}
}

// WriteIncident writes one standalone report using temp-file + rename so a
// partial report is never offered by Export. The caller must have captured all
// Service state before invoking this method.
func (d *Diagnostics) WriteIncident(reason string, ws WSState, queue QueueState, takes TakeStatusSnapshot) (string, error) {
	if d == nil {
		return "", nil
	}
	snapshot := d.Snapshot()
	pluginTail := d.readPluginLogTail()
	now := time.Now()

	d.mu.Lock()
	d.reportSeq++
	sequence := d.reportSeq
	d.mu.Unlock()

	name := fmt.Sprintf("producews-incident-%s-%03d.txt", now.Format("20060102-150405.000"), sequence)
	path := filepath.Join(d.logDir, name)
	content := buildIncidentReport(now, reason, ws, queue, takes, snapshot, pluginTail)
	if err := os.MkdirAll(d.logDir, 0o755); err != nil {
		d.noteWriteFailure()
		return "", fmt.Errorf("创建制作日志目录失败: %w", err)
	}
	temporary, err := os.CreateTemp(d.logDir, name+".*.tmp")
	if err != nil {
		d.noteWriteFailure()
		return "", fmt.Errorf("创建制作诊断临时文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		d.noteWriteFailure()
		return "", fmt.Errorf("写入制作诊断报告失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		d.noteWriteFailure()
		return "", fmt.Errorf("关闭制作诊断临时文件失败: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		d.noteWriteFailure()
		return "", fmt.Errorf("发布制作诊断报告失败: %w", err)
	}
	return path, nil
}

// ExportText returns a single sanitized support artifact containing the ring,
// rotating host logs, incident reports, and the deterministic plugin log tail.
func (d *Diagnostics) ExportText(ws WSState, queue QueueState, takes TakeStatusSnapshot) string {
	if d == nil {
		return buildDiagnosticExport(time.Now(), ws, queue, takes, DiagnosticsSnapshot{}, nil, nil, "")
	}
	snapshot := d.Snapshot()
	return buildDiagnosticExport(
		time.Now(),
		ws,
		queue,
		takes,
		snapshot,
		d.readHostLogFiles(),
		d.readIncidentReports(),
		d.readPluginLogTail(),
	)
}

func (d *Diagnostics) run(queue <-chan DiagnosticEvent, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case event := <-queue:
			d.appendEvent(event)
		case <-stop:
			for {
				select {
				case event := <-queue:
					d.appendEvent(event)
				default:
					return
				}
			}
		}
	}
}

func (d *Diagnostics) appendEvent(event DiagnosticEvent) {
	encoded, err := json.Marshal(event)
	if err != nil {
		d.noteWriteFailure()
		return
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(d.logDir, 0o755); err != nil {
		d.noteWriteFailure()
		return
	}
	if err := d.rotateIfNeeded(int64(len(encoded))); err != nil {
		d.noteWriteFailure()
		return
	}
	file, err := os.OpenFile(d.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		d.noteWriteFailure()
		return
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		d.noteWriteFailure()
	}
}

func (d *Diagnostics) rotateIfNeeded(nextSize int64) error {
	info, err := os.Stat(d.logPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && info.Size()+nextSize <= diagnosticLogMaxBytes {
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	for index := diagnosticLogBackupCount; index >= 1; index-- {
		from := fmt.Sprintf("%s.%d", d.logPath, index)
		if index == diagnosticLogBackupCount {
			if removeErr := os.Remove(from); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
			continue
		}
		to := fmt.Sprintf("%s.%d", d.logPath, index+1)
		if renameErr := os.Rename(from, to); renameErr != nil && !os.IsNotExist(renameErr) {
			return renameErr
		}
	}
	return os.Rename(d.logPath, d.logPath+".1")
}

func (d *Diagnostics) noteWriteFailure() {
	d.mu.Lock()
	d.writeFailures++
	d.mu.Unlock()
}

func (d *Diagnostics) readHostLogFiles() []diagnosticFile {
	paths := []string{d.logPath}
	for index := 1; index <= diagnosticLogBackupCount; index++ {
		paths = append(paths, fmt.Sprintf("%s.%d", d.logPath, index))
	}
	return readDiagnosticFiles(paths)
}

func (d *Diagnostics) readIncidentReports() []diagnosticFile {
	paths, err := filepath.Glob(filepath.Join(d.logDir, "producews-incident-*.txt"))
	if err != nil {
		return nil
	}
	sort.Strings(paths)
	return readDiagnosticFiles(paths)
}

func (d *Diagnostics) readPluginLogTail() string {
	return readFileTail(d.pluginLogPath, diagnosticPluginTailSize)
}

type diagnosticFile struct {
	Name    string
	Content string
}

func readDiagnosticFiles(paths []string) []diagnosticFile {
	files := make([]diagnosticFile, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		files = append(files, diagnosticFile{
			Name:    filepath.Base(path),
			Content: logging.SanitizeExportText(string(content)),
		})
	}
	return files
}

func readFileTail(path string, maxBytes int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, 0); err != nil {
		return ""
	}
	content := make([]byte, int(info.Size()-start))
	read, err := file.Read(content)
	if err != nil && read == 0 {
		return ""
	}
	return logging.SanitizeExportText(string(content[:read]))
}

func sanitizeDiagnosticEvent(event DiagnosticEvent) DiagnosticEvent {
	if strings.TrimSpace(event.Time) == "" {
		event.Time = time.Now().Format(time.RFC3339Nano)
	}
	event.Level = strings.ToLower(strings.TrimSpace(event.Level))
	if event.Level == "" {
		event.Level = "info"
	}
	event.Component = strings.TrimSpace(event.Component)
	event.Stage = strings.TrimSpace(event.Stage)
	event.Action = strings.TrimSpace(event.Action)
	event.Message = logging.SanitizeExportText(event.Message)
	event.Error = logging.SanitizeExportText(event.Error)
	if len(event.Meta) > 0 {
		meta := make(map[string]string, len(event.Meta))
		for key, value := range event.Meta {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			meta[key] = logging.SanitizeExportMetaValue(key, value)
		}
		if len(meta) > 0 {
			event.Meta = meta
		} else {
			event.Meta = nil
		}
	}
	return event
}

func cloneDiagnosticEvent(event DiagnosticEvent) DiagnosticEvent {
	clone := event
	if len(event.Meta) > 0 {
		clone.Meta = make(map[string]string, len(event.Meta))
		for key, value := range event.Meta {
			clone.Meta[key] = value
		}
	}
	return clone
}

func buildIncidentReport(now time.Time, reason string, ws WSState, queue QueueState, takes TakeStatusSnapshot, snapshot DiagnosticsSnapshot, pluginTail string) string {
	return buildDiagnosticExport(now, ws, queue, takes, snapshot, nil, nil, pluginTail, "incident: "+reason)
}

func buildDiagnosticExport(
	exportTime time.Time,
	ws WSState,
	queue QueueState,
	takes TakeStatusSnapshot,
	snapshot DiagnosticsSnapshot,
	hostLogs []diagnosticFile,
	incidents []diagnosticFile,
	pluginTail string,
	prefix ...string,
) string {
	queue = sanitizeQueueForExport(queue)
	takes = sanitizeTakeSnapshotForExport(takes)
	var output bytes.Buffer
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&output, format+"\n", args...)
	}
	write("CS2 Highlight Tool - Produce WebSocket Diagnostics")
	write("Exported At: %s", exportTime.Format(time.RFC3339Nano))
	for _, entry := range prefix {
		if strings.TrimSpace(entry) != "" {
			write("%s", logging.SanitizeExportText(entry))
		}
	}
	write("")
	write("[Counters]")
	write("DroppedEvents: %d", snapshot.DroppedEvents)
	write("WriteFailures: %d", snapshot.WriteFailures)
	write("")
	write("[WebSocket State]")
	writeJSON(&output, ws)
	write("[Queue State]")
	writeJSON(&output, queue)
	write("[Take Snapshot]")
	writeJSON(&output, takes)
	write("[In-Memory Timeline]")
	if len(snapshot.Events) == 0 {
		write("(no events)")
	} else {
		for index, event := range snapshot.Events {
			encoded, _ := json.Marshal(event)
			write("%04d %s", index+1, logging.SanitizeExportText(string(encoded)))
		}
	}
	write("[Rotated Host Logs]")
	writeDiagnosticFiles(&output, hostLogs)
	write("[Incident Reports]")
	writeDiagnosticFiles(&output, incidents)
	write("[Plugin Log Tail]")
	if strings.TrimSpace(pluginTail) == "" {
		write("(no plugin log available)")
	} else {
		write("%s", logging.SanitizeExportText(pluginTail))
	}
	return logging.SanitizeExportText(output.String())
}

func sanitizeQueueForExport(queue QueueState) QueueState {
	clone := queue
	paths := append([]string(nil), queue.Demos...)
	if clone.CurrentDemoPath != "" {
		clone.CurrentDemoPath = logging.SanitizeExportDemoPath(clone.CurrentDemoPath)
	}
	if len(paths) > 0 {
		clone.Demos = make([]string, len(paths))
		for index, path := range paths {
			clone.Demos[index] = logging.SanitizeExportDemoPath(path)
			clone.LastError = strings.ReplaceAll(clone.LastError, path, "<redacted-demo>")
		}
	}
	return clone
}

func sanitizeTakeSnapshotForExport(snapshot TakeStatusSnapshot) TakeStatusSnapshot {
	clone := snapshot
	clone.Items = make([]TakeStatus, len(snapshot.Items))
	for index, item := range snapshot.Items {
		clone.Items[index] = item
		if item.DemoPath != "" {
			clone.Items[index].DemoPath = logging.SanitizeExportDemoPath(item.DemoPath)
		}
	}
	if snapshot.LastEvent != nil {
		event := *snapshot.LastEvent
		if event.DemoPath != "" {
			event.DemoPath = logging.SanitizeExportDemoPath(event.DemoPath)
		}
		clone.LastEvent = &event
	}
	return clone
}

func writeJSON(output *bytes.Buffer, value any) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = output.WriteString("(snapshot encode failed)\n")
		return
	}
	_, _ = output.WriteString(logging.SanitizeExportText(string(encoded)))
	_, _ = output.WriteString("\n")
}

func writeDiagnosticFiles(output *bytes.Buffer, files []diagnosticFile) {
	if len(files) == 0 {
		_, _ = output.WriteString("(none)\n")
		return
	}
	for _, file := range files {
		_, _ = fmt.Fprintf(output, "--- %s ---\n", logging.SanitizeExportText(file.Name))
		_, _ = output.WriteString(logging.SanitizeExportText(file.Content))
		if !strings.HasSuffix(file.Content, "\n") {
			_, _ = output.WriteString("\n")
		}
	}
}
