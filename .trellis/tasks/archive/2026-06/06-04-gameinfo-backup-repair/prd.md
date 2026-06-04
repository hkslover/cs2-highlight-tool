# Gameinfo backup and repair

## Goal

Prevent CS2 `gameinfo.gi` from staying modified after an abnormal tool exit by keeping a long-lived clean backup in the application data directory, showing the current gameinfo health in Settings, and providing a one-click repair action that restores the clean backup.

## What I Already Know

* The current produce flow injects `Game\tcsgo/plugin` into CS2 `gameinfo.gi`.
* The current session backup is stored next to the game file with suffix `.cs2ht_produce.bak` and is removed after normal restore.
* Abnormal exits can leave the game file injected and lose the in-memory session state needed for automatic restore.
* Settings are implemented in `frontend/src/features/settings/components/SettingsPanel.vue` and already use Wails-bound App methods for storage stats/actions.
* The app data root is available through `App.dataPath(...)`; it should be used for the long-lived backup.

## Requirements

* Add a long-lived clean `gameinfo.gi` backup under the application data directory.
* Ensure the long-lived backup never contains `Game\tcsgo/plugin`; do not overwrite a clean backup with an injected file.
* Attempt to create or refresh the long-lived clean backup when the app starts if CS2 `gameinfo.gi` is resolvable and currently clean.
* Also create the long-lived clean backup before injecting `Game\tcsgo/plugin` for a produce session.
* Add backend Wails methods for:
  * Reading current gameinfo health.
  * Restoring the current CS2 `gameinfo.gi` from the long-lived clean backup.
* Settings page shows:
  * A protection option/status area for gameinfo repair.
  * `配置文件正常` when current CS2 `gameinfo.gi` does not contain `Game\tcsgo/plugin`.
  * `配置文件异常` when current CS2 `gameinfo.gi` contains `Game\tcsgo/plugin`.
  * A repair button only when status is abnormal and a clean backup is available.
* Repair copies the long-lived clean backup into the current CS2 `gameinfo.gi` path, then refreshes status.
* Add or update tests for clean backup creation, avoiding injected backup overwrite, status detection, and repair.
* Update stable contract documentation because new Wails methods/types are added.

## Acceptance Criteria

* [ ] On app startup with a configured clean CS2 `gameinfo.gi`, a clean backup exists under the data directory.
* [ ] If current `gameinfo.gi` contains `Game\tcsgo/plugin`, startup/status logic does not overwrite the clean backup.
* [ ] Settings displays normal/abnormal status based on the actual current CS2 `gameinfo.gi`.
* [ ] When abnormal and backup exists, clicking repair restores the clean backup to the game directory.
* [ ] Repair is idempotent and reports a useful error if CS2 path or backup is missing.
* [ ] `go test ./...` passes.
* [ ] `cd frontend && npm run build` passes.

## Definition of Done

* Backend and frontend contracts stay aligned.
* Generated `frontend/wailsjs/**` files are not edited manually.
* `zh-CN.json` is updated for new UI strings; `en-US.json` is left untouched.
* Root and scoped `AGENTS.md` contract docs are updated for new Wails methods.

## Technical Approach

Recommended approach: keep the existing session backup/restore flow unchanged for normal produce lifecycle rollback, and add a separate persistent clean backup under `<dataDir>/backups/gameinfo/gameinfo.gi`. The persistent backup is only written from a currently clean gameinfo file. Settings calls explicit App methods to get a lightweight health object and to repair from that backup.

This keeps emergency repair independent from the temporary per-session backup and avoids restoring stale in-memory state after abnormal exits.

## Decision (ADR-lite)

**Context**: A session-local backup is correct for normal rollback but is not enough after process death.

**Decision**: Add a long-lived clean backup in app data, guarded against injected content, plus explicit Settings repair APIs.

**Consequences**: The repair feature depends on having seen a clean gameinfo at least once. If the first run sees an already injected file and no clean backup exists, the UI can detect the abnormal state but cannot safely auto-create a clean backup.

## Out of Scope

* Reconstructing Valve's default `gameinfo.gi` from a hardcoded template.
* Automatically modifying CS2 files without user action when an abnormal state is detected.
* Cleaning plugin DLL files left in `csgo/plugin/bin`; this task only covers `gameinfo.gi`.

## Technical Notes

* Relevant backend files: `internal/app/produce_gameconfig.go`, `internal/producegame/gameinfo.go`, `internal/app/app.go`.
* Relevant frontend files: `frontend/src/features/settings/components/SettingsPanel.vue`, `frontend/src/shared/types/clips.ts`, `frontend/src/shared/i18n/zh-CN.json`.
* Relevant tests: `internal/app/produce_session_test.go`, `internal/producegame/gameinfo_test.go`.
