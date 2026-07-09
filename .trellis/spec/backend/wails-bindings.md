# Wails Bindings

Concrete contracts for Go methods exposed through `internal/app.App` and consumed by `window.go.app.App.*`.

## Scenario: Gameinfo Health Repair Contract

### 1. Scope / Trigger

- Trigger: The app must recover when a previous abnormal exit leaves any of this tool's injected search paths in CS2's `gameinfo.gi`. The known injected paths are enumerated by `producegame.SearchPathPlugin` (`csgo/plugin`) and `producegame.SearchPathPOV` (`csgo/pov.vpk`); both the health check and repair iterate that list, so adding a new path only requires extending it.
- Scope: `internal/app` Wails methods, `internal/producegame` line-level gameinfo helpers, frontend top bar health UI, and shared TypeScript types.
- Boundary: TopBar health button -> `GetGameInfoHealth` / `RepairGameInfo` -> CS2 `gameinfo.gi` file -> returned health state.

### 2. Signatures

```go
func (a *App) GetGameInfoHealth() (*GameInfoHealth, error)
func (a *App) RepairGameInfo() (*GameInfoHealth, error)

type GameInfoHealth struct {
    Status       string `json:"status"` // ok | needs_repair | unknown
    NeedsRepair bool   `json:"needs_repair"`
    GameInfoPath string `json:"gameinfo_path"`
    Message      string `json:"message"`
    Error        string `json:"error"`
}
```

Frontend shared type:

```ts
export type GameInfoHealthStatus = "ok" | "needs_repair" | "unknown";

export interface GameInfoHealth {
  status: GameInfoHealthStatus;
  needs_repair: boolean;
  gameinfo_path: string;
  message: string;
  error: string;
}
```

### 3. Contracts

- `GetGameInfoHealth` must be safe to call from the top bar on startup.
- If the workspace is not initialized, `GetGameInfoHealth` returns `status=unknown` and must not create `config.json`.
- `status=ok` means `gameinfo.gi` was found/read and contains no standalone `Game\t<known path>` or `Game <known path>` line for any path returned by `knownInjectedSearchPaths()` (currently `csgo/plugin` and `csgo/pov.vpk`).
- `status=needs_repair` means at least one standalone known-injected search path line exists and can be repaired by `RepairGameInfo`.
- `status=unknown` means config, CS2 exe, `gameinfo.gi`, or file reading could not be resolved; details go in `error`.
- `RepairGameInfo` removes every standalone line matching any path in `knownInjectedSearchPaths()`, then returns the latest health state. Detection and repair MUST iterate the same closure — never hardcode an individual path on one side and not the other.
- The user-facing health message is intentionally generic ("检测到 gameinfo.gi 搜索路径残留..."); do not regress to a plugin-specific string when the set may grow.
- The normal produce flow still uses backup-based restore; repair is a stateless crash-recovery fallback, not a replacement for session backups.

### 4. Validation & Error Matrix

- Workspace uninitialized -> return `unknown`, no config creation, no Go error.
- Config load failure -> return `unknown` with `error`, no Go error.
- CS2 exe/gameinfo missing -> return `unknown` with `error`, no Go error.
- Read failure during health check -> return `unknown` with `error`, no Go error.
- Read/write failure during repair after `needs_repair` -> return Go error with Chinese context.
- Healthy file repaired -> no-op and return `ok`.

### 5. Good/Base/Bad Cases

- Good: Abnormal exit leaves `Game\tcsgo/plugin` and/or `Game\tcsgo/pov.vpk`; startup health returns `needs_repair`, user clicks repair, every residual injected line is removed and state becomes `ok`.
- Base: Normal produce session still backs up `gameinfo.gi`, injects the required search paths (plugin always, pov when `PovHudEnabled`), and restores from backup on session end.
- Base: Workspace not initialized returns `unknown` without creating app data files.
- Bad: Using `strings.Replace` globally can remove comments or unrelated text.
- Bad: Replacing the normal backup restore path with line deletion loses exact restoration fidelity.
- Bad: Detection and repair iterating different sets of paths — repair removes a path the detection never reports, or detection flags a path repair leaves behind.

### 6. Tests Required

- `internal/producegame`: generic `HasSearchPath` / `RemoveSearchPath` / `InjectSearchPath` accept any path constant; tests must cover both `SearchPathPlugin` and `SearchPathPOV` (standalone tab/space lines detected, comments ignored, only the targeted path is touched).
- `internal/producegame`: a regression guard asserts every entry in the known-injected set round-trips through inject -> has -> remove without residual (`TestKnownInjectedSearchPathsAreHandledByHelpers`).
- `internal/producegame`: legacy `*PluginSearchPath` wrappers must remain as thin shims over the generic helpers so existing plugin callers compile unchanged.
- `internal/app`: `GetGameInfoHealth` reports `needs_repair` and `RepairGameInfo` repairs stale gameinfo without backup/session state — for both `csgo/plugin` and `csgo/pov.vpk` residuals, and any future entries in `knownInjectedSearchPaths()`.
- `internal/app`: healthy repair is idempotent.
- `internal/app`: uninitialized workspace health check returns `unknown` and does not create `config.json`.
- Frontend: `cd frontend && npm run build` passes for shared type and top bar usage.

### 7. Wrong vs Correct

#### Wrong

```go
// Hardcoded path + global string replace: damages comments and unrelated text,
// and silently misses any other injected path (e.g. csgo/pov.vpk).
content = strings.ReplaceAll(content, "Game\tcsgo/plugin", "")
```

```go
// Asymmetric detection vs. repair: detection only checks one path while
// repair removes several (or vice versa). Health state diverges from reality.
if producegame.HasSearchPath(content, producegame.SearchPathPlugin) {
    needsRepair = true
}
for _, p := range knownInjectedSearchPaths() {
    content, _ = producegame.RemoveSearchPath(content, p)
}
```

#### Correct

```go
// Detection and repair iterate the same closure, so adding a path is a
// single-line change and the two sides cannot drift.
repaired := string(contentBytes)
changed := false
for _, searchPath := range knownInjectedSearchPaths() {
    next, pathChanged := producegame.RemoveSearchPath(repaired, searchPath)
    repaired = next
    changed = changed || pathChanged
}
```

`RemoveSearchPath` itself walks the file line-by-line and only drops standalone `Game\t<path>` / `Game <path>` entries, preserving comments and unrelated content.

## Scenario: Debug Plugin DLL Override Contract

### 1. Scope / Trigger

- Trigger: Debug mode needs a temporary way to test a custom CS2 plugin DLL without replacing the installed plugin component.
- Scope: `internal/app` Wails methods, `preparePluginDLLForProduce`, frontend settings debug section, and shared TypeScript state type.
- Boundary: Settings debug row -> `GetDebugPluginDLLOverride` / `PickDebugPluginDLLOverride` / `ClearDebugPluginDLLOverride` -> session-only App state -> produce launch copies the selected DLL into CS2 `game/csgo/plugin/bin/server.dll`.

### 2. Signatures

```go
func (a *App) GetDebugPluginDLLOverride() DebugPluginDLLOverrideState
func (a *App) PickDebugPluginDLLOverride() (DebugPluginDLLOverrideState, error)
func (a *App) ClearDebugPluginDLLOverride() DebugPluginDLLOverrideState

type DebugPluginDLLOverrideState struct {
    Active bool   `json:"active"`
    Path   string `json:"path"`
}
```

Frontend shared type:

```ts
export interface DebugPluginDLLOverrideState {
  active: boolean;
  path: string;
}
```

### 3. Contracts

- The override is session-only state on `internal/app.App`; it must not be written to `config.json`.
- `PickDebugPluginDLLOverride` validates that the selected path is an existing `.dll` file. The filename does not need to be `server.dll` because the produce flow always copies the selected source to the fixed target `server.dll`.
- `preparePluginDLLForProduce` resolves the source DLL as debug override first, then `config.PluginDLL`.
- Target path, backup, rollback, and empty-directory cleanup remain owned by the existing `preparePluginDLLForProduce` / `forceRestorePluginDLLForProduce` flow.
- Startup plugin component checks and local version detection continue to use the installed plugin directory and its `changelog.xml`; debug override must not affect component readiness or version truth.
- Frontend may show the override only inside the debug settings section and must use the Wails methods as the source of truth.

### 4. Validation & Error Matrix

- `GetDebugPluginDLLOverride` before selection -> `{active:false,path:""}`.
- User cancels the file picker -> return current state, no error.
- Empty selected path -> return current state, no error.
- Non-`.dll` selected path -> return Go error `请选择 DLL 文件: <path>`.
- Missing selected path -> return Go error `调试插件 DLL 不存在: <path>`.
- Directory selected where a file is required -> return Go error `调试插件 DLL 路径不是文件: <path>`.
- Override file deleted after selection -> produce launch fails through `preparePluginDLLForProduce` with the normal missing-plugin error path.

### 5. Good/Base/Bad Cases

- Good: A developer selects `D:\dev\my-plugin.dll`; produce launch copies those bytes to `<cs2>/game/csgo/plugin/bin/server.dll`, then restores/removes the target through the existing session cleanup.
- Base: No override is selected; produce launch uses `config.PluginDLL` exactly as before.
- Base: Clearing the override returns immediately to the configured installed plugin DLL.
- Bad: Persisting the debug path as `config.PluginDLL`, because startup plugin version checks require `changelog.xml` next to the installed `server.dll`.
- Bad: Writing directly into the CS2 target from the picker; this bypasses the produce session backup/restore safety net.

### 6. Tests Required

- `internal/app`: `preparePluginDLLForProduce` uses the debug override bytes when an override is active.
- `internal/app`: clearing the debug override falls back to the configured `PluginDLL`.
- `go test ./...` must pass after adding or changing the Wails methods.
- `cd frontend && npm run build` must pass after changing shared types or the settings UI.

### 7. Wrong vs Correct

#### Wrong

```go
cfg.PluginDLL = selectedDebugDLL
_ = config.Save(a.configPath(), cfg)
```

This pollutes persistent component state and can break startup version detection.

#### Correct

```go
pluginSourcePath := a.resolvePluginDLLSourcePath(cfg) // override first, config fallback
if err := copyFileWithReplace(pluginSourcePath, targetPath); err != nil {
    return fmt.Errorf("注入插件 DLL 失败: %w", err)
}
```

The debug path affects only the source file selected for this produce session.

## Scenario: Startup FFmpeg Reinstall Probe Cancellation Contract

### 1. Scope / Trigger

- Trigger: User clicks FFmpeg “重新安装” while the startup FFmpeg capability detector may still be running.
- Scope: `internal/app` Wails `ReinstallStartupComponent`, `internal/envsetup` FFmpeg reinstall flow, and `internal/ffmpegprofile` capability probing.
- Boundary: UI reinstall button -> `ReinstallStartupComponent("ffmpeg")` -> `Service.reinstallFFmpeg()` -> cancel/wait active detector -> remove `<dataDir>/ffmpeg`.

### 2. Signatures

```go
func (a *App) ReinstallStartupComponent(componentID string) (envsetup.StartupState, error)
func (s *Service) reinstallFFmpeg() error
func (s *Service) stopFFmpegCapabilityDetection()
func DetectCapabilities(ctx context.Context, ffmpegExe string, cmdFactory CommandContextFunc) (Capabilities, error)
```

### 3. Contracts

- FFmpeg reinstall must stop any active FFmpeg capability detection before deleting `<dataDir>/ffmpeg`.
- Stopping detection must cancel the detector context and wait for the detector goroutine to exit.
- Canceled detection must not persist `ffmpeg_detected_preset`, `ffmpeg_detected_encoders`, or `ffmpeg_detected_at`.
- `DetectCapabilities` must return promptly with `context.Canceled` when its context is canceled between or during probes.
- HLAE / Plugin reinstall behavior is unchanged.

### 4. Validation & Error Matrix

- No active detector -> `stopFFmpegCapabilityDetection` returns immediately.
- Active detector -> cancel context, wait for `ffmpegDetectWG`, then continue reinstall.
- Detector canceled -> log probe completion with `canceled=true`, do not emit failure state, do not write detection cache.
- Directory removal still fails after detector stopped -> return `删除 ffmpeg 目录失败: %w`; remaining causes are external locks or filesystem errors.

### 5. Good/Base/Bad Cases

- Good: reinstall immediately after startup cancels the in-flight `ffmpeg.exe` probe before `os.RemoveAll`.
- Base: normal startup still schedules FFmpeg capability detection asynchronously and `ensureFFmpeg` returns without waiting for slow probes.
- Bad: calling `os.RemoveAll(<dataDir>/ffmpeg)` while `ffmpeg.exe` probe is still running.
- Bad: canceling detection but still persisting fallback encoder cache from a canceled probe run.

### 6. Tests Required

- `internal/envsetup`: slow detector can be canceled by `stopFFmpegCapabilityDetection` in under one second and writes no cache.
- `internal/envsetup`: existing async/single-flight detector tests remain green.
- `internal/ffmpegprofile`: package tests pass after adding context-cancel short-circuit behavior.
- `go test ./...` must pass.

### 7. Wrong vs Correct

#### Wrong

```go
func (s *Service) reinstallFFmpeg() error {
    return os.RemoveAll(filepath.Join(s.dataDir, "ffmpeg"))
}
```

This can fail on Windows because the app’s own background detector may still be executing `ffmpeg.exe`.

#### Correct

```go
func (s *Service) reinstallFFmpeg() error {
    s.stopFFmpegCapabilityDetection()
    return os.RemoveAll(filepath.Join(s.dataDir, "ffmpeg"))
}
```

Cancel and wait first, then delete the directory.

## Scenario: Startup Component Download Cancellation Contract

### 1. Scope / Trigger

- Trigger: Startup wizard lets users cancel active HLAE / Plugin / FFmpeg component downloads.
- Scope: `internal/app` Wails binding, `internal/envsetup` state machine, `internal/download` cancellation support, and frontend startup task actions.
- Boundary: UI cancel button -> Wails `CancelStartupDownload(componentID)` -> envsetup active download cancel func -> `download.FileWithContext`.

### 2. Signatures

```go
func (a *App) CancelStartupDownload(componentID string) envsetup.StartupState
func (s *Service) CancelStartupDownload(componentID string) StartupState
func FileWithContext(ctx context.Context, url, targetPath string, emitProgress ProgressFunc) error
```

Frontend call:

```ts
callBackend("CancelStartupDownload", componentID);
```

### 3. Contracts

- `componentID` may cancel only `hlae`, `plugin`, or `ffmpeg`.
- Self-update and `cs2` do not expose or honor cancel-download behavior.
- Cancel only mutates state when the component has an active download registered in `Service.cancelMap`.
- Successful cancel sets the component step to `failed`, sets `error` to `下载已取消`, stops progress, and keeps retry/manual import available through existing failed-state UI.
- `download.File(url, targetPath, progress)` remains the compatibility API for non-startup download callers; startup cancellation uses `FileWithContext`.

### 4. Validation & Error Matrix

- Unsupported component ID -> return current state unchanged and log a warning.
- Supported component with no active download -> return current state unchanged and log a warning.
- Active download canceled -> return state with that component failed and propagate `download.ErrCanceled`.
- `download.ErrCanceled` inside release URL fallback -> stop remaining URL attempts.
- `download.ErrCanceled` inside HLAE / Plugin local fallback wrapper -> do not convert to local-version `warning`.

### 5. Good/Base/Bad Cases

- Good: canceling HLAE during `downloading` stops the HTTP request, removes the partial archive, and does not attempt the next mirror URL.
- Base: 5E / Wanmei demo imports continue using `download.File` without context parameters.
- Bad: changing `download.File` signature and forcing unrelated import modules to pass `context.Background()`.
- Bad: detecting cancellation with `strings.Contains(err.Error(), "下载已取消")`.

### 6. Tests Required

- `internal/download`: canceling `FileWithContext` returns `errors.Is(err, download.ErrCanceled)` and removes the partial target.
- `internal/envsetup`: canceling with no active download or unsupported component leaves step status unchanged.
- `internal/envsetup`: canceling during `downloadAndInstallWithFallback` stops remaining candidates.
- `go test ./...` and `cd frontend && npm run build` must pass after adding the Wails method and frontend call.

### 7. Wrong vs Correct

#### Wrong

```go
func File(ctx context.Context, url string, targetPath string, progress ProgressFunc) error
```

This leaks startup-specific cancellation into unrelated demo import callers.

#### Correct

```go
func File(url, targetPath string, progress ProgressFunc) error {
    return FileWithContext(context.Background(), url, targetPath, progress)
}
```

Only startup download code opts into `FileWithContext` and cancellation tracking.

## Scenario: Clip Settings Recording Quality Contract

### 1. Scope / Trigger

- Trigger: Settings UI exposes recording quality through `GetClipSettings` / `SaveClipSettings`, and plugin JSON generation uses it to build HLAE recording ffmpeg params.
- Scope: `internal/config` persistence validation, `internal/app` Wails binding, `internal/clipsjson` build options, `internal/ffmpegprofile` recording params, and frontend `ClipSettings` type/options.
- Boundary: Settings UI selection -> Wails `SaveClipSettings` -> `config.json` -> plugin JSON bootstrap -> HLAE `mirv_streams settings add ffmpeg`.

### 2. Signatures

```go
func (a *App) GetClipSettings() (*ClipSettings, error)
func (a *App) SaveClipSettings(input ClipSettings) (*ClipSettings, error)

type ClipSettings struct {
    RecordQuality string `json:"record_quality"`
}

type Config struct {
    RecordQuality string `json:"record_quality"`
}

func BuildRecordingEncodeArgs(profileID string, quality string) (string, error)
```

Frontend shared type:

```ts
record_quality: "standard" | "high" | "ultra";
```

### 3. Contracts

- `record_quality` allowed values are `standard`, `high`, and `ultra`.
- Default is `high`.
- `high` must preserve the previous static HLAE recording params exactly.
- Software encoding (`c1/libx264`) maps quality to `crf`.
- Hardware encoding maps quality to encoder-specific QP parameters:
  - NVENC/AMF use `qp`.
  - QSV uses `q:v`.
- H264 fallback profiles (`n1_h264`, `a1_h264`, `i1_h264`) must be covered even though they are not exposed as frontend user presets.
- `auto` remains a user-facing preset only; resolve it to the selected concrete profile before applying recording quality.

### 4. Validation & Error Matrix

- Missing, empty, mixed-case, or unsupported `record_quality` -> normalize through `ffmpegprofile.NormalizeEditQuality` semantics and fall back to `high`.
- Unsupported recording profile -> return `不支持的 video_preset: <profile>`.
- `record_quality` changes must not alter record FPS, output directory, voice/xray commands, pixel format, GOP, or preset selection.

### 5. Good/Base/Bad Cases

- Good: selecting `ultra` with `n1` generates `-qp 10` in the plugin JSON bootstrap.
- Good: selecting `standard` with `i1_h264` generates `-q:v 22`.
- Base: missing `record_quality` in an existing config loads as `high` and emits the same HLAE params as before.
- Bad: only changing `c1`/CPU encoding and leaving hardware accelerated presets at fixed static values.
- Bad: applying edit-composition C1 CRF values (`18/16/14`) to recording; recording C1 keeps `high=-crf 4`.

### 6. Tests Required

- `internal/ffmpegprofile`: table-driven coverage for every recording profile and every quality level, including H264 fallback profiles.
- `internal/ffmpegprofile`: `high` output equals existing `profileCatalog.HLAEParams`.
- `internal/config`: defaults, valid preservation, and invalid fallback for `record_quality`.
- `internal/app`: `GetClipSettings` / `SaveClipSettings` round-trip and fallback behavior.
- `internal/clipsjson`: `BuildOptions.RecordQuality` changes the bootstrap ffmpeg command for a hardware preset.
- Frontend: `cd frontend && npm run build` must pass so the shared TypeScript union and settings select stay aligned.

### 7. Wrong vs Correct

#### Wrong

```go
if profileID == ffmpegprofile.UserPresetC1 {
    return buildCRFParams(quality)
}
return profile.HLAEParams
```

This makes the UI option work only for CPU/software encoding and silently ignores hardware accelerated recording.

#### Correct

```go
params, err := ffmpegprofile.BuildRecordingEncodeArgs(resolvedProfileID, settings.RecordQuality)
```

The resolved concrete profile determines whether quality is expressed as `crf`, `qp`, or `q:v`.

## Scenario: Clip Settings Launch Resolution Contract

### 1. Scope / Trigger

- Trigger: Settings UI exposes CS2 launch resolution choices through `GetClipSettings` / `SaveClipSettings`, and HLAE launch consumes the persisted value.
- Scope: `internal/app` Wails binding, `internal/config` persistence validation, frontend `ClipSettings` type/options, and HLAE command-line generation.
- Boundary: Settings UI selection -> Wails `SaveClipSettings` -> `config.json` -> HLAE `-cmdLine`.

### 2. Signatures

```go
func (a *App) GetClipSettings() (*ClipSettings, error)
func (a *App) SaveClipSettings(input ClipSettings) (*ClipSettings, error)

type ClipSettings struct {
    LaunchResolution string `json:"launch_resolution"`
}
```

Frontend shared type:

```ts
launch_resolution: "16:9" | "4:3" | "4:3_1280x960";
```

### 3. Contracts

- `launch_resolution` allowed values:
  - `16:9`: launch without explicit 4:3 width/height override.
  - `4:3`: launch with `-w 1440 -h 1080`; recording output is tagged with FFmpeg `-aspect 16:9` for stretched playback.
  - `4:3_1280x960`: launch with `-w 1280 -h 960`; recording output is tagged with FFmpeg `-aspect 16:9` for stretched playback.
- Default is `4:3`.
- Existing `4:3` config values must continue to mean `1440x1080`; do not repurpose this value for lower 4:3 resolutions.
- Frontend option labels are i18n keys under `main.settings.*`; per frontend rules, new labels are added to `zh-CN.json` only.

### 4. Validation & Error Matrix

- Missing, empty, or unsupported `launch_resolution` -> normalize to `config.DefaultLaunchResolution`.
- Supported value with surrounding whitespace -> trim and preserve the supported value.
- HLAE command generation should ignore unsupported values and omit resolution override rather than inventing dimensions.

### 5. Good/Base/Bad Cases

- Good: selecting `4:3_1280x960` persists that exact value and launches with `-w 1280 -h 960`.
- Base: selecting `4:3` preserves backward-compatible `-w 1440 -h 1080` behavior.
- Base: 4:3 recording output uses display aspect metadata for 16:9 stretched playback; do not add pixel scaling unless a separate output-size feature requires it.
- Bad: changing the meaning of `4:3` to `1280x960`, which silently alters existing user configs.

### 6. Tests Required

- `internal/config`: `ApplyDefaults` preserves every supported `launch_resolution` value.
- `internal/app`: `SaveClipSettings` and `GetClipSettings` round-trip each supported launch resolution value.
- `internal/app`: `buildHLAECommandLine` maps `4:3` to `1440x1080`, maps `4:3_1280x960` to `1280x960`, and leaves `16:9` without 4:3 dimensions.
- `internal/clipsjson`: generated FFmpeg recording settings include `-aspect 16:9` for `4:3` and `4:3_1280x960`, and omit it for `16:9` / empty values.
- Frontend: `cd frontend && npm run build` must pass so the shared TypeScript union and settings options stay aligned.

### 7. Wrong vs Correct

#### Wrong

```go
if launchResolution == "4:3" {
    cmdLine += " -w 1280 -h 960"
}
```

This breaks existing persisted `4:3` settings that already mean `1440x1080`.

#### Correct

```go
switch launchResolution {
case config.LaunchResolution4x3:
    cmdLine += " -w 1440 -h 1080"
case config.LaunchResolution4x3Low:
    cmdLine += " -w 1280 -h 960"
}
```

## Scenario: Clip Settings Hide All UI Contract

### 1. Scope / Trigger

- Trigger: Settings UI exposes a hide-all-UI switch through `GetClipSettings` / `SaveClipSettings`, and plugin JSON generation consumes the persisted value.
- Scope: `internal/config` persistence, `internal/app` Wails binding, `internal/clipsjson` bootstrap generation, frontend `ClipSettings` type, and Settings UI.
- Boundary: Settings UI toggle -> Wails `SaveClipSettings` -> `config.json` -> plugin JSON bootstrap command list.

### 2. Signatures

```go
func (a *App) GetClipSettings() (*ClipSettings, error)
func (a *App) SaveClipSettings(input ClipSettings) (*ClipSettings, error)

type ClipSettings struct {
    HideAllUI bool `json:"hide_all_ui"`
}

type Config struct {
    HideAllUI bool `json:"hide_all_ui"`
}

type BuildOptions struct {
    HideAllUI bool
}
```

Frontend shared type:

```ts
hide_all_ui: boolean;
```

### 3. Contracts

- `hide_all_ui` default is `false`.
- `hide_all_ui=true` writes `cl_draw_only_deathnotices 1` into the plugin JSON bootstrap sequence.
- `hide_all_ui=false` must not write any `cl_draw_only_deathnotices` command.
- This is a global clip setting only; it is not part of per-clip `clip_overrides`.
- Frontend option labels are i18n keys under `main.settings.*`; per frontend rules, add new labels to `zh-CN.json` only.

### 4. Validation & Error Matrix

- Missing `hide_all_ui` in an existing config -> load as `false` and save back with `hide_all_ui:false`.
- Unsupported JSON type for `hide_all_ui` -> standard JSON unmarshal failure, surfaced from `config.LoadOrCreate`.
- Disabled setting -> no reset command is emitted; do not emit `cl_draw_only_deathnotices 0`.

### 5. Good/Base/Bad Cases

- Good: user enables the switch, saves settings, and generated bootstrap contains `cl_draw_only_deathnotices 1`.
- Base: legacy config without `hide_all_ui` loads with the switch off and generated bootstrap contains no `cl_draw_only_deathnotices` command.
- Bad: always writing `cl_draw_only_deathnotices 0` when disabled, because it changes generated command output even when the user did not opt in.

### 6. Tests Required

- `internal/config`: legacy config loads with `HideAllUI=false` and saved config contains `hide_all_ui`.
- `internal/app`: `GetClipSettings` default is false; `SaveClipSettings` round-trips true.
- `internal/clipsjson`: bootstrap contains `cl_draw_only_deathnotices 1` only when `BuildOptions.HideAllUI=true`, and contains no command with that prefix when false.
- Frontend: `cd frontend && npm run build` must pass so the shared TypeScript type and settings UI stay aligned.

### 7. Wrong vs Correct

#### Wrong

```go
actions = append(actions, Action{Cmd: "cl_draw_only_deathnotices 0", Tick: actionTick})
```

This writes a command even when the user did not enable hide-all-UI.

#### Correct

```go
if opts.HideAllUI {
    actions = append(actions, Action{Cmd: "cl_draw_only_deathnotices 1", Tick: actionTick})
}
```

The generated command appears only for the opt-in behavior.

## Scenario: Clip Settings Shoulder Camera Contract

### 1. Scope / Trigger

- Trigger: Settings UI exposes a shoulder-camera switch through `GetClipSettings` / `SaveClipSettings`, and plugin JSON generation consumes the persisted value.
- Scope: `internal/config` persistence, `internal/app` Wails binding, `internal/clipsjson` bootstrap generation, frontend `ClipSettings` type, and Settings UI.
- Boundary: Settings UI toggle -> Wails `SaveClipSettings` -> `config.json` -> plugin JSON bootstrap command list.

### 2. Signatures

```go
func (a *App) GetClipSettings() (*ClipSettings, error)
func (a *App) SaveClipSettings(input ClipSettings) (*ClipSettings, error)

type ClipSettings struct {
    UseShoulderCamera bool `json:"use_shoulder_camera"`
}

type Config struct {
    UseShoulderCamera bool `json:"use_shoulder_camera"`
}

type BuildOptions struct {
    UseShoulderCamera bool
}
```

Frontend shared type:

```ts
use_shoulder_camera: boolean;
```

### 3. Contracts

- `use_shoulder_camera` default is `false`.
- `use_shoulder_camera=true` writes the shoulder-camera command into the plugin JSON bootstrap sequence before `r_show_build_info 0`.
- `use_shoulder_camera=false` must not write any `cam_command` or `c_thirdpersonshoulder` command.
- This is a global clip setting only; it is not part of per-clip `clip_overrides`.
- Frontend option labels are i18n keys under `main.settings.*`; per frontend rules, add new labels to `zh-CN.json` only.

### 4. Validation & Error Matrix

- Missing `use_shoulder_camera` in an existing config -> load as `false` and save back with `use_shoulder_camera:false`.
- Unsupported JSON type for `use_shoulder_camera` -> standard JSON unmarshal failure, surfaced from `config.LoadOrCreate`.
- Disabled setting -> no reset command is emitted; do not emit shoulder-camera commands with zero values.

### 5. Good/Base/Bad Cases

- Good: user enables the switch, saves settings, and generated bootstrap starts with the shoulder-camera command followed by `r_show_build_info 0`.
- Base: legacy config without `use_shoulder_camera` loads with the switch off and generated bootstrap contains no shoulder-camera command.
- Bad: appending the shoulder-camera command after `r_show_build_info 0`, because the UI contract requires it before that bootstrap command.

### 6. Tests Required

- `internal/config`: legacy config loads with `UseShoulderCamera=false` and saved config contains `use_shoulder_camera`.
- `internal/app`: `GetClipSettings` default is false; `SaveClipSettings` round-trips true.
- `internal/clipsjson`: bootstrap contains the shoulder-camera command only when `BuildOptions.UseShoulderCamera=true`, and the command precedes `r_show_build_info 0`.
- Frontend: `cd frontend && npm run build` must pass so the shared TypeScript type and settings UI stay aligned.

## Scenario: Independent Sky Blackout and Clouds Settings Contract

### 1. Scope / Trigger

- Trigger: Settings must control the skybox and HLAE cloud layer independently when generating plugin JSON bootstrap actions.
- Scope: `internal/config` persistence, `internal/app` Wails settings binding, `internal/clipsjson` command generation, frontend `ClipSettings`, and Settings UI.
- Boundary: Settings switches -> `SaveClipSettings` -> `config.json` -> plugin JSON bootstrap command list.

### 2. Signatures

```go
type Config struct {
    SkyBlackout   bool `json:"sky_blackout"`
    DisableClouds bool `json:"disable_clouds"`
}

type ClipSettings struct {
    SkyBlackout   bool `json:"sky_blackout"`
    DisableClouds bool `json:"disable_clouds"`
}

type BuildOptions struct {
    SkyBlackout   bool
    DisableClouds bool
}
```

Frontend shared type:

```ts
sky_blackout: boolean;
disable_clouds: boolean;
```

### 3. Contracts

- `sky_blackout` defaults to `true`; when enabled it writes only `r_drawskybox 0`.
- `disable_clouds` defaults to `false`; when enabled it writes only `mirv_sky clouds draw 0`.
- The switches are independent. Enabling either setting must not emit the other setting's command.
- Disabled settings emit no reset command. In particular, do not write `r_drawskybox 1` or `mirv_sky clouds draw 1`.
- `disable_clouds` is a global clip setting only and is not part of per-clip `clip_overrides`.
- Frontend labels use `main.settings.*`; only `zh-CN.json` is maintained by AI changes.

### 4. Validation & Error Matrix

- Missing `sky_blackout` in an existing config -> backfill `true` using the existing compatibility rule.
- Missing `disable_clouds` in an existing config -> retain Go's zero value `false` and save the field as `false`.
- Unsupported JSON type for either boolean -> standard JSON unmarshal failure from `config.LoadOrCreate`.
- Both settings disabled -> generated bootstrap contains neither command.

### 5. Good/Base/Bad Cases

- Good: sky blackout on and clouds off -> only `r_drawskybox 0` is emitted.
- Good: sky blackout off and clouds on -> only `mirv_sky clouds draw 0` is emitted.
- Base: both on -> both commands are emitted; both off -> neither is emitted.
- Bad: nesting both commands under `if opts.SkyBlackout`, because the UI switches then have coupled behavior.
- Bad: backfilling `disable_clouds=true` for legacy configs, because the product default is explicitly opt-in/off.

### 6. Tests Required

- `internal/clipsjson`: table-driven coverage for sky-only, clouds-only, both enabled, and both disabled; assert exact command presence/absence.
- `internal/config`: new and legacy configs default `DisableClouds=false`, save `disable_clouds`, and preserve explicit values.
- `internal/app`: `GetClipSettings` defaults `DisableClouds=false`; `SaveClipSettings` round-trips `true` through disk reload.
- Frontend: `cd frontend && npm run build` passes with both default `ClipSettings` object literals and the Settings switch.
- Full backend: `go test ./...` passes.

### 7. Wrong vs Correct

#### Wrong

```go
if opts.SkyBlackout {
    actions = append(actions, Action{Cmd: "mirv_sky clouds draw 0", Tick: actionTick})
    actions = append(actions, Action{Cmd: "r_drawskybox 0", Tick: actionTick})
}
```

#### Correct

```go
if opts.DisableClouds {
    actions = append(actions, Action{Cmd: "mirv_sky clouds draw 0", Tick: actionTick})
}
if opts.SkyBlackout {
    actions = append(actions, Action{Cmd: "r_drawskybox 0", Tick: actionTick})
}
```

## Scenario: Settings Outputs Storage Management

### 1. Scope / Trigger

- Trigger: Settings needs to inspect and mutate app-managed files under `<dataDir>/outputs`.
- Scope: Wails binding methods in `internal/app`; frontend consumes the JSON response shape through shared TypeScript types.
- Boundary: Filesystem state -> Go binding response -> Vue Settings UI.

### 2. Signatures

```go
func (a *App) GetOutputsStorageStats() (*OutputsStorageStats, error)
func (a *App) OpenOutputsDirectory() error
func (a *App) ClearOutputsDirectory() (*OutputsStorageStats, error)

type OutputsStorageStats struct {
    OutputDir      string `json:"output_dir"`
    VideoCount     int    `json:"video_count"`
    TotalSizeBytes int64  `json:"total_size_bytes"`
}
```

### 3. Contracts

- `output_dir`: absolute managed outputs directory, resolved through `a.fixedRecordOutputDir()`.
- `video_count`: recursive count of generated video files with supported video extensions.
- `total_size_bytes`: recursive sum of all regular file bytes under `output_dir`, including non-video intermediate files.
- `GetOutputsStorageStats` must create `output_dir` if missing before scanning.
- `OpenOutputsDirectory` must create `output_dir` if missing before opening it.
- `ClearOutputsDirectory` deletes every direct child under `output_dir` and preserves `output_dir` itself.

### 4. Validation & Error Matrix

- Missing outputs directory -> create it and return zero stats.
- Directory creation fails -> return `创建输出目录失败: %w`.
- Directory scan/read fails -> return `统计输出目录失败: %w` or `读取输出目录失败: %w`.
- Child deletion fails -> return `清理输出目录失败: %w`.
- OS folder opener fails -> return `打开输出目录失败: %w`.

### 5. Good/Base/Bad Cases

- Good: nested `.mp4/.mov/.mkv/.avi` files are counted as videos, and all files contribute to `total_size_bytes`.
- Base: empty or missing `outputs` returns `video_count=0` and `total_size_bytes=0`.
- Bad: cleanup must not remove `<dataDir>/outputs` itself and must not target any sibling directory.

### 6. Tests Required

- Filesystem test with nested video and non-video files:
  - Assert `output_dir` equals `<dataDir>/outputs`.
  - Assert video count is recursive and case-insensitive.
  - Assert total size includes all files.
- Cleanup test:
  - Seed files and nested folders under outputs.
  - Call `ClearOutputsDirectory`.
  - Assert returned stats are zero, outputs directory remains, and direct children are gone.

### 7. Wrong vs Correct

#### Wrong

```go
os.RemoveAll(a.dataPath("outputs"))
```

This removes the managed directory itself and makes later UI/open-folder behavior depend on implicit recreation.

#### Correct

```go
entries, err := os.ReadDir(outputDir)
for _, entry := range entries {
    if err := os.RemoveAll(filepath.Join(outputDir, entry.Name())); err != nil {
        return nil, fmt.Errorf("清理输出目录失败: %w", err)
    }
}
```

This clears all managed children while preserving the stable directory path.

## Scenario: Settings Demo Storage Management

### 1. Scope / Trigger

- Trigger: Settings needs to inspect and mutate app-managed Demo files under `<dataDir>/demo`.
- Scope: Wails binding methods in `internal/app`; frontend consumes the JSON response shape through shared TypeScript types.
- Boundary: Filesystem state -> Go binding response -> Vue Settings UI.

### 2. Signatures

```go
func (a *App) GetDemoStorageStats() (*DemoStorageStats, error)
func (a *App) OpenDemoDirectory() error
func (a *App) ClearDemoDirectory() (*DemoStorageStats, error)

type DemoStorageStats struct {
    DemoDir        string `json:"demo_dir"`
    DemoCount      int    `json:"demo_count"`
    TotalSizeBytes int64  `json:"total_size_bytes"`
}
```

### 3. Contracts

- `demo_dir`: absolute managed Demo directory, resolved through `a.dataPath("demo")`.
- `demo_count`: recursive count of `.dem` files under `demo_dir`, case-insensitive.
- `total_size_bytes`: recursive sum of all regular file bytes under `demo_dir`, including non-Demo metadata or temporary files.
- `GetDemoStorageStats` must create `demo_dir` if missing before scanning.
- `OpenDemoDirectory` must create `demo_dir` if missing before opening it.
- `ClearDemoDirectory` deletes every direct child under `demo_dir` and preserves `demo_dir` itself.
- Clearing `demo_dir` is intentionally broad: it removes `raw`, `wanmei`, `5e`, and any other direct child owned by the managed Demo storage root.

### 4. Validation & Error Matrix

- Missing Demo directory -> create it and return zero stats.
- Directory creation fails -> return `创建Dem 目录失败: %w`.
- Directory scan/read fails -> return `统计Dem 目录失败: %w` or `读取Dem 目录失败: %w`.
- Child deletion fails -> return `清理Dem 目录失败: %w`.
- OS folder opener fails -> return `打开Dem 目录失败: %w`.

### 5. Good/Base/Bad Cases

- Good: nested `.dem/.DEM` files in `raw`, `wanmei`, and `5e` are counted as demos, and all files contribute to `total_size_bytes`.
- Base: empty or missing `demo` returns `demo_count=0` and `total_size_bytes=0`.
- Bad: cleanup must not remove `<dataDir>/demo` itself and must not target `<dataDir>/projects`, `<dataDir>/outputs`, or sibling directories.

### 6. Tests Required

- Filesystem test with nested Demo and non-Demo files:
  - Assert `demo_dir` equals `<dataDir>/demo`.
  - Assert Demo count is recursive and case-insensitive.
  - Assert total size includes all files.
- Cleanup test:
  - Seed files and nested folders under demo.
  - Call `ClearDemoDirectory`.
  - Assert returned stats are zero, demo directory remains, and direct children are gone.

### 7. Wrong vs Correct

#### Wrong

```go
os.RemoveAll(a.dataPath("demo"))
```

This removes the managed root and makes later UI/open-folder behavior depend on implicit recreation while widening the destructive operation.

#### Correct

```go
entries, err := os.ReadDir(demoDir)
for _, entry := range entries {
    if err := os.RemoveAll(filepath.Join(demoDir, entry.Name())); err != nil {
        return nil, fmt.Errorf("清理Dem 目录失败: %w", err)
    }
}
```

This clears all managed Demo children while preserving the stable directory path.

## Scenario: 5E Recent Match Query Input Normalization

### 1. Scope / Trigger

- Trigger: 5E import needs to accept both a raw 5E domain ID and the profile share text/link copied from the 5E client.
- Scope: `internal/app.App.ListFiveERecentMatches` Wails binding normalizes the input before config persistence and `internal/fivee` API calls.
- Boundary: UI text input -> Wails binding -> config cache -> 5E match-list HTTP query.

### 2. Signatures

```go
func (a *App) GetFiveEPlayerName() string
func (a *App) ListFiveERecentMatches(playerName string, page int) (*fivee.FiveEMatchListResult, error)
func NormalizePlayerDomainInput(raw string) string
```

### 3. Contracts

- `playerName`: accepts either a raw 5E domain ID such as `12139xi22eza`, a URL/query string containing `domain=12139xi22eza`, or a full client share text containing that URL.
- `ListFiveERecentMatches` must call `fivee.NormalizePlayerDomainInput` before saving `fivee_player_name` and before issuing the remote request.
- `fivee_player_name`: stores the normalized domain ID when extraction succeeds, not the full pasted share text.
- Empty or whitespace-only input remains empty, saves empty, skips remote calls, and returns an empty match list.
- Non-link input with no `domain=` parameter remains trim-normalized and is passed through unchanged for backward compatibility.

### 4. Validation & Error Matrix

- Whitespace-only input -> no error; return `{player_name:"", matches:[]}` and do not call the 5E API.
- Share text/link with non-empty `domain` -> use extracted domain ID for persistence and query.
- Input without `domain` -> use trimmed input as-is.
- 5E API HTTP/JSON/business error -> return the existing wrapped Chinese error from `internal/fivee`.

### 5. Good/Base/Bad Cases

- Good: `【5E对战平台：...】https://csgo.5eplay.com/app/share_loding_type7?domain=12139xi22eza&tab=77` queries with `domain=12139xi22eza` and caches `12139xi22eza`.
- Base: `12139xi22eza` queries and caches `12139xi22eza`.
- Bad: caching the whole share text causes subsequent app startup auto-refresh to send an invalid `domain` query.

### 6. Tests Required

- App-layer regression test:
  - Call `ListFiveERecentMatches` with a full 5E share text.
  - Assert outbound HTTP query `domain` equals the extracted ID.
  - Assert returned `player_name` and cached `fivee_player_name` equal the extracted ID.
- Leaf helper test:
  - Assert raw ID, full share text, query-string-only, and empty input normalize as expected.
- Existing empty-input test must continue to assert no remote call.

### 7. Wrong vs Correct

#### Wrong

```go
playerName = strings.TrimSpace(playerName)
cfg.FiveEPlayerName = playerName
matches, err := fivee.ListRecentMatches(playerName, page)
```

This stores and queries the entire pasted share text.

#### Correct

```go
playerName = fivee.NormalizePlayerDomainInput(playerName)
cfg.FiveEPlayerName = playerName
matches, err := fivee.ListRecentMatches(playerName, page)
```

This keeps all callers on the same normalized 5E domain ID contract.

## Scenario: POV HUD Recording Contract

### 1. Scope / Trigger

- Trigger: A user-facing `pov_hud_enabled` toggle in Settings controls whether produce sessions inject a POV HUD overlay (rendered by a CS2 `pov.vpk` asset shipped in the binary). The asset is embedded into the Go binary, dropped into `csgo/pov.vpk` at session start when the toggle is on, and removed afterwards.
- Scope: `internal/config` persistence, `internal/app` Wails bindings (`GetClipSettings` / `SaveClipSettings`), `internal/app` produce session prepare/restore (`prepareGameInfoForProduce` / `preparePovForProduce` / `forceRestorePovForProduce`), `internal/producegame` (`PovVPK` embedded asset, generic SearchPath helpers), frontend `ClipSettings` type, and Settings UI switch.
- Boundary: Settings UI switch → `SaveClipSettings` → `config.json (pov_hud_enabled)` → produce launch sequence drops `csgo/pov.vpk` + injects `csgo/pov.vpk` search path → session end / failure restores both.

### 2. Signatures

```go
// internal/config
type Config struct {
    PovHudEnabled bool `json:"pov_hud_enabled"`
}

// internal/app
type ClipSettings struct {
    PovHudEnabled bool `json:"pov_hud_enabled"`
}

// internal/producegame
//go:embed assets/pov.vpk
var PovVPK []byte

// internal/app produce session
type povSessionState struct {
    vpkPath      string // resolved csgo/pov.vpk path under CS2 dir
    vpkInstalled bool   // true ONLY when this session wrote the file
}

func (a *App) prepareGameInfoForProduce() error  // multi-path: plugin + (pov if enabled)
func (a *App) preparePovForProduce() error        // no-op when !cfg.PovHudEnabled
func (a *App) forceRestorePovForProduce() error   // deletes only when vpkInstalled
```

Frontend shared type:

```ts
pov_hud_enabled: boolean;
```

### 3. Contracts

- `pov_hud_enabled` default is `false`; missing-from-config loads as `false`.
- The `pov.vpk` asset is embedded into the Go binary via `go:embed`; there is NO online download fallback and NO version negotiation. Asset bytes are tied to the app release.
- **Multi-path gameinfo invariant**: `prepareGameInfoForProduce` builds a target-path set (`["csgo/plugin"]` always; appends `"csgo/pov.vpk"` when `cfg.PovHudEnabled`). It performs ONE backup (`.cs2ht_produce.bak`) and ONE write injecting all missing paths via `producegame.InjectSearchPath`. The injected path for POV is the **vpk file path** (`csgo/pov.vpk`), NOT a directory — CS2 mounts the vpk archive directly.
- **Single-backup invariant**: gameinfo has exactly ONE backup file per session, `.cs2ht_produce.bak`. No `.cs2ht_pov.bak` or any other gameinfo backup variant may be created — neither for gameinfo nor for the vpk file. (The R4 health repair flow in the Gameinfo Health Repair Contract above relies on this.)
- **Idempotent early-return**: When gameinfo already contains ALL target paths, `prepareGameInfoForProduce` returns without backup/write and records `modified=false`. The check MUST iterate every target path (not just `csgo/plugin`) — short-circuiting on the first match leaves a half-prepared gameinfo when plugin is present but pov is not.
- **vpk "write-if-absent, delete-only-what-we-wrote" lifecycle**: `preparePovForProduce` stats `csgo/pov.vpk`. If the file already exists, it sets `vpkInstalled=false` and leaves the user's file alone. If absent, it writes `producegame.PovVPK` and sets `vpkInstalled=true`. `forceRestorePovForProduce` deletes the file ONLY when `vpkInstalled=true`; otherwise it is a no-op. This protects users who manually placed a custom pov.vpk.
- **Restore order**: `forceRestoreProduceEnvironmentForProduce` runs `pluginDLL → POV vpk → gameinfo`. Gameinfo is restored last because the search-path entry must remain valid until the dependent assets (plugin DLL, vpk) are torn down.
- **Launch rollback**: Any failure in `preparePovForProduce` (or `prepareGameInfoForProduce` / `preparePluginDLLForProduce` / `launchHLAEGame`) MUST trigger `forceRestoreProduceEnvironmentForProduce` before returning; partial state is not allowed to escape.
- **Toggle-off regression guard**: When `cfg.PovHudEnabled=false`, the gameinfo target-path set is plugin-only, `preparePovForProduce` returns nil with empty state, and behavior is byte-identical to the legacy plugin-only path.
- **Crash residual handling**: The Gameinfo Health Repair Contract (above) covers `csgo/pov.vpk` search-path residuals via `knownInjectedSearchPaths()`. The vpk file itself is intentionally NOT in the repair scope — once the search path is removed, CS2 will not load `csgo/pov.vpk`, so the file becomes inert.

### 4. Validation & Error Matrix

- Missing `pov_hud_enabled` in an existing config → load as `false`, save back with `pov_hud_enabled:false`.
- `cfg.PovHudEnabled=true` + cannot resolve CS2 dir / gameinfo path → return Go error from `prepareGameInfoForProduce` with Chinese context; nothing is written; launch aborted.
- `cfg.PovHudEnabled=true` + `csgo/pov.vpk` write fails (e.g., disk full, ACL) → return Go error from `preparePovForProduce`; launch caller MUST call `forceRestoreProduceEnvironmentForProduce` to undo gameinfo + plugin DLL.
- `cfg.PovHudEnabled=true` + `csgo/pov.vpk` already exists → silently use the user's file (`vpkInstalled=false`); no error; no backup.
- Restore fails after a successful prepare → `forceRestorePovForProduce` joins errors with the rest of the restore chain via `errors.Join`; partial restore is reported to the user as a single combined message.

### 5. Good/Base/Bad Cases

- **Good (toggle on, clean state)**: User flips the switch, saves settings, launches a take. gameinfo gets both `csgo/plugin` and `csgo/pov.vpk` from a single backup/write. `csgo/pov.vpk` is written from the embedded bytes. Session ends → vpk deleted, gameinfo restored from `.cs2ht_produce.bak`, csgo dir matches its pre-session state byte-for-byte.
- **Good (toggle on, user pre-placed vpk)**: User already has a custom `csgo/pov.vpk` (e.g., a community HUD). prepare skips the write (`vpkInstalled=false`). Session ends → user's file untouched, gameinfo restored normally.
- **Base (toggle off)**: gameinfo target-path set is `["csgo/plugin"]` only; no vpk activity; behavior identical to pre-task plugin-only path. This MUST remain a regression guard.
- **Bad: introducing `.cs2ht_pov.bak`** — adding a second backup file for the vpk (or for gameinfo's pov state) breaks the single-backup invariant and creates ambiguity about the source of truth during health repair.
- **Bad: short-circuit early-return on plugin alone** — `if HasPluginSearchPath(content) { return }` skips POV injection when plugin is already present, leaving gameinfo half-prepared.
- **Bad: deleting `csgo/pov.vpk` unconditionally during restore** — destroys a user-placed file when `vpkInstalled=false`.
- **Bad: restoring gameinfo before tearing down vpk / plugin DLL** — produces a transient window where CS2 sees the original gameinfo while the dependent assets are still in place.

### 6. Tests Required

- `internal/config`: legacy config without `pov_hud_enabled` loads with the field defaulted to `false`; saved config emits the key.
- `internal/app`: `GetClipSettings` default is `false`; `SaveClipSettings` round-trips `true`.
- `internal/producegame`: `PovVPK` is non-empty and matches the expected asset size (regression guard so silent asset-truncation is caught in CI).
- `internal/app`: with `PovHudEnabled=true` and no pre-existing vpk, after `prepareGameInfoForProduce` + `preparePovForProduce`:
  - gameinfo contains both `Game\tcsgo/plugin` and `Game\tcsgo/pov.vpk`
  - exactly one backup file `.cs2ht_produce.bak` exists in csgo dir
  - `csgo/pov.vpk` exists with the embedded bytes
  - `vpkInstalled=true`
- `internal/app`: with `PovHudEnabled=true` and a pre-existing `csgo/pov.vpk` (different bytes), after prepare:
  - vpk bytes are unchanged (user file preserved)
  - `vpkInstalled=false`
- `internal/app`: with `PovHudEnabled=true`, after `forceRestoreProduceEnvironmentForProduce`:
  - gameinfo restored to pre-session bytes
  - `csgo/pov.vpk` deleted IFF `vpkInstalled=true` (else present and unchanged)
  - NO `.cs2ht_pov.bak` file anywhere in csgo dir at any point — assert via directory scan
- `internal/app`: with `PovHudEnabled=false`, prepare + restore is a strict regression of the legacy plugin-only flow (no vpk touched, gameinfo only contains `csgo/plugin`).
- `internal/app`: launch-failure rollback — simulate `launchHLAEGame` failure with `PovHudEnabled=true`, assert `forceRestoreProduceEnvironmentForProduce` ran and the csgo dir is clean.
- Frontend: `cd frontend && npm run build` passes; `ClipSettings` interface and SettingsPanel switch stay aligned.

### 7. Wrong vs Correct

#### Wrong

```go
// Hardcoded plugin-only early-return — silently skips POV injection when
// plugin is already present but POV is not, leaving gameinfo half-prepared.
content := string(contentBytes)
if producegame.HasPluginSearchPath(content) {
    a.produceState.gameInfo = gameInfoSessionState{gameInfoPath: gameInfoPath, modified: false}
    return nil
}
injected, _ := producegame.InjectPluginSearchPath(content)
```

```go
// Backup-then-overwrite for vpk introduces .cs2ht_pov.bak, breaking the
// single-backup invariant and risking ambiguity during health repair.
backup := vpkPath + ".cs2ht_pov.bak"
_ = os.Rename(vpkPath, backup)
_ = os.WriteFile(vpkPath, producegame.PovVPK, 0644)
state.vpkBackup = backup
```

#### Correct

```go
// Build the target-path set from the toggle, then require ALL paths to be
// present for the early-return. The injection loop handles the missing ones.
targetPaths := []string{producegame.SearchPathPlugin}
if cfg.PovHudEnabled {
    targetPaths = append(targetPaths, producegame.SearchPathPOV)
}
content := string(contentBytes)
allPresent := true
for _, p := range targetPaths {
    if !producegame.HasSearchPath(content, p) {
        allPresent = false
        break
    }
}
if allPresent {
    a.produceState.gameInfo = gameInfoSessionState{gameInfoPath: gameInfoPath, modified: false}
    return nil
}
injected := content
for _, p := range targetPaths {
    next, ok := producegame.InjectSearchPath(injected, p)
    if !ok {
        return fmt.Errorf("无法在 gameinfo.gi 中注入搜索路径 %s", p)
    }
    injected = next
}
// ... single backup + single write
```

```go
// Stat first, write only if absent, track ownership in vpkInstalled.
// Restore deletes ONLY what we wrote. No backup file is ever created.
if _, err := os.Stat(vpkPath); err == nil {
    a.produceState.pov = povSessionState{vpkPath: vpkPath, vpkInstalled: false}
    return nil
}
if err := os.WriteFile(vpkPath, producegame.PovVPK, 0644); err != nil {
    return fmt.Errorf("写入 POV vpk 失败: %w", err)
}
a.produceState.pov = povSessionState{vpkPath: vpkPath, vpkInstalled: true}
```

## Scenario: Full-Round Player POV Recording Contract

### 1. Scope / Trigger

- Trigger: The clips/produce flow supports a separate full-round first-person POV recording mode for a selected demo/player, independent from the existing kill-window clip selection model.
- Scope: `internal/demo` round/death parsing, `internal/app` Wails request/response structs, `internal/clipsjson` action generation, `internal/plugingen` history keys, and frontend clip/produce state.
- Boundary: Clip selection UI -> `GeneratePluginJSONBatchAndLaunchHLAE` / `GeneratePluginJSONBatch` -> demo parser -> plugin JSON take plans -> produce history.

### 2. Signatures

```go
type GeneratePluginJSONRequest struct {
    DemoPath      string             `json:"demo_path"`
    TickRate      float64            `json:"tick_rate"`
    SelectedItems []SelectedClipItem `json:"selected_items,omitempty"`
    FullRoundPOV  *FullRoundPOVItem  `json:"full_round_pov,omitempty"`
}

type FullRoundPOVItem struct {
    PlayerSteamID string `json:"player_steam_id"`
}

// Wails method (no file generation, only returns plan for UI preview)
func (a *App) PreviewFullRoundPOV(demoPath, playerSteamID string) (*demo.FullRoundPOVPlan, error)

type SelectedClipItem struct {
    Kill          demo.ClipKill      `json:"kill"`
    IncludeKiller *bool             `json:"include_killer,omitempty"`
    IncludeVictim bool              `json:"include_victim"`
}

type ProduceTakePlan struct {
    View          string   `json:"view"` // killer | victim | full_round_pov
    KillIDs       []string `json:"kill_ids"`
    SourceID      string   `json:"source_id,omitempty"`
    Round         int      `json:"round,omitempty"`
    PlayerName    string   `json:"player_name,omitempty"`
    PlayerSteamID string   `json:"player_steam_id,omitempty"`
    StartTick     int      `json:"start_tick,omitempty"`
    EndTick       int      `json:"end_tick,omitempty"`
    EndReason     string   `json:"end_reason,omitempty"` // target_death | round_end
}
```

Frontend request:

```ts
interface GeneratePluginJSONRequest {
  demo_path: string;
  tick_rate: number;
  selected_items: GeneratePluginSelectedItem[];
  full_round_pov?: { player_steam_id: string };
}
```

### 3. Contracts

- `full_round_pov.player_steam_id` is optional. When present, it identifies exactly one tracked player for that demo.
- Full-round POV is demo/player-level state. It must not be represented as fake `selected_items[]` kills.
- `selected_items[].include_killer` defaults to `true` when omitted. Full-round POV mode uses `include_killer=false` for victim-only kill rows.
- Full-round POV take generation must run before victim clip take generation.
- Full-round POV segment recording starts at `RoundStart`; fallback to `RoundFreezetimeEnd` only if round start is missing. Valid kill/death filtering still starts at `RoundFreezetimeEnd` when available, so freeze-time events do not create live-round segments.
- Full-round POV segment end is the tracked player's in-round death tick plus exactly 1 second converted with the request tick rate; this does not use `killer_post_seconds`. If the tracked player survives, use the next round's `RoundStart` minus exactly 1 second when available, falling back to the parsed round-end tick only when no next round start exists. Do not use `RoundEndOfficial` as the normal cutoff.
- `take_plans[].view=full_round_pov` and a stable `source_id` distinguish full-round takes from kill-window takes and from each other.
- `ParseDemoFile.players[]` includes `steam_id_text`; frontend must use that string for full-round requests to avoid JavaScript number precision loss.
- Rounds where the tracked player did not get any kill are skipped — only kill-bearing rounds are turned into full-round POV takes.

### 4. Validation & Error Matrix

- Missing `demo_path` -> Go error `demo 路径为空`.
- Invalid or empty `full_round_pov.player_steam_id` -> Go error explaining the tracked SteamID is invalid.
- Demo parse failure -> wrapped Go error from `demo.ParseFullRoundPOVPlan`.
- Full-round POV requested but no valid live-round segments are found -> Go error `没有可导出的整局 POV 回合片段`.
- No full-round segments and no selected kill items -> Go error `没有可导出的录制片段`.
- Player has no kill clips -> frontend shows empty kill-list state; this is not an error if full-round POV is enabled.
- Tracked player has zero kills in all rounds -> backend returns empty segments; frontend prevents generation (start-produce disabled, empty-state warning shown).

### 5. Good/Base/Bad Cases

- Good: User enables full-round POV for one demo/player; selected old kill materials for that demo are cleared; produce generates one `full_round_pov` take per live round, followed by any newly selected victim takes.
- Good: A zero-kill player can still be selected from `players[]`; right-side kill list shows an empty state and produce still sends `full_round_pov.player_steam_id`.
- Base: Existing kill-window clip mode sends only `selected_items[]` and continues to generate killer/victim takes as before.
- Bad: Encoding full-round POV as synthetic `demo.ClipKill` rows. This mixes two different selection semantics and breaks future maintenance.
- Bad: Using numeric `players[].steam_id` in frontend request payloads. SteamID64 exceeds JavaScript safe integer range.
- Bad: Building produce history keys for full-round takes from empty `kill_ids` only. Multiple rounds would collapse to the same key.

### 6. Tests Required

- `internal/demo`: pure plan tests for round-start recording, target death end, next-round-start metadata for survival cutoff, and ignoring deaths outside the live `[RoundFreezetimeEnd, RoundEnd]` decision window.
- `internal/clipsjson`: builder test proving full-round POV takes precede victim takes and carry `source_id` / round metadata.
- `internal/plugingen`: history key tests proving distinct `source_id` values do not collide while legacy clip keys remain compatible.
- `internal/app`: conversion tests for full-round plan -> plugin segment target/tick/player metadata, including fixed 1-second death padding and next-round-start-minus-1-second survival cutoff.
- Required checks: `go test ./...` and `cd frontend && npm run build`.

### 7. Wrong vs Correct

#### Wrong

```ts
// Loses precision and cannot represent zero-kill player selection cleanly.
full_round_pov: { player_steam_id: String(player.steam_id) }
```

```go
// All full-round rounds share empty kill_ids, so history dedup collides.
key := plugingen.BuildProduceHistoryKey(plan.DemoPath, plan.View, plan.SpecMode, plan.KillIDs)
```

#### Correct

```ts
full_round_pov: { player_steam_id: player.steam_id_text }
```

```go
key := plugingen.BuildProduceHistoryKeyWithSourceID(
    plan.DemoPath,
    plan.View,
    plan.SpecMode,
    plan.KillIDs,
    plan.SourceID,
)
```

The full-round path remains independent from kill-window selections while still reusing shared low-level action generation and produce session plumbing.
