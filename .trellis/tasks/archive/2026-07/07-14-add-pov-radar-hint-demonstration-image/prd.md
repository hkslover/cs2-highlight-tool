# Add POV radar hint demonstration image

## Goal

在设置页面 POV 雷达功能的悬浮问号提示中展示一张功能演示图，帮助用户理解 beta 功能效果。

## Requirements

* 将工作区中的 `ScreenShot_2026-07-14_181548_695.png` 以语义化名称 `pov-radar-demo.png` 放入 `frontend/src/assets/images/`。
* 在 `SettingsPanel.vue` 中将该图片加入 POV 雷达的 `n-tooltip` 提示内容，并保留现有提示文字。
* 图片使用 Vite 资源导入，设置合理的展示尺寸与响应式约束，避免撑破提示框。
* 不新增后端接口，不改变 POV 雷达开关逻辑与现有 i18n 文案。

## Acceptance Criteria

* [x] POV 雷达设置行的问号悬浮提示同时显示提示文字和演示图片。
* [x] 图片可被前端构建正确解析并打包。
* [x] 图片名称与存放目录符合前端现有资源约定。
* [x] `cd frontend && npm run build` 通过。

## Definition of Done

* 前端类型检查与生产构建通过。
* 不修改自动生成文件。
* 保留用户工作区中已有的其他未提交改动。

## Technical Approach

复制并重命名图片到 `frontend/src/assets/images/pov-radar-demo.png`，在 `SettingsPanel.vue` 中通过静态资源 import 获取 URL，在带有 `hintKey` 的 tooltip 内针对 `pov_radar_enabled` 渲染带文字和图片的内容；其他提示继续只显示文本。

## Decision (ADR-lite)

**Context**: 当前提示只有 beta 风险说明，用户缺少对 POV 雷达效果的直观理解。

**Decision**: 使用前端静态资源导入图片，并只在 POV 雷达专属 tooltip 中展示。

**Consequences**: 图片随前端构建产物发布，无需运行时文件路径或后端支持；未来如需替换图片只需替换资源文件。

## Out of Scope

* 不修改 POV 雷达的实现、默认值或持久化逻辑。
* 不增加图片放大预览、点击交互或新的翻译键。
* 不调整其他设置项的提示样式。

## Technical Notes

* 已检查：`frontend/src/features/settings/components/SettingsPanel.vue` 使用 Naive UI `n-tooltip` 和 `<style scoped>`。
* 已检查：`frontend/src/assets/images/` 是现有静态图片资源目录，平台图标采用静态 import。
* 原始图片：`ScreenShot_2026-07-14_181548_695.png`，内容为 CS2 雷达示意图。
* 相关规范：`.trellis/spec/frontend/component-guidelines.md`、`.trellis/spec/frontend/quality-guidelines.md`。
