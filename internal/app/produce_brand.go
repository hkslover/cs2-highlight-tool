package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/producegame"
)

// Only a VPK created by this session may be removed during teardown.
type brandSessionState struct {
	vpkPath      string
	vpkInstalled bool
}

// prepareBrandForProduce is unconditional: no recording setting controls branding.
func (a *App) prepareBrandForProduce() error {
	return a.prepareBrandForProduceWithOptions(true)
}

// prepareBrandForProduceWithOptions skips installation when branding is
// disabled for the current (debug) session. A pre-existing brand.vpk is never
// deleted; the disabled search path is handled by gameinfo preparation.
func (a *App) prepareBrandForProduceWithOptions(brandEnabled bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	return a.prepareBrandWithConfig(cfg, brandEnabled)
}

// prepareBrandWithConfig installs the brand VPK from one frozen config snapshot.
// A pre-existing brand.vpk is never deleted; disabling branding only skips the
// install, and the disabled search path is handled by gameinfo preparation.
func (a *App) prepareBrandWithConfig(cfg *config.Config, brandEnabled bool) error {
	if !brandEnabled {
		return nil
	}
	a.produceStateMu.Lock()
	gameInfoPath := strings.TrimSpace(a.produceState.gameInfo.gameInfoPath)
	a.produceStateMu.Unlock()
	if gameInfoPath == "" {
		cs2Exe, err := resolveCS2ExeForLaunch(cfg)
		if err != nil {
			return err
		}
		gameInfoPath, err = producegame.ResolveGameInfoPath(cs2Exe, config.CleanPath(cfg.CS2Dir))
		if err != nil {
			return err
		}
	}
	vpkPath := filepath.Join(filepath.Dir(gameInfoPath), "brand.vpk")
	if info, err := os.Lstat(vpkPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("品牌 vpk 目标路径不是普通文件: %s", vpkPath)
		}
		// Preserve both a user-supplied VPK and ownership on repeated preparation.
		a.produceStateMu.Lock()
		if a.produceState.brand.vpkPath != vpkPath {
			a.produceState.brand = brandSessionState{vpkPath: vpkPath}
		}
		a.produceStateMu.Unlock()
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取品牌 vpk 失败: %w", err)
	}
	if len(producegame.BrandVPK) == 0 {
		return fmt.Errorf("品牌 vpk 资源为空，无法投放")
	}
	file, err := os.OpenFile(vpkPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("创建品牌 vpk 失败: %w", err)
	}
	// Register before writing so launch rollback also removes a partial write.
	a.produceStateMu.Lock()
	a.produceState.brand = brandSessionState{vpkPath: vpkPath, vpkInstalled: true}
	a.produceStateMu.Unlock()
	_, writeErr := file.Write(producegame.BrandVPK)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("写入品牌 vpk 失败: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭品牌 vpk 失败: %w", closeErr)
	}
	return nil
}

func (a *App) forceRestoreBrandForEpoch(epoch uint64) error {
	a.produceStateMu.Lock()
	defer a.produceStateMu.Unlock()
	if epoch != 0 && a.produceState.envEpoch != epoch {
		return nil
	}
	state := a.produceState.brand
	if state.vpkInstalled && strings.TrimSpace(state.vpkPath) != "" {
		if err := os.Remove(state.vpkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("移除品牌 vpk 失败: %w", err)
		}
	}
	a.produceState.brand = brandSessionState{}
	return nil
}
