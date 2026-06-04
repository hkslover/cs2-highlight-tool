package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producegame"
)

const gameInfoCleanBackupRelativePath = "backups/gameinfo/gameinfo.gi"

type GameInfoStatus struct {
	Status       string `json:"status"`
	Clean        bool   `json:"clean"`
	HasBackup    bool   `json:"has_backup"`
	CanRepair    bool   `json:"can_repair"`
	GameInfoPath string `json:"gameinfo_path,omitempty"`
	BackupPath   string `json:"backup_path,omitempty"`
	Message      string `json:"message,omitempty"`
}

func (a *App) GetGameInfoStatus() (*GameInfoStatus, error) {
	return a.gameInfoStatus()
}

func (a *App) RepairGameInfoFromBackup() (*GameInfoStatus, error) {
	gameInfoPath, err := a.resolveCurrentGameInfoPath()
	if err != nil {
		return nil, err
	}
	backupPath := a.cleanGameInfoBackupPath()
	backupBytes, err := os.ReadFile(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("未找到可用于修复的 gameinfo 原版备份")
		}
		return nil, fmt.Errorf("读取 gameinfo 原版备份失败: %w", err)
	}
	if producegame.HasPluginSearchPath(string(backupBytes)) {
		return nil, fmt.Errorf("gameinfo 原版备份包含 csgo/plugin，无法用于修复")
	}
	if err := copyFileWithReplace(backupPath, gameInfoPath); err != nil {
		return nil, fmt.Errorf("恢复 gameinfo.gi 失败: %w", err)
	}
	return a.gameInfoStatus()
}

func (a *App) ensureCleanGameInfoBackup() error {
	gameInfoPath, err := a.resolveCurrentGameInfoPath()
	if err != nil {
		return err
	}
	return a.ensureCleanGameInfoBackupFromPath(gameInfoPath)
}

func (a *App) ensureCleanGameInfoBackupFromPath(gameInfoPath string) error {
	currentBytes, err := os.ReadFile(gameInfoPath)
	if err != nil {
		return fmt.Errorf("读取 gameinfo.gi 失败: %w", err)
	}
	if producegame.HasPluginSearchPath(string(currentBytes)) {
		return nil
	}

	backupPath := a.cleanGameInfoBackupPath()
	if _, err := os.ReadFile(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 gameinfo 原版备份失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return fmt.Errorf("创建 gameinfo 备份目录失败: %w", err)
	}
	if err := copyFileWithReplace(gameInfoPath, backupPath); err != nil {
		return fmt.Errorf("备份 gameinfo.gi 失败: %w", err)
	}
	return nil
}

func (a *App) gameInfoStatus() (*GameInfoStatus, error) {
	status := &GameInfoStatus{
		Status:     "unavailable",
		BackupPath: a.cleanGameInfoBackupPath(),
		Message:    "未找到 gameinfo.gi，请确认 CS2 路径配置",
	}

	gameInfoPath, err := a.resolveCurrentGameInfoPath()
	if err != nil {
		status.HasBackup = a.hasCleanGameInfoBackup()
		return status, nil
	}
	status.GameInfoPath = gameInfoPath

	contentBytes, err := os.ReadFile(gameInfoPath)
	if err != nil {
		return nil, fmt.Errorf("读取 gameinfo.gi 失败: %w", err)
	}
	status.Clean = !producegame.HasPluginSearchPath(string(contentBytes))
	status.HasBackup = a.hasCleanGameInfoBackup()
	status.CanRepair = !status.Clean && status.HasBackup
	if status.Clean {
		status.Status = "normal"
		status.Message = "配置文件正常"
		return status, nil
	}
	status.Status = "abnormal"
	status.Message = "配置文件异常"
	if !status.HasBackup {
		status.Message = "配置文件异常，且未找到可用于修复的原版备份"
	}
	return status, nil
}

func (a *App) hasCleanGameInfoBackup() bool {
	backupBytes, err := os.ReadFile(a.cleanGameInfoBackupPath())
	if err != nil {
		return false
	}
	return !producegame.HasPluginSearchPath(string(backupBytes))
}

func (a *App) resolveCurrentGameInfoPath() (string, error) {
	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return "", err
	}
	cs2Exe, err := resolveCS2ExeForLaunch(cfg)
	if err != nil {
		return "", err
	}
	gameInfoPath, err := producegame.ResolveGameInfoPath(cs2Exe, config.CleanPath(cfg.CS2Dir))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(gameInfoPath), nil
}

func (a *App) cleanGameInfoBackupPath() string {
	return a.dataPath(filepath.FromSlash(gameInfoCleanBackupRelativePath))
}
