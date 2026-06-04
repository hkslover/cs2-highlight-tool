# Wails Bindings

Concrete contracts for Go methods exposed through `internal/app.App` and consumed by `window.go.app.App.*`.

## Scenario: CS2 Gameinfo Clean Backup and Repair Contract

### 1. Scope / Trigger

- Trigger: Produce mode temporarily injects `csgo/plugin` into CS2 `gameinfo.gi`, and abnormal app exits can leave the file modified.
- Scope: `internal/app` Wails bindings, `internal/producegame` gameinfo helpers, app data backup storage, and Settings UI status/repair actions.
- Boundary: App startup / produce preparation -> clean backup creation; Settings UI -> `GetGameInfoStatus()` / `RepairGameInfoFromBackup()` -> current CS2 `gameinfo.gi`.

### 2. Signatures

```go
func (a *App) GetGameInfoStatus() (*GameInfoStatus, error)
func (a *App) RepairGameInfoFromBackup() (*GameInfoStatus, error)

type GameInfoStatus struct {
    Status       string `json:"status"`
    Clean        bool   `json:"clean"`
    HasBackup    bool   `json:"has_backup"`
    CanRepair    bool   `json:"can_repair"`
    GameInfoPath string `json:"gameinfo_path,omitempty"`
    BackupPath   string `json:"backup_path,omitempty"`
    Message      string `json:"message,omitempty"`
}
```

Frontend shared type:

```ts
status: "normal" | "abnormal" | "unavailable";
```

### 3. Contracts

- Long-lived clean backup path is `<dataDir>/backups/gameinfo/gameinfo.gi`.
- Backup creation may run on app startup and before produce injection.
- A backup may be created or refreshed only from a current `gameinfo.gi` that does not contain `csgo/plugin`.
- Current status values:
  - `normal`: current `gameinfo.gi` is found and does not contain `csgo/plugin`.
  - `abnormal`: current `gameinfo.gi` is found and contains `csgo/plugin`.
  - `unavailable`: current CS2/gameinfo path cannot be resolved; this is non-fatal for status display.
- `can_repair=true` only when current status is `abnormal` and a clean long-lived backup exists.
- Repair must copy the long-lived backup to the current CS2 `gameinfo.gi` path, then return a fresh status snapshot.
- Existing per-session `.cs2ht_produce.bak` rollback stays unchanged and is still used for normal produce lifecycle cleanup.

### 4. Validation & Error Matrix

- CS2 path/gameinfo unresolved during status -> return `status=unavailable`, not an error.
- CS2 path/gameinfo unresolved during repair -> return an error because there is no target to restore.
- Current gameinfo contains `csgo/plugin` during backup creation -> do not write or overwrite long-lived backup.
- Backup file missing during repair -> return `未找到可用于修复的 gameinfo 原版备份`.
- Backup file contains `csgo/plugin` during repair -> return `gameinfo 原版备份包含 csgo/plugin，无法用于修复`.
- Filesystem copy/read failure -> wrap with Chinese user-facing context and `%w`.

### 5. Good/Base/Bad Cases

- Good: first startup after CS2 is configured and gameinfo is clean writes `<dataDir>/backups/gameinfo/gameinfo.gi`.
- Good: CS2 updates a clean gameinfo file; the next backup pass refreshes the long-lived backup from the new clean file.
- Base: produce session still writes and cleans `.cs2ht_produce.bak` for in-session rollback.
- Bad: abnormal leftover `Game\tcsgo/plugin` overwrites the long-lived clean backup.
- Bad: repair uses a hardcoded Valve template instead of the user's clean backup.

### 6. Tests Required

- `internal/app`: clean current file creates long-lived backup under the data dir.
- `internal/app`: injected current file does not overwrite an existing clean backup.
- `internal/app`: clean current file refreshes an existing backup.
- `internal/app`: status reports abnormal and repair restores the clean backup.
- `internal/app`: repair rejects a backup containing `csgo/plugin`.
- `go test ./...` and `cd frontend && npm run build` must pass after adding the Wails methods and Settings UI calls.

### 7. Wrong vs Correct

#### Wrong

```go
if strings.Contains(currentGameInfo, "csgo/plugin") {
    _ = copyFile(gameInfoPath, cleanBackupPath)
}
```

This preserves the already-injected state as the "original" and makes the repair button restore the broken file.

#### Correct

```go
if producegame.HasPluginSearchPath(currentGameInfo) {
    return nil
}
return copyFileWithReplace(gameInfoPath, cleanBackupPath)
```

Only clean current files are allowed to create or refresh the long-lived repair backup.

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
  - `4:3`: launch with `-w 1440 -h 1080`.
  - `4:3_1280x960`: launch with `-w 1280 -h 960`.
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
- Bad: changing the meaning of `4:3` to `1280x960`, which silently alters existing user configs.

### 6. Tests Required

- `internal/config`: `ApplyDefaults` preserves every supported `launch_resolution` value.
- `internal/app`: `SaveClipSettings` and `GetClipSettings` round-trip each supported launch resolution value.
- `internal/app`: `buildHLAECommandLine` maps `4:3` to `1440x1080`, maps `4:3_1280x960` to `1280x960`, and leaves `16:9` without 4:3 dimensions.
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
