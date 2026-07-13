# feat: add POV radar toggle

## Goal

Expose a persisted `启用 POV雷达` switch in the recording configuration list. When enabled, generated plugin JSON bootstrap actions must contain `csdm_radar_pov 1`; when disabled, that command must not be generated.

## What I already know

* Settings are persisted through `internal/config.Config`, exposed through `internal/app.GetClipSettings` / `SaveClipSettings`, and consumed by `internal/clipsjson.Build`.
* The settings list in `frontend/src/features/settings/components/SettingsPanel.vue` uses a shared searchable item definition and fixed eight-item pagination.
* Existing `PovHudEnabled` controls the POV HUD VPK/gameinfo lifecycle and is a separate feature; the radar switch must not change that behavior.
* Existing bootstrap commands are emitted by `internal/clipsjson.buildBootstrapSequence` at `DefaultActionTick`.
* The project maintains only `frontend/src/shared/i18n/zh-CN.json` for AI i18n changes.

## Assumptions

* The new setting is opt-in and defaults to `false`, because it is explicitly a beta feature and must not alter existing recordings for users who do not enable it.
* The persisted field and Wails/frontend field will be named `pov_radar_enabled` / `PovRadarEnabled`.
* The hover help text will state that this is a beta feature, may break after game updates, and should be disabled if the game crashes.

## Requirements

* Add `pov_radar_enabled` to config defaults, load/save behavior, Wails clip settings, plugin generation, and the frontend `ClipSettings` type/default state.
* Add `启用 POV雷达` to the recording configuration list.
* Render a small `?` tooltip trigger next to this option with the beta/crash warning.
* When enabled, append exactly `Action{Cmd: "csdm_radar_pov 1", Tick: actionTick}` to the bootstrap actions.
* When disabled, do not emit `csdm_radar_pov` or a reset command.
* Add regression tests covering default/persistence and bootstrap enabled/disabled behavior.
* Update the stable contract/rules documentation for the new clip setting.

## Acceptance Criteria

* [ ] Existing configs without the field load with `pov_radar_enabled=false` and save the field.
* [ ] `GetClipSettings` / `SaveClipSettings` round-trip the setting.
* [ ] `BuildOptions.PovRadarEnabled=true` emits `csdm_radar_pov 1` in the bootstrap.
* [ ] `BuildOptions.PovRadarEnabled=false` emits no `csdm_radar_pov` command.
* [ ] Settings UI shows the label, switch, and hover help text.
* [ ] `go test ./...` passes.
* [ ] `cd frontend && npm run build` passes.

## Definition of Done

* Tests added/updated for backend persistence and action generation.
* Backend and frontend build checks are green.
* Contract docs are updated without editing generated Wails files or `en-US.json`.

## Out of Scope

* No changes to the standalone `cs2-server-plugin` repository.
* No changes to `PovHudEnabled`, `pov.vpk`, or `gameinfo.gi` handling.
* No per-clip override for the radar setting.

## Technical Notes

* Relevant files: `internal/config/config.go`, `internal/app/clip_settings.go`, `internal/app/plugin_generate.go`, `internal/clipsjson/builder.go`, `frontend/src/features/settings/components/SettingsPanel.vue`, `frontend/src/shared/types/clips.ts`, `frontend/src/shared/i18n/zh-CN.json`, and their existing tests.
* UI tooltip pattern already exists in `frontend/src/features/startup/components/StartupWizard.vue` using `n-tooltip` and a `.hint-dot` trigger.
