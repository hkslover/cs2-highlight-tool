package app

import (
	"fmt"
	"strings"

	"cs2-highlight-tool-v2/internal/changelog"
	"cs2-highlight-tool-v2/internal/config"
)

// PendingChangelog 是 GetPendingChangelog 的返回值。
// ShouldShow=true 表示前端需要弹出更新日志 modal；ShouldShow=false 时前端不应渲染。
// BodyZh / BodyEn 是 markdown 正文；前端按当前 locale 选段（缺一段时回退到另一段）。
type PendingChangelog struct {
	Version    string `json:"version"`
	BodyZh     string `json:"body_zh"`
	BodyEn     string `json:"body_en"`
	ShouldShow bool   `json:"should_show"`
}

// GetPendingChangelog 返回当前版本是否需要展示更新日志。
// 展示规则：
//   - LastChangelogVersion == 当前版本 → 不展示
//   - LastChangelogVersion != 当前版本（含老用户首次升级到带本功能版本的情况，此时为空字符串）→ 尝试读 embed 文件
//   - embed 文件缺失 → 不展示（容错跳过，不写回 LastChangelogVersion）
//
// 全新安装的"首装静默"语义在 App 初始化阶段通过 config.EnsureFirstInstallChangelogSeed 实现，
// 这里不再区分"首装"与"老用户"。
func (a *App) GetPendingChangelog() (*PendingChangelog, error) {
	currentVersion := strings.TrimSpace(a.version)
	if currentVersion == "" {
		return &PendingChangelog{}, nil
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.LastChangelogVersion == currentVersion {
		return &PendingChangelog{Version: currentVersion, ShouldShow: false}, nil
	}
	notes, ok := changelog.Get(currentVersion)
	if !ok {
		return &PendingChangelog{Version: currentVersion, ShouldShow: false}, nil
	}
	return &PendingChangelog{
		Version:    notes.Version,
		BodyZh:     notes.BodyZh,
		BodyEn:     notes.BodyEn,
		ShouldShow: true,
	}, nil
}

// AckChangelog 在用户关闭更新日志 modal 后调用。
// 把 config.LastChangelogVersion 写回当前版本，避免下次启动重复弹窗。
// 当 version 与 a.version 不一致时仍以传入的 version 为准（前端不应当篡改，
// 但即便如此也不会留下"已读了一个不存在的版本"这种坏数据：下次升级时
// LastChangelogVersion 仍然与新的 a.version 不同，会再次弹出）。
func (a *App) AckChangelog(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("更新日志版本号为空")
	}
	_, err := a.updateConfig(func(cfg *config.Config) error {
		if cfg.LastChangelogVersion != version {
			cfg.LastChangelogVersion = version
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("保存更新日志确认状态失败: %w", err)
	}
	return nil
}
