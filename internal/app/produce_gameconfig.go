package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producegame"
)

const (
	produceGameInfoBackupSuffix  = ".cs2ht_produce.bak"
	producePluginDLLBackupSuffix = ".cs2ht_plugin.bak"
	producePovBackupSuffix       = ".cs2ht_pov.bak"
)

type gameInfoSessionState struct {
	gameInfoPath string
	backupPath   string
	modified     bool
}

type povSessionState struct {
	gameInfoPath     string
	gameInfoBackup   string
	gameInfoModified bool
	vpkPath          string
	vpkInstalled     bool
}

type pluginDLLSessionState struct {
	targetPath       string
	backupPath       string
	binDirPath       string
	pluginDirPath    string
	modified         bool
	binDirCreated    bool
	pluginDirCreated bool
}

func (a *App) prepareGameInfoForProduce() error {
	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return err
	}
	cs2Exe, err := resolveCS2ExeForLaunch(cfg)
	if err != nil {
		return err
	}
	gameInfoPath, err := producegame.ResolveGameInfoPath(cs2Exe, config.CleanPath(cfg.CS2Dir))
	if err != nil {
		return err
	}
	contentBytes, err := os.ReadFile(gameInfoPath)
	if err != nil {
		return fmt.Errorf("读取 gameinfo.gi 失败: %w", err)
	}
	content := string(contentBytes)
	if strings.Contains(content, "Game\tcsgo/plugin") {
		a.produceStateMu.Lock()
		a.produceState.gameInfo = gameInfoSessionState{
			gameInfoPath: gameInfoPath,
			backupPath:   "",
			modified:     false,
		}
		a.produceStateMu.Unlock()
		return nil
	}
	injected, ok := producegame.InjectPluginSearchPath(content)
	if !ok {
		return fmt.Errorf("无法在 gameinfo.gi 中找到可注入位置")
	}
	backupPath := gameInfoPath + produceGameInfoBackupSuffix
	if err := copyFile(gameInfoPath, backupPath); err != nil {
		return fmt.Errorf("备份 gameinfo.gi 失败: %w", err)
	}
	if err := os.WriteFile(gameInfoPath, []byte(injected), 0644); err != nil {
		return fmt.Errorf("写入 gameinfo.gi 失败: %w", err)
	}
	a.produceStateMu.Lock()
	a.produceState.gameInfo = gameInfoSessionState{
		gameInfoPath: gameInfoPath,
		backupPath:   backupPath,
		modified:     true,
	}
	a.produceStateMu.Unlock()
	return nil
}

func (a *App) preparePluginDLLForProduce() (retErr error) {
	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return err
	}
	cs2Exe, err := resolveCS2ExeForLaunch(cfg)
	if err != nil {
		return err
	}
	pluginSourcePath := config.CleanPath(cfg.PluginDLL)
	if pluginSourcePath == "" {
		return fmt.Errorf("插件 DLL 路径为空，请先在设置中配置")
	}
	sourceInfo, err := os.Stat(pluginSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("插件 DLL 不存在: %s", pluginSourcePath)
		}
		return fmt.Errorf("读取插件 DLL 失败: %w", err)
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("插件 DLL 路径不是文件: %s", pluginSourcePath)
	}

	gameInfoPath := ""
	a.produceStateMu.Lock()
	gameInfoPath = strings.TrimSpace(a.produceState.gameInfo.gameInfoPath)
	a.produceStateMu.Unlock()
	if gameInfoPath == "" {
		gameInfoPath, err = producegame.ResolveGameInfoPath(cs2Exe, config.CleanPath(cfg.CS2Dir))
		if err != nil {
			return err
		}
	}

	csgoDir := filepath.Dir(gameInfoPath)
	pluginDirPath := filepath.Join(csgoDir, "plugin")
	binDirPath := filepath.Join(pluginDirPath, "bin")
	targetPath := filepath.Join(binDirPath, "server.dll")
	modified := !samePath(pluginSourcePath, targetPath)
	backupPath := ""
	pluginDirCreated := false
	binDirCreated := false

	defer func() {
		if retErr == nil {
			return
		}
		if modified {
			if strings.TrimSpace(backupPath) != "" {
				_ = copyFileWithReplace(backupPath, targetPath)
				_ = os.Remove(backupPath)
			} else if strings.TrimSpace(targetPath) != "" {
				_ = os.Remove(targetPath)
			}
		}
		if binDirCreated {
			_ = removeDirIfEmpty(binDirPath)
		}
		if pluginDirCreated {
			_ = removeDirIfEmpty(pluginDirPath)
		}
	}()

	pluginInfo, err := os.Stat(pluginDirPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("读取插件目录失败: %w", err)
		}
		pluginDirCreated = true
	} else if !pluginInfo.IsDir() {
		return fmt.Errorf("插件目录路径被文件占用: %s", pluginDirPath)
	}

	binInfo, err := os.Stat(binDirPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("读取插件 bin 目录失败: %w", err)
		}
		binDirCreated = true
	} else if !binInfo.IsDir() {
		return fmt.Errorf("插件 bin 目录路径被文件占用: %s", binDirPath)
	}

	if err := os.MkdirAll(binDirPath, 0755); err != nil {
		return fmt.Errorf("创建插件 bin 目录失败: %w", err)
	}

	if modified {
		targetInfo, targetErr := os.Stat(targetPath)
		if targetErr == nil {
			if targetInfo.IsDir() {
				return fmt.Errorf("目标插件 DLL 路径被目录占用: %s", targetPath)
			}
			backupPath = targetPath + producePluginDLLBackupSuffix
			if err := copyFileWithReplace(targetPath, backupPath); err != nil {
				return fmt.Errorf("备份目标插件 DLL 失败: %w", err)
			}
		} else if !os.IsNotExist(targetErr) {
			return fmt.Errorf("读取目标插件 DLL 失败: %w", targetErr)
		}

		if err := copyFileWithReplace(pluginSourcePath, targetPath); err != nil {
			return fmt.Errorf("注入插件 DLL 失败: %w", err)
		}
	}

	a.produceStateMu.Lock()
	a.produceState.pluginDLL = pluginDLLSessionState{
		targetPath:       targetPath,
		backupPath:       backupPath,
		binDirPath:       binDirPath,
		pluginDirPath:    pluginDirPath,
		modified:         modified,
		binDirCreated:    binDirCreated,
		pluginDirCreated: pluginDirCreated,
	}
	a.produceStateMu.Unlock()
	return nil
}

func (a *App) forceRestoreGameInfoForProduce() error {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	state := a.produceState.gameInfo
	if !state.modified || strings.TrimSpace(state.backupPath) == "" {
		return nil
	}
	if _, err := os.Stat(state.backupPath); err != nil {
		if os.IsNotExist(err) {
			a.produceState.gameInfo = gameInfoSessionState{}
			return nil
		}
		return fmt.Errorf("读取 gameinfo 备份失败: %w", err)
	}
	if err := copyFile(state.backupPath, state.gameInfoPath); err != nil {
		return fmt.Errorf("恢复 gameinfo.gi 失败: %w", err)
	}
	_ = os.Remove(state.backupPath)
	a.produceState.gameInfo = gameInfoSessionState{}
	return nil
}

func (a *App) forceRestorePluginDLLForProduce() error {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	state := a.produceState.pluginDLL
	if !state.modified {
		return nil
	}

	var restoreErr error
	if strings.TrimSpace(state.backupPath) != "" {
		if _, err := os.Stat(state.backupPath); err != nil {
			if os.IsNotExist(err) {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("插件 DLL 备份不存在: %s", state.backupPath))
			} else {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("读取插件 DLL 备份失败: %w", err))
			}
		} else if err := copyFileWithReplace(state.backupPath, state.targetPath); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复目标插件 DLL 失败: %w", err))
		}
		if err := os.Remove(state.backupPath); err != nil && !os.IsNotExist(err) {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("清理插件 DLL 备份失败: %w", err))
		}
	} else if strings.TrimSpace(state.targetPath) != "" {
		if err := os.Remove(state.targetPath); err != nil && !os.IsNotExist(err) {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("移除注入插件 DLL 失败: %w", err))
		}
	}

	if state.binDirCreated {
		if err := removeDirIfEmpty(state.binDirPath); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("清理插件 bin 目录失败: %w", err))
		}
	}
	if state.pluginDirCreated {
		if err := removeDirIfEmpty(state.pluginDirPath); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("清理插件目录失败: %w", err))
		}
	}

	if restoreErr != nil {
		return restoreErr
	}
	a.produceState.pluginDLL = pluginDLLSessionState{}
	return nil
}

func (a *App) preparePovForProduce() error {
	cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
	if err != nil {
		return err
	}
	if !cfg.PovHudEnabled {
		return nil
	}

	cs2Exe, err := resolveCS2ExeForLaunch(cfg)
	if err != nil {
		return fmt.Errorf("POV HUD: 解析 CS2 路径失败: %w", err)
	}
	gameInfoPath, err := producegame.ResolveGameInfoPath(cs2Exe, config.CleanPath(cfg.CS2Dir))
	if err != nil {
		return fmt.Errorf("POV HUD: 未找到 gameinfo.gi: %w", err)
	}
	csgoDir := filepath.Dir(gameInfoPath)

	// 定位 pov.vpk 源文件 (dataDir/presets/pov/pov.vpk)
	povSrc := filepath.Join(a.dataRoot(), "presets", "pov", "pov.vpk")
	if _, err := os.Stat(povSrc); err != nil {
		return fmt.Errorf("POV HUD: pov.vpk 未找到 (%s)，请确保 presets/pov/pov.vpk 已安装", povSrc)
	}

	vpkTarget := filepath.Join(csgoDir, "pov.vpk")
	vpkInstalled := false
	vpkBackup := ""

	// 如果目标已存在 pov.vpk 且不是我们放进去的，先备份
	if info, err := os.Stat(vpkTarget); err == nil && !info.IsDir() {
		vpkBackup = vpkTarget + producePovBackupSuffix
		if err := copyFile(vpkTarget, vpkBackup); err != nil {
			return fmt.Errorf("POV HUD: 备份现有 pov.vpk 失败: %w", err)
		}
	}

	if err := copyFile(povSrc, vpkTarget); err != nil {
		// 如果复制失败但之前做了备份，恢复
		if vpkBackup != "" {
			_ = copyFile(vpkBackup, vpkTarget)
			_ = os.Remove(vpkBackup)
		}
		return fmt.Errorf("POV HUD: 安装 pov.vpk 失败: %w", err)
	}
	vpkInstalled = true

	// 读取 gameinfo.gi
	contentBytes, err := os.ReadFile(gameInfoPath)
	if err != nil {
		// 回滚 VPK
		if vpkInstalled {
			if vpkBackup != "" {
				_ = copyFile(vpkBackup, vpkTarget)
				_ = os.Remove(vpkBackup)
			} else {
				_ = os.Remove(vpkTarget)
			}
		}
		return fmt.Errorf("POV HUD: 读取 gameinfo.gi 失败: %w", err)
	}
	content := string(contentBytes)

	povPath := "csgo/pov"
	if strings.Contains(content, "Game\t"+povPath) {
		// 已经注入过，跳过
		a.produceStateMu.Lock()
		a.produceState.pov = povSessionState{
			gameInfoPath:     gameInfoPath,
			gameInfoBackup:   "",
			gameInfoModified: false,
			vpkPath:          vpkTarget,
			vpkInstalled:     vpkInstalled,
		}
		a.produceStateMu.Unlock()
		return nil
	}

	injected, ok := producegame.InjectSearchPath(content, povPath)
	if !ok {
		_ = os.Remove(vpkTarget)
		if vpkBackup != "" {
			_ = copyFile(vpkBackup, vpkTarget)
			_ = os.Remove(vpkBackup)
		}
		return fmt.Errorf("POV HUD: 无法在 gameinfo.gi 中注入 csgo/pov 搜索路径")
	}

	// 备份 gameinfo.gi
	gameInfoBackup := gameInfoPath + producePovBackupSuffix
	if err := copyFile(gameInfoPath, gameInfoBackup); err != nil {
		_ = os.Remove(vpkTarget)
		if vpkBackup != "" {
			_ = copyFile(vpkBackup, vpkTarget)
			_ = os.Remove(vpkBackup)
		}
		return fmt.Errorf("POV HUD: 备份 gameinfo.gi 失败: %w", err)
	}

	if err := os.WriteFile(gameInfoPath, []byte(injected), 0644); err != nil {
		_ = os.Remove(gameInfoBackup)
		_ = os.Remove(vpkTarget)
		if vpkBackup != "" {
			_ = copyFile(vpkBackup, vpkTarget)
			_ = os.Remove(vpkBackup)
		}
		return fmt.Errorf("POV HUD: 写入 gameinfo.gi 失败: %w", err)
	}

	a.produceStateMu.Lock()
	a.produceState.pov = povSessionState{
		gameInfoPath:     gameInfoPath,
		gameInfoBackup:   gameInfoBackup,
		gameInfoModified: true,
		vpkPath:          vpkTarget,
		vpkInstalled:     vpkInstalled,
	}
	a.produceStateMu.Unlock()
	return nil
}

func (a *App) forceRestorePovForProduce() error {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	state := a.produceState.pov
	var restoreErr error

	// 恢复 gameinfo.gi
	if state.gameInfoModified && strings.TrimSpace(state.gameInfoBackup) != "" {
		if _, err := os.Stat(state.gameInfoBackup); err == nil {
			if err := copyFile(state.gameInfoBackup, state.gameInfoPath); err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("POV HUD: 恢复 gameinfo.gi 失败: %w", err))
			}
			_ = os.Remove(state.gameInfoBackup)
		}
	}

	// 移除 pov.vpk，恢复原有文件(如果有备份)
	if state.vpkInstalled && strings.TrimSpace(state.vpkPath) != "" {
		vpkBackup := state.vpkPath + producePovBackupSuffix
		_ = os.Remove(state.vpkPath)
		if _, err := os.Stat(vpkBackup); err == nil {
			if err := copyFile(vpkBackup, state.vpkPath); err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("POV HUD: 恢复原有 pov.vpk 失败: %w", err))
			}
			_ = os.Remove(vpkBackup)
		}
	}

	a.produceState.pov = povSessionState{}
	return restoreErr
}

func (a *App) forceRestoreProduceEnvironmentForProduce() error {
	var restoreErr error
	if err := a.forceRestorePovForProduce(); err != nil {
		restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复 POV HUD 失败: %w", err))
	}
	if err := a.forceRestorePluginDLLForProduce(); err != nil {
		restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复插件 DLL 失败: %w", err))
	}
	if err := a.forceRestoreGameInfoForProduce(); err != nil {
		restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复 gameinfo 失败: %w", err))
	}
	return restoreErr
}
