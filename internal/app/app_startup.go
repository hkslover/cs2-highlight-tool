package app

import (
	"fmt"

	"cs2-highlight-tool-v2/internal/envsetup"
)

// workspaceNotInitializedErr 工作目录未初始化时的统一错误信息。
func workspaceNotInitializedErr() error {
	return fmt.Errorf("请先完成工作目录初始化")
}

func (a *App) GetStartupState() envsetup.StartupState {
	release, _, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	defer release()
	snapshot := a.workspaceSnapshot()
	svc := snapshot.service
	if svc == nil {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	return svc.GetStartupState()
}

// beginStartupTask snapshots the current service only after reserving the
// workspace. ResetWorkspace therefore cannot detach the service between the
// pointer read and the operation that uses it.
func (a *App) beginStartupTask() (func(), *envsetup.Service, bool) {
	release, _, err := a.beginManagedWorkspaceUse()
	if err != nil {
		return nil, nil, false
	}
	a.serviceMu.Lock()
	svc := a.service
	a.serviceMu.Unlock()
	if svc == nil {
		release()
		return nil, nil, false
	}
	return release, svc, true
}

func (a *App) RunStartupChecks() envsetup.StartupState {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	defer release()
	return svc.RunStartupChecks()
}

func (a *App) RetryStartupComponent(componentID string) envsetup.StartupState {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	defer release()
	return svc.RetryStartupComponent(componentID)
}

func (a *App) CancelStartupDownload(componentID string) envsetup.StartupState {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	defer release()
	return svc.CancelStartupDownload(componentID)
}

func (a *App) OpenManualDownload(componentID string) error {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return workspaceNotInitializedErr()
	}
	defer release()
	return svc.OpenManualDownload(componentID)
}

func (a *App) OpenExternalURL(rawURL string) error {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return workspaceNotInitializedErr()
	}
	defer release()
	return svc.OpenExternalURL(rawURL)
}

func (a *App) ImportManualDownload(componentID string) envsetup.StartupState {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	defer release()
	return svc.ImportManualDownload(componentID)
}

func (a *App) PickCS2Path() envsetup.StartupState {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}
	}
	defer release()
	return svc.PickCS2Path()
}

func (a *App) EnterMainApp() error {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return workspaceNotInitializedErr()
	}
	defer release()
	return svc.EnterMainApp()
}

func (a *App) ReinstallStartupComponent(componentID string) (envsetup.StartupState, error) {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return envsetup.StartupState{Mode: envsetup.ModeWorkspaceInit}, workspaceNotInitializedErr()
	}
	defer release()
	return svc.ReinstallStartupComponent(componentID)
}

func (a *App) ExportStartupLogs() (string, error) {
	release, svc, ok := a.beginStartupTask()
	if !ok {
		return "", workspaceNotInitializedErr()
	}
	defer release()
	return svc.ExportStartupLogs()
}
