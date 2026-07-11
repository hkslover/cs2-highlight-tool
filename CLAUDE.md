# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Run/Test Commands

```bash
# Live development (hot-reload frontend + Go backend)
wails dev

# Production build
wails build

# Backend tests (all packages)
go test ./...

# Backend tests (specific packages — required for envsetup/release changes)
go test ./internal/envsetup ./internal/release

# Frontend type-check + build (required before any frontend change)
cd frontend && npm run build
```

## Tech Stack

Wails v2 + Go 1.24 + Vue 3 + TypeScript + Naive UI + vue-router@4 (hash mode).

## High-Level Architecture

```
main.go                          # Wails bootstrap
internal/
  app/                           # Wails binding layer — top of dep tree, no business logic
  envsetup/                      # Startup state machine (component detect/dl/install)
  producews/                     # WebSocket server (OS-assigned loopback port) for HLAE/CS2 game comm
  clipsjson/                     # HLAE plugin JSON builder from kill data
  demo/                          # CS2 .dem parser (demoinfocs-golang v5)
  config/                        # config.json read/write, defaults, path normalization
  release/                       # Unified release API client, version compare, asset select
  endpoints/                     # Source registry, GeoIP gh-proxy decisions, URL rewrites
  download/                      # HTTP download with progress, zip/7z extraction
  logging/                       # Structured slog adapter with sanitization
frontend/src/
  app/                           # Shell (AppShell.vue), router.ts, TopBar, MainApp
  features/startup/              # Startup wizard UI
  features/import/               # Demo import, parsing, material selection
  features/clips/                # Kill/clip selection page
  features/produce/              # Recording/queue management
  features/settings/             # Clip settings panel
  shared/                        # types.ts (all shared TS interfaces), i18n/
  wailsjs/                       # Auto-generated Go↔JS bindings (NEVER hand-edit)
```

**Dependency direction:** `app` → `envsetup`/`clipsjson`/`demo`/`producews`/`config`/`release`. Lower packages (`endpoints`, `download`, `logging`) never import `app`. Frontend: `app` → `features` → `shared` (shared never depends on features/app).

**Routes (hash mode):** `/` → redirects to `/import`; `/import` (with nested `/import/wanmei`, `/import/5e`); `/clips`; `/produce`; `/settings`.

## Hard Constraints

Do NOT hand-edit these auto-generated files:
- `frontend/wailsjs/**`
- `frontend/src/auto-imports.d.ts`
- `frontend/src/components.d.ts`

Immutable identifiers and enums (do NOT rename without explicit request):
- Component IDs: `hlae`, `plugin`, `ffmpeg`, `cs2`
- Statuses: `pending`, `checking`, `downloading`, `installing`, `ready`, `warning`, `failed`, `needs_action`
- Phases: `detecting_source`, `waiting_source`, `running_tasks`, `ready`
- Wails events: `startup_state_changed`, `download_progress`, `compose_progress`, `produce_ws_state_changed`, `produce_queue_state_changed`, `produce_take_status_changed`

i18n: Only modify `zh-CN.json`. The `en-US.json` file is maintained separately by the user.

Import alias: Use `@/` for `frontend/src/**` imports.

## Key Conventions

- **HLAE/plugin local version:** Must be read from installation directory's `changelog.xml` (first `<version>` element), not from persisted `config.json`.
- **Logging:** All startup logs go through `internal/logging` (slog adapter). Sensitive values (tokens, auth headers, home paths) are automatically sanitized. Trace fields: `component`, `stage`, `action`, `source`, `attempt`, `error`, `elapsed_ms`.
- **Unified release source:** A single release API snapshot is fetched at startup and consumed by all components. GitHub download URLs are rewritten via gh-proxy only when `country_code=CN`; non-CN goes direct. No multi-source fallback.
- **Concurrency:** Use existing `mu`/`configMu` mutexes for `Service.state`, `Service.logs`, `Service.config`. No blocking I/O inside locks. Always call `emitState()` after state mutation.
- **Wails binding boundary:** Frontend accesses all backend methods via `window.go.app.App.*` and receives push events via `runtime.EventsOn(...)`. New Wails-exposed methods are stable public contracts — do not rename without coordination.

## Stable Wails Public Methods (on `internal/app.App`)

Workspace: `GetWorkspaceState`, `PickWorkspaceDir`, `ValidateWorkspaceDir`, `SetWorkspaceDir`, `ResetWorkspace`, `ExitApp`
Startup: `GetStartupState`, `RunStartupChecks`, `RetryStartupComponent`, `ReinstallStartupComponent`, `OpenManualDownload`, `ImportManualDownload`, `PickCS2Path`, `EnterMainApp`, `ExportStartupLogs`
Demo: `PickDemoFiles`, `ParseDemoFile`
Clips: `GetClipSettings`, `SaveClipSettings`, `GeneratePluginJSON`, `GeneratePluginJSONBatch`, `GeneratePluginJSONBatchAndLaunchHLAE`
Produce: `GetProduceWSState`, `GetProduceQueueState`, `GetProduceTakeSnapshot`, `GetProduceHistorySnapshot`, `GetProduceTakeFiles`, `OpenProducedClipInFolder`, `ExportProduceHistoryVideos`
Material: `GetClipMode`, `SaveClipMode`, `SaveClipProject`, `LoadClipProject`, `PickMaterialDirectory`, `ScanMaterialClips`, `AutoMatchMaterials`, `ComposeClipProject`, `CancelCompose`, `GenerateMaterialPluginJSON`
