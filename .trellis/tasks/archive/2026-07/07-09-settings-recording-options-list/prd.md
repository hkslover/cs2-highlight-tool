# Settings Recording Options Searchable List

## Goal

Move the growing group of recording/display-related clip settings into a compact list with search and controllable per-page count, so the settings page stays usable as more options are added.

## Requirements

* Keep existing `ClipSettings` fields, defaults, Wails methods, and auto-save behavior unchanged.
* Group these settings in a searchable paginated list: `enable_spec_show_xray_zero`, `hide_all_ui`, `use_shoulder_camera`, `pov_hud_enabled`, `sky_blackout`, `disable_clouds`, `kill_feed_lifetime`, `block_kill_feed`.
* Support fuzzy, case-insensitive search over the visible Chinese label and field key/aliases.
* Show 8 matching settings per page by default, without exposing a per-page selector or visible shown/total count.
* Reset/clamp pagination when the search query or page size changes.
* Only add Simplified Chinese i18n keys; do not edit `en-US.json`.

## Acceptance Criteria

* [x] The listed settings no longer appear as a long uninterrupted block in the clip settings card.
* [x] The list shows a search box and pagination when more than 8 matching settings exist.
* [x] Switches and numeric input still bind to the same `settings` fields and trigger existing auto-save.
* [x] Searching by Chinese labels or technical keywords like `xray`, `hud`, `cloud`, and `kill` returns the expected rows.
* [x] Empty search results render a clear empty state.
* [x] `cd frontend && npm run build` passes.

## Definition of Done

* Frontend code follows existing Vue 3 + Naive UI patterns.
* No generated files are manually edited.
* Required frontend build passes.

## Technical Approach

Implement the grouping inside `frontend/src/features/settings/components/SettingsPanel.vue` using local typed metadata for the searchable settings. Derive filtered and paginated rows with `computed`, bind controls through typed update handlers, and add scoped styles for the list toolbar, frame, rows, and pagination. Add only the necessary `main.settings.*` keys to `frontend/src/shared/i18n/zh-CN.json`.

## Out of Scope

* Backend field or contract changes.
* New setting fields.
* Editing `frontend/src/shared/i18n/en-US.json`.
* Introducing frontend test infrastructure.

## Technical Notes

* Root `AGENTS.md` and `frontend/AGENTS.md` require frontend changes to run `cd frontend && npm run build`.
* `.trellis/spec/frontend/quality-guidelines.md` says this frontend currently has no test infrastructure; validation is type-check plus Vite build.
* Existing dirty changes in `SettingsPanel.vue` and `zh-CN.json` include debug plugin DLL UI text and handlers; preserve them.
