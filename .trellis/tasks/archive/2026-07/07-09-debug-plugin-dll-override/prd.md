# debug: custom plugin dll override

## Goal

Allow debug mode users to choose a custom CS2 plugin DLL for the current app session so recording/produce flows inject that DLL into `game/csgo/plugin/bin/server.dll` instead of the installed component DLL.

## Requirements

* Debug mode remains activated by repeated title clicks.
* The settings panel shows a separate `debug` section only when debug mode is active.
* Debug-only settings are not mixed into normal clip/record/edit settings.
* The existing debug `keep_intermediate_files` toggle stays under the debug section.
* Add a debug-only custom plugin DLL picker.
* The selected DLL is session-scoped and does not change `config.json` or startup component version checks.
* Produce launch uses the custom DLL as the source for `csgo/plugin/bin/server.dll` when the override is set.
* Clearing the override returns produce launch to the installed plugin DLL.
* The backend validates that the selected override is an existing `.dll` file.

## Acceptance Criteria

* [x] A Go test proves `preparePluginDLLForProduce` uses the debug override bytes instead of `config.PluginDLL`.
* [x] A Go test proves clearing the override returns to the configured plugin DLL.
* [x] Frontend build passes with the new debug settings UI.
* [x] Backend tests pass for changed packages.

## Definition of Done

* Tests added/updated where appropriate.
* `go test ./...` passes.
* `cd frontend && npm run build` passes.
* Stable Wails method additions are documented in `AGENTS.md`.

## Technical Approach

Add session-only state on `internal/app.App` protected by a mutex. Expose Wails methods to get, pick, set, and clear the override. Update `preparePluginDLLForProduce` to resolve its source DLL through the override first, then fall back to `config.PluginDLL`.

The frontend will reuse `useDebugSettings` for shared debug state and add a compact row in `SettingsPanel.vue` under the existing debug card. The UI will call the new backend methods through the existing `callBackend` helper pattern.

## Decision (ADR-lite)

**Context**: Persisting the custom DLL through `config.PluginDLL` would interfere with startup plugin component checks, because installed plugin version is derived from `plugin/changelog.xml` next to the configured `server.dll`.

**Decision**: Keep the custom DLL override session-only in `internal/app.App`.

**Consequences**: Debug testing is isolated from normal startup/update behavior. The selected DLL must be reselected after restarting the app.

## Out of Scope

* Persisting the custom DLL path across app restarts.
* Replacing the installed plugin component in `<dataDir>/plugin`.
* Editing Wails generated files by hand.

## Technical Notes

* Root `AGENTS.md`, `internal/AGENTS.md`, and `frontend/AGENTS.md` were read before implementation.
* Existing source injection path: `internal/app/produce_gameconfig.go`.
* Existing debug state/UI: `frontend/src/shared/state/useDebugSettings.ts` and `frontend/src/features/settings/components/SettingsPanel.vue`.
* Existing debug produce request option: `debug.keep_intermediate_files`.
