# CLAUDE.md

Guidance for Claude Code in this repository.

**`AGENTS.md` is the source of truth.** The root file is imported below; `internal/AGENTS.md` and `frontend/AGENTS.md` add backend/frontend rules and are loaded through `internal/CLAUDE.md` / `frontend/CLAUDE.md` when you work in those trees. This file is only a quick index: if it disagrees with an `AGENTS.md`, the `AGENTS.md` wins. When a change touches contracts (Wails methods, fields, events, states, download strategy, directory roles), update the affected `AGENTS.md` files, not this one.

@AGENTS.md

## Commands

```bash
wails dev                      # live development
wails build                    # production build
wails generate module          # regenerate frontend/wailsjs after changing Go-exposed methods/models

go test ./...                  # required for any backend change
go vet ./...
go test ./internal/envsetup ./internal/release   # startup state machine / release / download fallback

cd frontend && npm run build   # vue-tsc + vite; required for any frontend change
cd frontend && npm test        # Node-native domain/adapter tests + TS contract checks
```

Cross-layer or contract changes need all three: `go test ./...`, `npm test`, `npm run build`. CI (`.github/workflows/ci.yml`) runs these on windows-latest. Tests and builds do not prove real Wails events, Vue interaction, or Windows HLAE/CS2 injection, recording and environment restore — report those as unverified.

## Tech Stack

Wails v2 + Go (version in `go.mod`) + Vue 3 + TypeScript (strict) + Naive UI + vue-router 4 (hash mode). Windows desktop tool: workspace init → startup environment → import → pick clips → produce → edit.

## Code Map

```
main.go                     Wails bootstrap
internal/
  app/                      Wails boundary + orchestration (public request/response types live here)
  appdata/                  workspace dir validation, Windows registry, legacy cleanup
  envsetup/                 startup state machine, self-update, HLAE/plugin/ffmpeg/CS2 checks & installs
  config/                   config.json defaults/normalization + per-workspace config.Store transactions
  release/ endpoints/ download/   release snapshot, download URL strategy, download/extract/atomic replace
  demo/                     demo parsing, kill facts, full-round POV
  wanmei/ fivee/            platform match lookup + demo download
  plugingen/ clipsjson/     side-effect-free generation planning + plugin JSON actions
  producews/                produce WebSocket, queue, take state, diagnostics
  producegame/ producemerge/  game env prepare/restore, take merge
  edit/                     edit planning, filter graphs, ffprobe/FFmpeg runners (no Wails)
  ffmpegprofile/ procutil/  encoder detection/fallback, process utils
  logging/ changelog/       structured slog + sanitization, embedded changelog
frontend/src/
  app/                      shell, navigation, router.ts
  features/                 workspace-init, startup, import, clips, produce, edit, settings, debug, ads, changelog
  domains/                  headless cross-page state (TS only, no SFC)
  shared/backend/           the only typed Wails call adapter
  shared/types/             hand-written TS contracts mirroring Go models
  shared/ui/, shared/i18n/  shared presentational components, locales
frontend/tests/             Node-native unit tests and type contract checks
frontend/wailsjs/           generated bindings
tools/hud-pov-vpk/          POV HUD VPK template and build
```

Routes (hash): `/` → `/import` (children `/import/wanmei`, `/import/5e`), `/clips`, `/produce`, `/edit`, `/settings`, `/debug` (only after debug mode is unlocked).

## Hard Constraints (summary — details in AGENTS.md)

- Never hand-edit `frontend/wailsjs/**`, `frontend/src/auto-imports.d.ts`, `frontend/src/components.d.ts`; change the Go source and regenerate.
- Do not rename or add to: component IDs `hlae|plugin|ffmpeg|cs2`; statuses `pending|checking|downloading|installing|ready|warning|failed|needs_action`; `mode` `workspace_init|startup|main`; `phase` `detecting_source|waiting_source|running_tasks|ready`.
- i18n: only edit `frontend/src/shared/i18n/zh-CN.json`; `en-US.json` is maintained by the user.
- Frontend calls the backend only through `@/shared/backend/` (no ad-hoc `callBackend`/`any`); import Vue APIs explicitly from `vue`; use `@/` for `src/**` imports.
- `<dataDir>` (user-chosen workspace) is distinct from `<exeDir>`; all components, config, demos, outputs and logs live under `<dataDir>`.
- Config reads/writes go through the shared `config.Store`; no blocking I/O, network or event emission inside `Service.mu` or the Store lock; call `emitState()` after startup state changes.
- HLAE/plugin local versions come from the install dir's `changelog.xml`, never from config.
- Do not restore `PROJECT_STRUCTURE.md`, and do not grow this file into a second rule book.

## Wails Events

`startup_state_changed`, `download_progress`, `log`, `produce_ws_state_changed`, `produce_queue_state_changed`, `produce_take_status_changed`, `produce_take_file_changed`, `produce_history_changed`, `compose_progress`, `debug_session_state_changed`. Frontend-local events (`clip-settings-saved`, `open-produce-history`) live in `frontend/src/shared/events.ts`.

Public Wails methods are stable contracts. Check the current list in `internal/app/` and the per-area entries in `AGENTS.md` rather than a copy here.
