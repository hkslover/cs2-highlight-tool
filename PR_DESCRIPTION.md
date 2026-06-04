# PR: 多项功能增强与问题修复

## 概述

本次 PR 包含 5 项改进：路径中文/空格支持、POV HUD 实验性功能、4:3 录制拉伸修复、FFmpeg GPU 加速优化、应用日志持久化与自动清理。

---

## 改动详情

### 1. 安装路径支持中文和空格

**问题**：`internal/appdata/validate.go` 中 `validateDataDirChars()` 阻止了中文（非 ASCII）、空格等 Windows 原生支持的路径字符，导致用户无法使用含中文的用户名或带空格的目录。

**修改**：
- 移除 `validateDataDirChars()` 中的非 ASCII 检测（`r > 127`）、空格检测、正则白名单三步限制
- 仅保留 Windows 非法字符 `< > " | ? *` 检测
- `MaxDataDirLength` 从 200 增加到 260（Windows MAX_PATH 标准）
- `internal/app/app.go` 中 `isUsableDataDir()` 同步移除非 ASCII 检查
- 更新前端 i18n 描述文本

### 2. 新增实验性 "POV HUD" 功能

**功能**：启用后录制时会临时安装 `pov.vpk` 并修改 CS2 的 `gameinfo.gi` 注入 `csgo/pov` 搜索路径，录制完成后自动恢复。使录制画面更贴近第一人称 HUD 风格。

**修改**：
- `config.Config` / `ClipSettings` 新增 `PovHudEnabled` 字段
- `internal/producegame/gameinfo.go`：`InjectPluginSearchPath` 泛化为 `InjectSearchPath(content, path)` 支持任意搜索路径
- `internal/app/produce_gameconfig.go`：新增 `povSessionState`、`preparePovForProduce()`、`forceRestorePovForProduce()`，遵循现有备份/恢复模式
- `produce_session.go`：扩展会话状态
- `plugin_generate.go`：启动 HLAE 前调用 POV 准备
- 前端设置页新增开关 + 详细警告文本（中英文双语）

### 3. 修复 4:3 录制不自动拉伸

**问题**：4:3 分辨率（1440×1080 / 1280×960）录制时，HLAE 捕获的是方形像素的游戏帧缓冲。ffmpeg 编码时未写入 SAR/DAR 元数据，导致播放画面为窄 4:3 而非期望的 16:9。

**修改**：
- `internal/clipsjson/builder.go`：`BuildOptions` 新增 `LaunchResolution` 字段；`buildFFmpegParams` 对 4:3 分辨率自动追加 `-aspect 16:9`
- `plugin_generate.go`：传递 `LaunchResolution` 到 BuildOptions

### 4. 修复 FFmpeg GPU 加速

**问题**：当 FFmpeg 能力检测未完成或失败时，`ResolvePluginVideoPreset` 回退到 CPU 编码 `c1`（libx264），导致编码性能下降。

**修改**：
- `plugin_generate.go`：录制前检查 `FFmpegDetectedPreset` 和 `FFmpegDetectedEncoders` 缓存，若为空则执行同步快速检测（10秒超时），将结果持久化到 config
- 确保即使用户首次启动后立即开始录制，也能自动检测 GPU 编码器

### 5. 应用运行日志持久化与自动清理

**功能**：将应用运行日志自动写入 `{dataDir}/logs/cs2ht-YYYY-MM-DD.log` 文件（JSON 格式，逐行），同时输出到 stderr。启动时自动清理 7 天前的日志，且总大小不超过 50MB。

**新增文件**：
- `internal/logging/rotation.go`：`RotatingWriter` — 按天滚动的日志文件写入器，实现 `io.Writer`
- `internal/logging/cleanup.go`：`CleanOldLogs()` — 双重策略清理（天数 + 大小限制）

**修改**：
- `internal/envsetup/service.go`：创建 `RotatingWriter`，`io.MultiWriter` 双写 stderr + 文件；`Startup()` 调用清理
- `internal/envsetup/service_startup.go`：`ensureWorkDirs()` 新增 `logs` 目录
- `internal/app/outputs_storage.go`：新增 `GetLogStorageStats()` / `ClearLogs()` API
- 前端设置页新增日志存储信息卡片（文件数、占用空间、清理按钮）

---

## 修改文件清单

### 新增 (2)
- `internal/logging/rotation.go`
- `internal/logging/cleanup.go`

### 修改 (16)
| 改动 | 文件 |
|------|------|
| 1 | `internal/appdata/validate.go` |
| 1 | `internal/app/app.go` |
| 1 | `internal/appdata/validate_test.go` |
| 1 | `frontend/src/shared/i18n/zh-CN.json` |
| 1 | `frontend/src/shared/i18n/en-US.json` |
| 2,3,4 | `internal/app/plugin_generate.go` |
| 2 | `internal/config/config.go` |
| 2 | `internal/app/clip_settings.go` |
| 2 | `internal/producegame/gameinfo.go` |
| 2 | `internal/app/produce_gameconfig.go` |
| 2 | `internal/app/produce_session.go` |
| 2,5 | `frontend/src/features/settings/components/SettingsPanel.vue` |
| 2,5 | `frontend/src/shared/types/clips.ts` |
| 5 | `frontend/src/shared/types/index.ts` |
| 5 | `internal/envsetup/service.go` |
| 5 | `internal/envsetup/service_startup.go` |
| 5 | `internal/app/outputs_storage.go` |

---

## 测试

- `go vet` 通过（仅预存的 `cs2_process_windows.go:121 unsafe.Pointer` 警告）
- `go test ./internal/appdata/...` — 中文/空格/非 ASCII 测试通过
- `go test ./internal/clipsjson/...` — 通过
- `go test ./internal/producegame/...` — 通过
- `go test ./internal/logging/...` — 通过

## 注意事项

- POV HUD 功能需要用户在 `{dataDir}/presets/pov/` 下放置 `pov.vpk` 文件才会生效，否则启动录制时会提示文件未找到
- 所有改动向后兼容，不影响现有配置文件
