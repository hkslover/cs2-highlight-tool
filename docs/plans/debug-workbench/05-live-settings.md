# 05：快捷设置与调试草稿

前置：04。复用 `clip_settings.go`、`clipsjson/builder.go` 中的初始化命令和 `ffmpegprofile`，不要在 Vue 内重新拼接后端命令。

## 实施

1. 将适合共享的纯命令构建从 `buildBootstrapSequence` 提取为小型 helper：游戏设置命令、录制初始化命令分开；原 Build 使用 helper 并保持原 JSON 行为。调试不包含 `go_to_next_sequence`、tick、disconnect、自动录制边界。
2. 调试草稿独立于 settings 的自动保存草稿。初次读取设置成功后初始化，失败显示错误；不得把占位默认值保存到后端。增加“从正式设置重新加载”和“应用当前游戏设置”按钮，不新增永久配置文件。
3. 展示分组：启动资源、运行时画面/声音、下一段录制、正式工作流专用项。不要把每个配置字段都伪装为控制台命令。
4. 单项切换可发送一次对应命令；“应用全部”按确定顺序发 commands 数组。前端显示构建出的命令便于调试，自定义输入保持独立。
5. 正常制作关闭某些开关时不写反向命令的语义保持原样。debug 为测试开/关提供显式恢复命令，不修改正式 Build 的关闭行为。

## 设置分类与命令对照

| 设置 | 调试行为 |
| --- | --- |
| 插件、POV HUD、品牌 VPK、分辨率 | 启动前冻结；运行中改草稿显示需重启，不假装热加载 |
| POV 雷达 | 共享插件特殊入口 `csdm_radar_pov 1/0`；无插件禁用，与 POV HUD 独立 |
| 天空关闭 | `r_drawskybox 0`；debug 恢复用 `r_drawskybox 1` |
| 云层关闭 | `mirv_sky clouds draw 0`；debug 恢复 `mirv_sky clouds draw 1`；不联动天空 |
| 隐藏 UI | `cl_draw_only_deathnotices 1/0` |
| 隐藏玩家头像 | `cl_teamcounter_playercount_instead_of_avatars true/false` |
| 隐藏聊天 | 开启 `hidehud 128`；debug 关闭为 `hidehud 0`，UI 说明这会清掉其他 hidehud 位；首版不做 cvar 快照恢复 |
| X-ray | 复用 `xrayCommandValue` 与当前字段真实语义，不按字段名猜极性 |
| 语音 | `tv_listen_voice_indices`、`tv_listen_voice_indices_h` 两条同步，复用 `voiceIndicesValue` |
| 越肩 | 开启复用 `shoulderCameraCommand`；关闭使用经 Windows 验证的恢复组合，至少处理 `c_thirdpersonshoulder` 和 camera 模式，不只改 UI 布尔值 |
| 击杀信息持续时间 | `mirv_deathmsg lifetime <value>`，复用归一化范围 |
| 屏蔽击杀信息 | 开启沿用 `mirv_deathmsg filter add block 1`；关闭提供明确移除/清理方式，经 HLAE 命令实际验证，提示对当前过滤器的影响 |
| FPS、录制质量、video_preset | 下一段录制前应用，录制中冻结；auto 复用后端实际编码选择，不改成持久化硬件 ID |
| record_output_dir | 固定受管控 outputs/debug 子目录，不允许从草稿任意写盘 |
| edit_fps/edit_quality | 正式剪辑专用；调试合并不重编码，不提供无效控制台开关 |
| 前后秒数、自动添加 victim | 片段规划专用；手动录制不使用，只说明不适用 |

上表中的现有开启命令已从源码核对；新增加的恢复命令必须在对应 HLAE/CS2 版本实测。越肩与击杀过滤器恢复不能仅凭猜测标记完成：优先核对本地 HLAE 源码/帮助，再实机确认；若确实没有无损恢复，展示明确的重置按钮及影响，不静默失效。

## 验收

- 所有能热更的设置通过 03 通道到达游戏，快捷命令与自定义命令共用执行逻辑。
- 新资源选项不污染正式配置；brand=false 只影响 debug；正常制作 JSON 命令行为回归通过。
- 正式配置显式 false 保持原值，debug 草稿不触发 settings 保存防抖。
- 雷达首次开关/重复开启正常，天空与云层独立，语音两字段一致。
- 每个新增关闭动作有真实游戏验证；只测纯 helper 的关键语义，不为静态控件增加大量模板测试。
- 执行桌面端三项检查；有插件 helper 调整时重新 Windows 构建和联调。
