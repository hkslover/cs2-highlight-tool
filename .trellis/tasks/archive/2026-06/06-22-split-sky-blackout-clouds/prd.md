# 拆分天空变黑与关闭云层选项

## Goal

将当前由“天空变黑”同时控制的两条 HLAE/CS2 指令拆分为两个独立持久化开关，使用户可以分别控制天空盒和云层。

## What I already know

* “天空变黑”只应在开启时写入 `r_drawskybox 0`。
* 新增“关闭云层”选项，只在开启时写入 `mirv_sky clouds draw 0`。
* 两个开关必须互相独立，任一组合都不应隐式开启另一条指令。
* 前端使用 `ClipSettings` 和 `SettingsPanel.vue` 编辑/保存设置，`ClipsPage.vue` 另有一份默认设置。
* 只修改 `zh-CN.json`，不修改 `en-US.json`。

## Requirements

* `sky_blackout=true` 只生成 `r_drawskybox 0`。
* `disable_clouds=true` 只生成 `mirv_sky clouds draw 0`。
* 开关为 `false` 时不生成对应指令。
* `disable_clouds` 对新安装和不包含该字段的旧配置均默认为 `false`。
* 新开关通过 `config.json` 持久化，并通过现有 `GetClipSettings` / `SaveClipSettings` 前后端契约读写。
* 设置面板在“天空变黑”附近展示“关闭云层”开关。
* 新持久化字段使用 `disable_clouds`，Go 字段使用 `DisableClouds`。
* 不增加新 Wails 方法或事件，只扩展现有 `ClipSettings` 合同。

## Acceptance Criteria

* [x] 只开启天空变黑时，bootstrap 包含 `r_drawskybox 0` 且不包含 `mirv_sky clouds draw 0`。
* [x] 只开启关闭云层时，bootstrap 包含 `mirv_sky clouds draw 0` 且不包含 `r_drawskybox 0`。
* [x] 两者都关闭时，bootstrap 不包含上述任一指令。
* [x] 新设置可通过 UI 编辑并经现有设置 API 持久化。
* [x] `go test ./...` 通过。
* [x] `cd frontend && npm run build` 通过。

## Definition of Done

* 先添加/修改失败测试，再实现后端行为。
* 前后端字段、默认值与持久化语义一致。
* 同步更新根级与前端 AGENTS 的稳定契约说明。

## Technical Approach

在现有 `ClipSettings` 端到端数据流中新增 `disable_clouds` 布尔字段，不引入新 API。`clipsjson` bootstrap 构建器对 `SkyBlackout` 和 `DisableClouds` 分别判断，测试覆盖独立开启与全部关闭。前端在现有设置面板中增加一个开关，默认值与后端均为 `false`。

## Decision (ADR-lite)

**Context**: 旧行为将两条语义不同的指令绑在“天空变黑”开关下。  
**Decision**: 新增独立 `disable_clouds` 开关且默认关闭；`sky_blackout` 不再生成关闭云层指令。  
**Consequences**: 升级后旧用户不会自动关闭云层，需要时可在设置中主动开启。

## Out of Scope (explicit)

* 不新增恢复天空盒/云层的反向指令。
* 不调整其他 HUD、X 光、击杀信息或录制设置。
* 不手工修改 `frontend/wailsjs/**` 自动生成文件。

## Technical Notes

* 行为实现：`internal/clipsjson/builder.go`。
* 现有行为测试：`internal/clipsjson/builder_test.go`。
* 配置持久化：`internal/config/config.go` 与 `internal/config/config_test.go`。
* Wails 设置合同：`internal/app/clip_settings.go`。
* 构建请求组装：`internal/app/plugin_generate.go`。
* 前端类型/状态/UI：`frontend/src/shared/types/clips.ts`、`frontend/src/features/clips/pages/ClipsPage.vue`、`frontend/src/features/settings/components/SettingsPanel.vue`。
