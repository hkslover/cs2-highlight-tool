package envsetup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/logging"
	"cs2-highlight-tool-v2/internal/release"
)

type Service struct {
	ctx        context.Context
	exeDir     string
	dataDir    string
	configPath string
	config     *config.Config
	version    string

	state    StartupState
	mu       sync.Mutex
	configMu sync.Mutex

	runTasksFn      func(source DownloadSource)
	logger          logging.Logger
	logWriter       *logging.RotatingWriter
	logs            []LogMessage
	releaseSnapshot *release.UnifiedLatest

	cancelMap map[string]*activeDownloadCancel
	cancelMu  sync.Mutex

	ffmpegDetectMu      sync.Mutex
	ffmpegDetectRunning bool
	ffmpegDetectCancel  context.CancelFunc
	ffmpegDetectWG      sync.WaitGroup
}

type activeDownloadCancel struct {
	cancel context.CancelFunc
}

func New(exeDir string, version string) *Service {
	return NewWithDataDir(exeDir, exeDir, version)
}

func NewWithDataDir(exeDir string, dataDir string, version string) *Service {
	if dataDir == "" {
		dataDir = exeDir
	}
	cfg := config.Default(dataDir)
	s := &Service{
		exeDir:     exeDir,
		dataDir:    dataDir,
		configPath: filepath.Join(dataDir, "config.json"),
		config:     cfg,
		version:    version,
		state:      newStartupState(cfg, version),
		cancelMap:  make(map[string]*activeDownloadCancel),
	}

	// 创建日志滚动写入器，同时输出到 stderr 和文件
	logDir := filepath.Join(dataDir, "logs")
	logWriter, err := logging.NewRotatingWriter(logDir)
	if err != nil {
		// 日志目录创建失败不致命，仅输出到 stderr
		logWriter = nil
	}
	var writer io.Writer = os.Stderr
	if logWriter != nil {
		writer = io.MultiWriter(os.Stderr, logWriter)
		s.logWriter = logWriter
	}
	s.logger = logging.NewSlogAdapter(logging.Options{
		Writer: writer,
		Sink:   s.appendLogEntry,
	})
	s.runTasksFn = s.runTasksDefault
	return s
}

func (s *Service) Startup(ctx context.Context) {
	s.ctx = ctx
	if s.exeDir == "" {
		return
	}

	// 清理旧日志文件（保留7天 + 最多50MB）
	if s.logWriter != nil {
		removed, removedSize, cleanErr := logging.CleanOldLogs(
			s.logWriter.LogDir(),
			logging.DefaultLogMaxAgeDays,
			logging.DefaultLogMaxTotalSize,
		)
		if cleanErr != nil {
			s.emitLog("warn", fmt.Sprintf("日志清理失败: %v", cleanErr))
		} else if removed > 0 {
			s.emitLog("info", fmt.Sprintf("已清理 %d 个旧日志文件，释放 %.1f MB", removed, float64(removedSize)/(1024*1024)))
		}
	}

	cfg, err := config.LoadOrCreate(s.configPath, s.dataDir)
	if err != nil {
		cfg = config.Default(s.dataDir)
		s.emitLog("error", fmt.Sprintf("加载配置失败: %v", err))
	}
	s.mu.Lock()
	s.config = cfg
	s.state = newStartupState(cfg, s.version)
	s.logs = nil
	s.releaseSnapshot = nil
	s.mu.Unlock()
	s.emitState()
}

func (s *Service) GetStartupState() StartupState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.clone()
}
