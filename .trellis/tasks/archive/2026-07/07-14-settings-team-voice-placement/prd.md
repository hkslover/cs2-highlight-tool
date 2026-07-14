# 设置页启用队伍语音归入配置列表

## Goal

将设置页面中独立显示在片段配置区域的“启用队伍语音”开关移动到“配置”列表框内，使其与其他可搜索、分页的录制选项保持一致，同时保持现有设置值、自动保存和后端调用行为不变。

## What I already know

* 用户要求：设置页面中的“启用队伍语音”不再独立显示，移动到“配置”列表框中。
* 相关前端组件是 `frontend/src/features/settings/components/SettingsPanel.vue`。
* 当前开关位于片段配置卡片中，配置列表通过 `searchableClipSettings`、搜索和分页渲染。
* `ClipSettings.enable_voice` 已存在，后端契约无需改变。
* 相关中文文案 `main.settings.enable_voice` 已存在于 `frontend/src/shared/i18n/zh-CN.json`。
* 当前工作区已有用户/其他任务的未提交修改：`frontend/src/shared/i18n/zh-CN.json` 以及 `frontend/wailsjs/**`；本任务不应覆盖或手工修改自动生成文件。

## Assumptions

* “移动到配置列表框中”表示把该开关加入现有可搜索/分页的配置列表，而不是新增独立配置卡片。
* 继续使用现有 `enable_voice` 字段和统一的 `switchSettingValue` / `updateSwitchSetting` 逻辑。
* 列表中的显示顺序放在配置项开头，便于用户发现；不新增翻译键。

## Open Questions

* 无阻塞问题；需求和现有代码结构已足够明确。

## Requirements

* 移除片段配置区域中独立的 `enable_voice` 行。
* 将 `enable_voice` 加入 `SearchableSwitchSettingKey` 类型和 `searchableClipSettings` 列表。
* 列表项使用现有 `main.settings.enable_voice` 文案和开关渲染/更新逻辑。
* 保持搜索、分页、自动保存、加载设置和后端字段语义不变。
* 不修改 `frontend/wailsjs/**` 或 `en-US.json`。

## Acceptance Criteria

* [ ] 设置页不再在配置列表外单独显示“启用队伍语音”开关。
* [ ] “启用队伍语音”出现在“配置”列表中，并可以正常切换。
* [ ] 搜索“启用队伍语音”能够筛选出该配置项，分页行为正常。
* [ ] 开关变更仍通过现有自动保存流程保存到 `ClipSettings.enable_voice`。
* [ ] `cd frontend && npm run build` 通过。
* [ ] 创建包含本次变更的 git 提交记录，且不覆盖工作区已有的无关修改。

## Definition of Done

* 完成最小范围前端修改。
* 前端构建通过。
* 变更经过差异和状态检查。
* 创建清晰的 git 提交记录。

## Out of Scope

* 不改变后端接口、配置字段、默认值或录制语音逻辑。
* 不修改英文翻译文件。
* 不重构设置列表或分页组件。

## Technical Notes

* Spec: `.trellis/spec/frontend/component-guidelines.md`
* Spec: `.trellis/spec/frontend/i18n-guidelines.md`
* Spec: `.trellis/spec/frontend/quality-guidelines.md`
* Component: `frontend/src/features/settings/components/SettingsPanel.vue`
