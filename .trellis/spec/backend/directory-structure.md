# Directory Structure

> How Go backend and Vue frontend code is organized in this project.

---

## Overview

This is a **Wails v2** desktop application with a Go backend and Vue 3 + TypeScript frontend. The project follows a clean-architecture-lite layering: Wails binding methods in `internal/app` are the only public surface exposed to the frontend; lower packages never import `internal/app` or `internal/envsetup`.

---

## Top-Level Layout

```
.
├── main.go                   # Wails bootstrap entry point
├── internal/                 # Go backend
│   ├── app/                  # Wails binding layer (UI-callable methods)
│   ├── appdata/              # App data root resolution and legacy exe-dir data migration
│   ├── clipsjson/            # HLAE plugin JSON builder from kill data
│   ├── config/               # config.json read/write, defaults, path normalization
│   ├── demo/                 # CS2 .dem parser (demoinfocs-golang v5)
│   ├── download/             # HTTP download with progress, zip/7z extraction
│   ├── endpoints/            # Source registry, GeoIP gh-proxy decisions, URL rewrites
│   ├── envsetup/             # Startup state machine (component detect/dl/install)
│   ├── ffmpegprofile/        # FFmpeg capability detection and encoder profiles
│   ├── fivee/                # 5EPlay API client: match listing, demo download/caching
│   ├── logging/              # Structured slog adapter plus shared support-export redaction
│   ├── plugingen/            # Plugin JSON generation helpers: history keys, subdirs, preset resolution, history filtering
│   ├── procutil/             # OS-specific process creation helpers (Windows/other)
│   ├── producegame/          # CS2 game environment helpers: gameinfo.gi path resolution and plugin search path injection
│   ├── producemerge/         # FFmpeg-based video/audio merge logic for produce session take files
│   ├── producews/            # Loopback WebSocket server, reconnect health, bounded diagnostics/reports
│   ├── release/              # Unified release API client, version compare, asset select
│   └── wanmei/               # Wanmei (完美) API client: match listing, demo download/caching
├── frontend/                 # Vue 3 + TypeScript frontend
│   └── src/
│       ├── app/              # Shell, router, TopBar, MainApp container
│       ├── features/         # Page components organized by domain
│       ├── shared/           # Cross-cutting utilities (i18n, types, state)
│       └── wailsjs/          # Auto-generated Go↔JS bindings (NEVER hand-edit)
├── docs/                     # Architecture documentation
├── tools/                    # Local mock release API and debug scripts
└── build/                    # Wails build artifacts
```

---

## Go Backend Package Organization

### `internal/app/` — Wails Binding Layer

- **Purpose**: Thin wrapper that exposes methods callable from the frontend via `window.go.app.App.*`.
- **No business logic** — delegates to `envsetup`, `clipsjson`, `demo`, `producews` etc.
- Files are named by domain with a `prefix_` convention:
  - `app_*` — top-level startup, demo import, app shell
  - `clip_*` / `plugin_*` / `hlae_*` — clip settings, plugin JSON generation, HLAE launch
  - `produce_*` — produce session lifecycle, take file history, merge queue, game config backup, cleanup
  - `edit_*` — clip concatenation, ffmpeg command execution, compose progress tracking
  - `cs2_process*` — CS2 process detection (with `_windows.go`/`_other.go` suffix)
- Platform-conditional files use the `_windows.go` / `_other.go` suffix convention (e.g., `cs2_process_windows.go`, `cs2_process_other.go`)

### `internal/envsetup/` — Startup State Machine

- **Purpose**: Orchestrates the startup wizard — component detection, download, installation, and self-update version detection (download/replace of the app binary itself is handled manually by the user via the browser).
- Split across multiple files by concern:
  - `service.go` — `Service` struct, constructor, `Startup()` lifecycle
  - `service_state.go` — State manipulation helpers (snapshot, step updating, config persistence)
  - `service_startup.go` — Main `RunStartupChecks()` entry point
  - `service_actions.go` — User-initiated actions (retry, reinstall, manual download, pick path)
  - `service_logs_export.go` — Log export to clipboard/file
  - `state.go` — Type definitions (`StartupState`, `ComponentStatus`, `LogMessage`, etc.)
  - `events.go` — Wails event emission (`startup_state_changed`, `download_progress`)
  - `logger_bridge.go` — Adapters between envsetup and `internal/logging`
  - `source.go` — GeoIP download source resolution
  - `cs2.go` — CS2 path resolution (Steam detection + manual pick)
  - `cs2_detect.go` / `cs2_detect_windows.go` / `cs2_detect_other.go` — Platform-specific Steam detection
  - `hlae.go` / `hlae_test.go` — HLAE component lifecycle + tests
  - `plugin.go` / `plugin_test.go` — Plugin component lifecycle + tests
  - `ffmpeg.go` / `ffmpeg_detect.go` / `ffmpeg_detect_test.go` — FFmpeg component lifecycle
  - `selfupdate.go` — Self-update check and apply
  - `release_fallback.go` / `release_fallback_test.go` — Release snapshot fallback behavior
  - `ads.go` / `ads_test.go` — Startup advertisement loading
  - `component_logging_test.go` — Logging integration tests
  - `log_test_helpers_test.go` — Test helper utilities
  - `source_test.go` — Source resolution tests
  - `source_fallback_logging_test.go` — Source fallback logging tests
  - `cs2_test.go` — CS2 path tests
  - `cs2_detect_test.go` — Steam detection tests

### `internal/appdata/` — Application Data Paths

- Resolves the app-managed data root. On Windows the default is `%LOCALAPPDATA%\CS2 Highlight Tool\`.
- Migrates known legacy app-managed files/directories from the executable directory into the data root without overwriting existing destination data.
- Does not own Wails/WebView2 browser cache paths.

### `internal/config/` — Configuration

- `config.go` — `Config` struct, `LoadOrCreate()`, `Save()`, `ApplyDefaults()`, validation
- `config_test.go` — Validation, defaults, range-constrained field tests

### `internal/release/` — Release API Client

- `resolver.go` — `FetchUnifiedLatest()`, asset selection, URL normalization
- `version.go` / `version_test.go` — Version string comparison
- `ads.go` / `ads_test.go` — Release API advertisement parsing and validation
- `resolver_test.go` — Release resolution tests

### `internal/endpoints/` — Download Source Registry

- `endpoints.go` / `endpoints_test.go` — Source whitelist, URL builders, manual download URL generation

### `internal/download/` — Download & Extraction

- `file.go` — HTTP download with progress callback
- `archive.go` — Zip/7z extraction, file/directory copy utilities (non-generic — each helper has a specific job)

### `internal/fivee/` — 5EPlay API Client

- `client.go` — `ListRecentMatches()`, `ImportDemo()`, `ExtractMatchID()`, `ProgressComponentID()`; injectable `HTTPRequestFn`, `DownloadFileFn`, `UnzipFn`, `FindFirstByExtFn`, `CopyFileFn` vars for test injection
- `client_test.go` — Unit tests for all exported functions

### `internal/wanmei/` — Wanmei (完美) API Client

- `client.go` — `ListRecentMatches()`, `ImportDemo()`, `ExtractNumericMatchID()`, `ProgressComponentID()`; injectable function vars for test injection; OSSResolve, PVP sign, and demo URL build helpers
- `client_test.go` — Unit tests for all exported functions

### `internal/plugingen/` — Plugin JSON Generation Helpers

- `helpers.go` — Pure utility functions used by `internal/app`: `BuildProduceHistoryKey()`, `SanitizeDemoSubDirName()`, `BuildBatchRecordSubDirs()`, `ResolvePluginVideoPreset()`
- `helpers_test.go` — Unit tests for all helpers
- `filter.go` — `TakePlan` type and `FilterItemsByHistory()` — filters clip items by produce history, keeping only items not yet recorded in the current session
- `filter_test.go` — Unit tests for `FilterItemsByHistory`

### `internal/producegame/` — CS2 Game Environment Helpers

- `gameinfo.go` — `ResolveGameInfoPath()`, `InjectPluginSearchPath()` — pure functions for locating and patching CS2's `gameinfo.gi` to enable plugin search paths
- `gameinfo_test.go` — Unit tests for both functions

### `internal/producemerge/` — FFmpeg Merge Logic

- `merge.go` — `MergeTakeVideoAudio()`, `WaitForTakeFilesReady()`, `NextMergedVideoPath()`, plus `FFmpegCommand`/`FFmpegCommandContext` injectable vars for test injection
- `merge_test.go` — Unit tests for merge, wait, and path-generation functions

### `internal/logging/` — Structured Logging

- `logger.go` — Interfaces (`Logger`, `Entry`, `Fields`, `StepToken`) and types
- Plus: `slog` adapter implementation (declared via `logging.NewSlogAdapter`)

---

## Frontend Organization

### `frontend/src/app/`

- `AppShell.vue` — Outer layout shell
- `TopBar.vue` — App navigation top bar
- `MainApp.vue` — Main content area after startup
- `router.ts` — vue-router hash-mode routes

### `frontend/src/features/`

One directory per business domain, each containing:
- `pages/` — Route-level page components
- `components/` — Domain-specific reusable components
- `composables/` — Composition API state/effect hooks

| Feature | Contents |
|---------|----------|
| `startup/` | `StartupWizard.vue`, `useStartupWizard.ts` |
| `import/` | `ImportPage.vue`, `WanmeiImport.vue`, `FiveEImport.vue`, `ImportActions.vue`, `ImportDemoList.vue`, `ImportDetailPanel.vue`, `useImportDemos.ts` |
| `clips/` | `ClipsPage.vue`, `DeathNoticeLine.vue` |
| `edit/` | `EditPage.vue`, `ClipLibrary.vue`, `Timeline.vue`, `TransitionPicker.vue`, `useEditState.ts` |
| `produce/` | `ProducePage.vue`, `useProducePageState.ts`, `useProduceHistory.ts` |
| `settings/` | `SettingsPage.vue`, `SettingsPanel.vue` |
| `ads/` | `MainTopBannerAds.vue`, `useMainTopBannerAds.ts` |

### `frontend/src/shared/`

- `types.ts` — All shared TypeScript interfaces (mirrors Go struct contracts)
- `events.ts` — Wails event subscription/emission constants
- `i18n/` — Internationalization
- `state/` — Cross-feature reactive state (`useDebugSettings.ts`)

### `frontend/src/auto-imports.d.ts`

Auto-generated by unplugin-auto-import — **never hand-edit**.

### `frontend/src/components.d.ts`

Auto-generated by unplugin-vue-components — **never hand-edit**.

### `frontend/wailsjs/`

Auto-generated by Wails from Go binding methods — **never hand-edit**.

---

## Dependency Direction

```
internal/app (top, Wails binding)
    ↑ delegates to
internal/envsetup, internal/clipsjson, internal/demo,
internal/producews, internal/config, internal/release,
internal/fivee, internal/wanmei, internal/plugingen,
internal/producegame, internal/producemerge
    ↑ imported by
internal/endpoints, internal/download, internal/logging,
internal/procutil (bottom)
```

Lower packages never import higher packages. `internal/app` never imports business logic directly — it delegates to service packages.

**Type-conversion wrapper convention**: when a lower-layer package cannot use `internal/app` types (to avoid circular imports), `internal/app` keeps a thin private wrapper that converts its own types before delegating:

```go
// In internal/app/plugin_generate.go:
func filterItemsByHistory(items []clipsjson.Item, plans []ProduceTakePlan, ...) []clipsjson.Item {
    return plugingen.FilterItemsByHistory(items, toPlugingenTakePlans(plans), ...)
}
func toPlugingenTakePlans(plans []ProduceTakePlan) []plugingen.TakePlan { ... }
```

The lower-layer package (`plugingen`) defines its own minimal type (`TakePlan`); the `app` layer maps to it. This keeps lower packages free of app-layer imports while preserving testability at both layers.

Phase 1 of the architecture refactoring (produce + plugin domain) added:
- `internal/producegame` — CS2 game file manipulation logic (gameinfo.gi path resolution and plugin search path injection)
- `internal/producemerge` — FFmpeg-based video/audio merge logic
- Extended `internal/plugingen` with `FilterItemsByHistory` and `TakePlan` type

Frontend: `app/` → `features/` → `shared/` (shared never depends on features or app).

---

## Naming Conventions

- **Go files**: `snake_case.go` matching the primary exported type or concern
- **Platform-conditional files**: `name_windows.go`, `name_other.go` (or `name_darwin.go`, `name_linux.go`)
- **Test files**: `name_test.go` alongside the source
- **Vue files**: `PascalCase.vue` (single-word names for simple components, multi-word for complex)
- **TypeScript files**: `camelCase.ts` (composables prefixed with `use`)
- **Go packages**: Single-word lowercase (`envsetup`, `endpoints`, `clipsjson`)

> **Exception**: `internal/procutil` uses snake_case in filename (`no_window_windows.go`, `no_window_other.go`) — consistent within the package.
