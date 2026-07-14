# Database Guidelines

> Data persistence conventions in this project.

---

## No Database

This project **does not use a database**. There is no SQL, no ORM, no migration tool, and no persistent query layer.

All persistent state is stored in the app-managed data root (`dataDir`). On Windows the default data root is `%LOCALAPPDATA%\CS2 Highlight Tool\`.

1. **`config.json`** — a single JSON file in the application's data directory
2. **`.clipproject.json`** files — per-demo project files stored in `<dataDir>/projects/`
3. **File system** — raw demo files, extracted materials, produced clips

---

## `config.json` Persistence

### Location

```go
configPath := filepath.Join(dataDir, "config.json")
```

### Loading Pattern

Use `config.LoadOrCreate(path, dataDir)` — it loads existing config or creates a new one with defaults:

```go
cfg, err := config.LoadOrCreate(s.configPath, s.dataDir)
if err != nil {
    cfg = config.Default(s.dataDir)
    s.emitLog("error", fmt.Sprintf("加载配置失败: %v", err))
}
```

### Saving Pattern

Config writes flow through `persistConfig` (safe concurrent write pattern):

```go
func (s *Service) persistConfig(mutate func(*config.Config) error) (*config.Config, error) {
    s.configMu.Lock()
    defer s.configMu.Unlock()

    cfg, err := config.LoadOrCreate(s.configPath, s.dataDir)
    if err != nil {
        return nil, err
    }
    if mutate != nil {
        if err := mutate(cfg); err != nil {
            return nil, err
        }
    }
    if err := config.Save(s.configPath, cfg); err != nil {
        return nil, err
    }
    s.mu.Lock()
    s.config = cfg
    s.mu.Unlock()
    s.updateConfig(cfg)
    return cfg, nil
}
```

Key aspects:
- **Mutator callback pattern**: the `mutate` function receives the config and modifies it. This keeps read-modify-write atomic at the callback level.
- **Double mutex**: `configMu` protects disk I/O; `mu` protects in-memory `Service.config` reference. Never hold both at the same time.
- **Always reads from disk**, not from the in-memory cache, to avoid stale writes.

### Field Validation

All config field validation happens in `ApplyDefaults()` — called on every load:

```go
func ApplyDefaults(cfg *Config, dataDir string) {
    // Path normalization
    cfg.HLAEExe = CleanPath(cfg.HLAEExe)

    // Range clamping for numeric fields
    if cfg.EditFPS < MinEditFPS { cfg.EditFPS = MinEditFPS }
    if cfg.EditFPS > MaxEditFPS { cfg.EditFPS = MaxEditFPS }

    // Enum validation
    cfg.EditQuality = strings.ToLower(strings.TrimSpace(cfg.EditQuality))
    if !isSupportedEditQuality(cfg.EditQuality) {
        cfg.EditQuality = DefaultEditQuality
    }
}
```

**Rule**: Do not add a new config field without also updating `ApplyDefaults()`.

---

## Scenario: Atomic App-level config updates

### 1. Scope / Trigger

This applies whenever a Wails-bound method in `internal/app` reads and changes
`config.json`, especially settings, changelog acknowledgements, and 5E query
state. Concurrent Wails calls must not overwrite one another or expose a
partially written JSON file.

### 2. Signatures

```go
func (a *App) loadConfig() (*config.Config, error)
func (a *App) updateConfig(func(*config.Config) error) (*config.Config, error)
func config.LoadOrCreate(path, dataDir string) (*Config, error)
func config.Save(path string, cfg *Config) error
```

### 3. Contracts

- `App.loadConfig` and `App.updateConfig` serialize access with `App.configMu`.
- Every App read-modify-write uses `updateConfig`; callers must not pair a
  direct `LoadOrCreate` with a later `Save`.
- `LoadOrCreate` only writes after creation or when defaults/normalization/
  field migration changes the effective config.
- `Save` writes a temp file beside the target, flushes and closes it, then
  replaces the target with `os.Rename`; the target is never truncated in
  place.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Config is missing | Create defaults and return the config |
| Config is normalized and complete | Return without rewriting the file |
| Defaults or a managed field is missing | Apply defaults and persist migration |
| Temp write/sync/rename fails | Return a wrapped error and leave the existing target intact |

### 5. Good/Base/Bad Cases

- Good: two App updates serialize and the final config contains both changes.
- Base: a read-only settings query does not rewrite an already normalized file.
- Bad: call `LoadOrCreate`, unlock, mutate, and call `Save` from a Wails method;
  the second caller can overwrite the first caller's snapshot.

### 6. Tests Required

- `internal/config`: normalized config does not change its file timestamp;
  concurrent `Save` calls leave valid JSON; legacy fields are persisted.
- `internal/app`: two blocked `updateConfig` callbacks execute serially and
  preserve both mutations.

### 7. Wrong vs Correct

#### Wrong

```go
cfg, _ := config.LoadOrCreate(path, dataDir)
cfg.FiveEPlayerName = name
config.Save(path, cfg)
```

#### Correct

```go
_, err := a.updateConfig(func(cfg *config.Config) error {
    cfg.FiveEPlayerName = name
    return nil
})
```

## Config Fields Convention

```go
type Config struct {
    CS2Dir          string  `json:"cs2_dir"`
    CS2Exe          string  `json:"cs2_exe"`
    HLAEExe         string  `json:"hlae_exe"`
    PluginDLL       string  `json:"plugin_dll"`
    FFmpegDir       string  `json:"ffmpeg_dir"`
    FiveEPlayerName string  `json:"fivee_player_name"`
    DownloadSource  string  `json:"download_source"`
    CountryCode     string  `json:"country_code"`
    // ... clip settings, FFmpeg detection cache, etc.
}
```

- **snake_case JSON tags** always
- **Omit empty fields** use `omitempty` where appropriate (e.g., cached detection results)
- **No nested structs** except `ClipActionSettings` (pointer struct with `omitempty`)
- **Do not store computed or transient state** in config — only persisted user preferences and detection caches

---

## Per-Demo Project Files

Clip edit projects are saved per-demo as JSON:

```go
// Location: <dataDir>/projects/<demo_basename>.clipproject.json
```

The project file stores the full timeline, material references, transitions, and edit settings for a single demo file. Handled via `SaveClipProject` / `LoadClipProject` in the `app` layer.

---

## File System as Storage

Demo files, material videos, and produced clips are stored on the filesystem:

| Data | Path | Format |
|------|------|--------|
| Raw demos | `<dataDir>/demo/raw/` | `.dem` |
| Imported (Wanmei) demos | `<dataDir>/demo/wanmei/<matchID>/` | `.dem` |
| Imported (5E) demos | `<dataDir>/demo/5e/<matchID>/` | `.dem` |
| Material clips | User-chosen directory, scanned by `ScanMaterialClips()` | `.mp4`, `.mov`, `.mkv`, `.avi` |
| Productions | `<dataDir>/outputs/` | `.mp4` |
| Temp files | `<dataDir>/temp/` | Various |

---

## Scenario: App Data Root Migration

### 1. Scope / Trigger

- Trigger: app-managed runtime data must not be written next to the executable by default.
- Applies to config, startup components, imported demos, projects, output videos, temp update assets, and exported logs.

### 2. Signatures

- `appdata.Resolve(exeDir string) appdata.Paths`
- `appdata.MigrateLegacyData(exeDir string, dataDir string) error`
- `envsetup.NewWithDataDir(exeDir string, dataDir string, version string) *Service`
- `config.LoadOrCreate(path string, dataDir string) (*Config, error)`

### 3. Contracts

- On Windows, default `dataDir` is `%LOCALAPPDATA%\CS2 Highlight Tool\`.
- `exeDir` is only for locating/replacing the executable; do not use it as the business data root.
- `config.json`, `hlae/`, `plugin/`, `ffmpeg/`, `demo/`, `projects/`, `outputs/`, `temp/`, `updates/`, and `logs/` belong under `dataDir`.
- Wails/WebView2 user-data cache is not covered by this contract unless `WebviewUserDataPath` is explicitly configured.

### 4. Validation & Error Matrix

- Empty `dataDir` in service construction -> fall back to `exeDir` for test/backward-compatible callers.
- Destination exists during migration -> skip that entry, never overwrite it.
- Source missing during migration -> skip that entry.
- Migration copy/move failure -> return a wrapped error and leave the source data intact.

### 5. Good/Base/Bad Cases

- Good: downloaded components install to `<dataDir>/hlae`, `<dataDir>/plugin`, and `<dataDir>/ffmpeg`.
- Base: tests that call `envsetup.New(exeDir, version)` keep using `exeDir` as `dataDir`.
- Bad: `internal/app` or `internal/envsetup` writes `config.json`, demos, temp files, or component folders through `exeDir`.

### 6. Tests Required

- `internal/appdata`: data-root resolution and non-overwriting migration behavior.
- `internal/envsetup`: `NewWithDataDir` sets config and component paths under `dataDir`.
- `internal/app`: managed demo import paths use `dataDir`.

### 7. Wrong vs Correct

#### Wrong

```go
cfg, err := config.LoadOrCreate(filepath.Join(a.exeDir, "config.json"), a.exeDir)
rawRoot := filepath.Join(a.exeDir, "demo", "raw")
```

#### Correct

```go
cfg, err := config.LoadOrCreate(a.configPath(), a.dataRoot())
rawRoot := a.dataPath("demo", "raw")
```

---

## What Not to Do

- ❌ **No SQL/ORM/query builder** — not needed for this desktop app
- ❌ **No migration framework** — config versioning is handled by `ApplyDefaults()` which adds missing fields
- ❌ **No concurrent write without mutex** — use `configMu` for config.json writes
- ❌ **No config write inside a `stateMu` lock** — avoid lock ordering issues
