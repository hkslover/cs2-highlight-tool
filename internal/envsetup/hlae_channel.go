package envsetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/release"
)

// defaultHLAEChannelID is the channel used when the user has not picked one.
// It lives in <dataDir>/hlae so installations from before channels existed
// keep working without a migration.
const defaultHLAEChannelID = "main"

// hlaeChannelsDirName holds every non-default channel, one directory per
// channel ID. It is a sibling of <dataDir>/hlae because installing the
// default channel replaces that whole directory.
const hlaeChannelsDirName = "hlae_channels"

type hlaeTarget struct {
	ChannelID string
	Dir       string
	Info      *release.Info
	// Notice explains why the selected channel could not be honored.
	Notice string
}

func normalizeHLAEChannelID(channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !release.IsValidChannelID(channelID) {
		return defaultHLAEChannelID
	}
	return channelID
}

func (s *Service) hlaeChannelDir(channelID string) string {
	channelID = normalizeHLAEChannelID(channelID)
	if channelID == defaultHLAEChannelID {
		return filepath.Join(s.dataDir, "hlae")
	}
	return filepath.Join(s.dataDir, hlaeChannelsDirName, channelID)
}

// isManagedHLAEDir reports whether dir is a channel directory owned by the
// app, so it may be deleted on reinstall.
func (s *Service) isManagedHLAEDir(dir string) bool {
	dir = filepath.Clean(dir)
	if strings.EqualFold(dir, filepath.Clean(filepath.Join(s.dataDir, "hlae"))) {
		return true
	}
	return strings.EqualFold(filepath.Dir(dir), filepath.Clean(filepath.Join(s.dataDir, hlaeChannelsDirName)))
}

// hlaeChannelIDForExe maps an HLAE.exe inside a channel directory back to
// its channel ID, or "" for a path the app does not manage.
func (s *Service) hlaeChannelIDForExe(hlaeExe string) string {
	dir := filepath.Dir(filepath.Clean(hlaeExe))
	if !s.isManagedHLAEDir(dir) {
		return ""
	}
	if strings.EqualFold(dir, filepath.Clean(s.hlaeChannelDir(defaultHLAEChannelID))) {
		return defaultHLAEChannelID
	}
	return filepath.Base(dir)
}

// hlaeInstallDirs lists every channel directory that currently exists.
func (s *Service) hlaeInstallDirs() []string {
	dirs := make([]string, 0, 2)
	if info, err := os.Stat(s.hlaeChannelDir(defaultHLAEChannelID)); err == nil && info.IsDir() {
		dirs = append(dirs, s.hlaeChannelDir(defaultHLAEChannelID))
	}
	entries, err := os.ReadDir(filepath.Join(s.dataDir, hlaeChannelsDirName))
	if err != nil {
		return dirs
	}
	for _, entry := range entries {
		if entry.IsDir() && release.IsValidChannelID(entry.Name()) {
			dirs = append(dirs, filepath.Join(s.dataDir, hlaeChannelsDirName, entry.Name()))
		}
	}
	return dirs
}

func findChannel(channels []release.Channel, channelID string) (release.Channel, bool) {
	for _, channel := range channels {
		if channel.ID == channelID {
			return channel, true
		}
	}
	return release.Channel{}, false
}

func (s *Service) releaseSnapshotChannels() []release.Channel {
	s.mu.Lock()
	snapshot := s.releaseSnapshot
	s.mu.Unlock()
	if snapshot == nil {
		return nil
	}
	return snapshot.ComponentChannels(componentHLAE)
}

// resolveHLAETarget maps the selected channel to the release to install.
// A channel that is gone or whose version is not synced falls back to the
// default channel, and a default channel that is not published falls back to
// latest.
func (s *Service) resolveHLAETarget(source DownloadSource, selected string) (hlaeTarget, error) {
	if err := s.ensureReleaseSnapshot(source, false); err != nil {
		return hlaeTarget{}, err
	}
	s.mu.Lock()
	snapshot := s.releaseSnapshot
	s.mu.Unlock()
	if snapshot == nil {
		return hlaeTarget{}, errServiceStopped
	}
	channels := snapshot.ComponentChannels(componentHLAE)

	selected = normalizeHLAEChannelID(selected)
	notice := ""
	if selected != defaultHLAEChannelID {
		channel, ok := findChannel(channels, selected)
		if ok {
			if info, found := snapshot.ComponentVersionInfo(componentHLAE, channel.Tag); found {
				return hlaeTarget{ChannelID: selected, Dir: s.hlaeChannelDir(selected), Info: info}, nil
			}
			notice = fmt.Sprintf("HLAE 版本通道 %s 的版本 %s 暂不可用，已使用默认通道", channel.Name, channel.Tag)
		} else {
			notice = fmt.Sprintf("HLAE 版本通道 %s 已下线，已使用默认通道", selected)
		}
	}

	target := hlaeTarget{ChannelID: defaultHLAEChannelID, Dir: s.hlaeChannelDir(defaultHLAEChannelID), Notice: notice}
	if channel, ok := findChannel(channels, defaultHLAEChannelID); ok {
		if info, found := snapshot.ComponentVersionInfo(componentHLAE, channel.Tag); found {
			target.Info = info
			return target, nil
		}
	}
	info, err := s.componentReleaseInfo(source, componentHLAE)
	if err != nil {
		return hlaeTarget{}, err
	}
	target.Info = info
	return target, nil
}

// hlaeChannelOptions reads each channel's installed version from disk; call
// it outside s.mu.
func (s *Service) hlaeChannelOptions(channels []release.Channel) []ChannelOption {
	if len(channels) == 0 {
		return nil
	}
	options := make([]ChannelOption, 0, len(channels))
	for _, channel := range channels {
		option := ChannelOption{
			ID:   channel.ID,
			Name: channel.Name,
			Note: channel.Note,
			Tag:  channel.Tag,
		}
		if version, err := resolveInstalledHLAEVersion(filepath.Join(s.hlaeChannelDir(channel.ID), "HLAE.exe")); err == nil {
			option.LocalVersion = version
			option.Installed = release.CompareVersions(version, channel.Tag) == 0
		}
		options = append(options, option)
	}
	return options
}

func (s *Service) refreshHLAEChannelOptions(activeChannelID string) {
	options := s.hlaeChannelOptions(s.releaseSnapshotChannels())
	s.updateStep(componentHLAE, func(step *ComponentStatus) {
		step.Channels = options
		if activeChannelID != "" {
			step.Channel = activeChannelID
		}
	})
}

// SelectHLAEChannel persists the user's HLAE channel and re-checks HLAE. A
// channel whose version is already on disk only switches the active
// HLAE.exe; otherwise its version is downloaded into the channel directory.
func (s *Service) SelectHLAEChannel(channelID string) (StartupState, error) {
	_, releaseTask, ok := s.beginTaskIfOpen()
	if !ok {
		return s.GetStartupState(), errServiceStopped
	}
	defer releaseTask()

	channelID = strings.TrimSpace(channelID)
	s.emitLogWithFields("info", "用户切换 HLAE 版本通道", logFields{
		Component: componentHLAE,
		Action:    "select_channel",
		Meta: map[string]string{
			"channel": channelID,
		},
	})
	if _, found := findChannel(s.releaseSnapshotChannels(), channelID); !found {
		return s.GetStartupState(), fmt.Errorf("HLAE 版本通道不存在: %s", channelID)
	}

	s.mu.Lock()
	busy := s.state.Running
	if step := s.findStepLocked(componentHLAE); step != nil {
		switch step.Status {
		case statusChecking, statusDownloading, statusInstalling:
			busy = true
		}
	}
	if !busy {
		if s.state.Phase == phaseReady {
			s.state.Phase = phaseRunningTasks
		}
		s.state.Running = true
		s.state.CanEnterMain = false
	}
	s.mu.Unlock()
	if busy {
		return s.GetStartupState(), fmt.Errorf("环境检查进行中，请稍后再切换 HLAE 版本")
	}
	s.emitState()
	defer func() {
		if s.isStopped() {
			return
		}
		s.mu.Lock()
		s.state.Running = false
		s.mu.Unlock()
		s.emitState()
	}()

	if _, err := s.persistConfig(func(next *config.Config) error {
		next.HLAEChannel = channelID
		return nil
	}); err != nil {
		s.refreshCanEnterMain()
		s.updatePhaseByReadiness()
		return s.GetStartupState(), err
	}
	s.runComponent(componentHLAE, s.ensureHLAEWithFallback)
	s.refreshCanEnterMain()
	s.updatePhaseByReadiness()
	return s.GetStartupState(), nil
}
