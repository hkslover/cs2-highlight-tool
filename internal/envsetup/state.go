package envsetup

import (
	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/endpoints"
)

const (
	componentHLAE   = "hlae"
	componentPlugin = "plugin"
	componentFFmpeg = "ffmpeg"
	componentCS2    = "cs2"

	statusPending     = "pending"
	statusChecking    = "checking"
	statusDownloading = "downloading"
	statusInstalling  = "installing"
	statusReady       = "ready"
	statusWarning     = "warning"
	statusFailed      = "failed"
	statusNeedsAction = "needs_action"

	phaseDetectingSource = "detecting_source"
	phaseWaitingSource   = "waiting_source"
	phaseRunningTasks    = "running_tasks"
	phaseReady           = "ready"

	// ModeWorkspaceInit 表示当前未完成工作目录初始化，前端应展示初始化 modal。
	// StartupState.Mode 合法取值：{ModeWorkspaceInit, "startup", "main"}。
	ModeWorkspaceInit = "workspace_init"
	ModeStartup       = "startup"
	ModeMain          = "main"

	downloadCanceledMessage = "下载已取消"
)

type DownloadSource string

const (
	DownloadSourceGitHub DownloadSource = "github"
	DownloadSourceCustom DownloadSource = "custom"
)

// SourceStepState represents the state of the IP detection and download source selection step.
type SourceStepState struct {
	Status      string `json:"status"`       // pending/checking/ready/failed/needs_action
	Source      string `json:"source"`       // current preferred download source
	CountryCode string `json:"country_code"` // detected country code
	Message     string `json:"message"`      // human-readable status message
	Error       string `json:"error"`        // error message if detection failed
}

type StartupState struct {
	Mode         string            `json:"mode"`
	Phase        string            `json:"phase"`
	Running      bool              `json:"running"`
	SourceStep   SourceStepState   `json:"source_step"`
	FatalError   string            `json:"fatal_error"`
	EntryNotice  string            `json:"entry_notice"`
	Ads          []StartupAd       `json:"ads"`
	SelfUpdate   SelfUpdateState   `json:"self_update"`
	Steps        []ComponentStatus `json:"steps"`
	CanEnterMain bool              `json:"can_enter_main"`
	Config       config.Config     `json:"config"`
}

type StartupAd struct {
	ID        string `json:"id"`
	Enabled   bool   `json:"enabled"`
	Placement string `json:"placement"`
	ClickURL  string `json:"click_url"`
	Sponsor   string `json:"sponsor"`
	Title     string `json:"title"`
	RichHTML  string `json:"rich_html"`
	ImageURL  string `json:"image_url"`
	ImageAlt  string `json:"image_alt,omitempty"`
}

type SelfUpdateState struct {
	Status    string `json:"status"`
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
	Error     string `json:"error"`
}

type ComponentStatus struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	LocalVersion  string `json:"local_version"`
	RemoteVersion string `json:"remote_version"`
	Path          string `json:"path"`
	Error         string `json:"error"`
	ManualURL     string `json:"manual_url"`
}

type LogMessage struct {
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Time      string            `json:"time"`
	Component string            `json:"component,omitempty"`
	Stage     string            `json:"stage,omitempty"`
	Action    string            `json:"action,omitempty"`
	Source    string            `json:"source,omitempty"`
	Attempt   int               `json:"attempt,omitempty"`
	ElapsedMS int64             `json:"elapsed_ms,omitempty"`
	Error     string            `json:"error,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type ProgressMessage struct {
	ComponentID   string  `json:"component_id"`
	Active        bool    `json:"active"`
	Percent       float64 `json:"percent"`
	Indeterminate bool    `json:"indeterminate"`
}

func newStartupState(cfg *config.Config, currentVersion string) StartupState {
	if cfg == nil {
		cfg = &config.Config{}
	}
	source := cfg.DownloadSource
	if source == "" {
		source = string(defaultDownloadSource())
	}
	return StartupState{
		Mode:    ModeStartup,
		Phase:   phaseDetectingSource,
		Running: false,
		SourceStep: SourceStepState{
			Status:      statusPending,
			Source:      source,
			CountryCode: "",
			Message:     "等待检查统一更新源",
		},
		Ads: nil,
		SelfUpdate: SelfUpdateState{
			Status:  statusPending,
			Current: currentVersion,
		},
		Steps: []ComponentStatus{
			{ID: componentHLAE, Name: "HLAE", Status: statusPending, Path: cfg.HLAEExe, ManualURL: endpoints.ManualURLFor(componentHLAE, source)},
			{ID: componentPlugin, Name: "插件 DLL", Status: statusPending, Path: cfg.PluginDLL, ManualURL: endpoints.ManualURLFor(componentPlugin, source)},
			{ID: componentFFmpeg, Name: "ffmpeg", Status: statusPending, Path: config.JoinExe(cfg.FFmpegDir, "ffmpeg.exe"), ManualURL: endpoints.ManualURLFor(componentFFmpeg, source)},
			{ID: componentCS2, Name: "CS2 路径", Status: statusPending, Path: cfg.CS2Exe},
		},
		Config: *cfg,
	}
}

func (s StartupState) clone() StartupState {
	s.Steps = append([]ComponentStatus(nil), s.Steps...)
	s.Ads = append([]StartupAd(nil), s.Ads...)
	s.Config = *config.Clone(&s.Config)
	return s
}
