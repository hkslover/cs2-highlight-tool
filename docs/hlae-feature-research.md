# 功能现状与 HLAE 高级能力落地调研

> 本文是**调研与规划文档**，不是接口契约。方法签名、字段、事件、状态语义仍以 `AGENTS.md`、`internal/AGENTS.md`、`frontend/AGENTS.md` 与源码为准；本文提到的 HLAE 行为以官方 wiki 与 `AfxHookSource2` changelog 为准，**上机行为必须另行在 Windows 实机验证**。

## 0. 结论来源与标注约定

| 来源 | 说明 |
| --- | --- |
| 源码 | `internal/`、`frontend/src/` 当前实现 |
| HLAE wiki | 本地副本 `advancedfx.wiki/`（`Source2:*` 为 CS2 页面，部分页面转发到 CS:GO 页面） |
| HLAE changelog | `resources/AfxHookSource2_changelog.xml`（读到 AfxHookSource2 0.41.8 条目） |
| 社区 cfg | `abandonedpools/hlae-cfg-cs2`（CS2 录制参数的事实参考） |

标注约定：

- ✅ **源码核实**：当前仓库里能直接看到实现或缺失。
- 📖 **文档核实**：HLAE 官方文档/changelog 明确写了的能力与参数。
- ⚠️ **需实机验证**：涉及游戏内行为、版本差异、路径展开、合并流程的部分。

---

## 1. 当前能力盘点（✅ 源码核实）

### 1.1 环境准备与工作目录

| 能力 | 实际作用 |
| --- | --- |
| 启动状态机（`internal/envsetup`） | 把「能不能开始玩」拆成 `pending/checking/downloading/installing/ready/warning/failed/needs_action`，UI 只需读快照，不猜状态 |
| 组件安装事务（`ReplaceDirWithContents`） | staging → backup → rename 三段提交，安装失败不会把旧 HLAE/FFmpeg 弄坏；备份清理失败只告警 |
| 自更新优先 | 自更新可用时**先更新软件再检查组件**，避免用旧客户端写新协议 |
| 区域化下载 | CN 时 `mirror_url`/`url` 竞速、非 CN 只走 GitHub；统一源失败可回退「本地已装版本 + warning」，不伪造远端成功 |
| 工作目录会话（`workspace_session.go`） | 每个工作目录一个 generation + 一个 `config.Store`；Reset/Shutdown 先关准入再取消任务，旧实例不能往新目录写文件 |

### 1.2 Demo 导入

| 能力 | 实际作用 |
| --- | --- |
| 本地 / 完美世界 / 5E 三源 | 免去手动找 `.dem`；平台导入产物落在 `<dataDir>/demo/<平台>/<matchID>/` |
| 在途任务合并（`import_coordinator.go`） | 同一 `(平台, matchID)` 并发只下载一次，多个入口同时点不会重复解压 |
| 原子提交 | 先写同目录临时文件再 rename，失败只删本次临时文件，不会留下半个 demo |

### 1.3 Demo 解析产出的事实（`internal/demo`）

这是后面一切"自动选片/自动运镜/数据叠加"的原料。

| 事实 | 实际作用 |
| --- | --- |
| 地图、比分、阵营名、`tick_rate` | 决定 tick→秒换算，以及开场/结算画面的位置 |
| `match_end_tick` | 录制窗口不会越过结算面板，避免成片尾巴出现比分屏 |
| `players[]`（K/D/A、SteamID） | 玩家选择与统计 |
| `kills[]`（全量击杀） | 单一事实来源，支持"同时约束击杀双方"的筛选（树状分组做不到） |
| 武器 / `weapon_class` | 武器筛选与图标展示 |
| `is_headshot`、`hit_group` | 爆头率、命中部位筛选；可做"只用爆头做集锦" |
| `is_wallbang` + `penetrated_objects` | 区分"穿一层"和"双穿" |
| `distance` | 距离筛选（远程 AWP / 近身） |
| `is_noscope`、`is_through_smoke`、`is_attacker_blind`、`is_assisted_flash`、`victim_blinded` | 观赏性强的特殊条件 |
| `killer_health/armor`、`killer_airborne/ducking/scoped` | 残血、空跳、蹲射、开镜等"名场面"判定 |
| `is_opening_kill`、`killer_round_kills`、`is_clutch_kill`、`bomb_planted` | 首杀 / 多杀 / 残局 / 下包后，集锦排序的核心权重来源 |
| `clip_players[]`、`death_players[]` | 按击杀者/被害者两种分组视角 |
| 整局 POV 计划（`full_round_pov.go`） | 每回合一个 take；死亡后 +1s，存活到下一回合前 1s |

**缺口**：只有 `kill.Distance` 一个标量，**没有逐 tick 的玩家位置/朝向时间序列**。这是 2D 回放、自动运镜、热力图、竖屏跟随裁剪的共同前置条件（`demoinfocs-golang v5` 能提供，只是当前没采）。

### 1.4 片段选择（`frontend/src/domains/clip-selection`、`shared/kill-filter.ts`）

| 能力 | 实际作用 |
| --- | --- |
| 组内 OR / 组间 AND 的筛选 | 可表达"AWP 或 AK 的爆头或穿墙"，购物网站式筛选体验 |
| 18 个 trait | 一键捞"残局 / 3K 以上 / 空跳 / 穿烟" |
| 主视角与对方视角（`primary_view`） | 选中玩家是击杀者还是被害者，决定谁拿长窗口；死亡模式不会把主视角错当击杀者 |
| 逐片段覆盖 | 单条片段单独调前后秒数、语音、X 光，不影响全局 |

### 1.5 制作录制（`internal/plugingen` + `internal/clipsjson` + `internal/producews`）

| 能力 | 实际作用 |
| --- | --- |
| tick 调度的动作序列 | 由插件按 Demo tick 精确触发命令，而不是靠 sleep 猜时间 |
| 三类 take：killer / victim / `full_round_pov` | 同一枪可以出"我的视角 + 被害者视角 + 整局"三种素材 |
| 连续同角色击杀合并 | 同回合连续多杀不会每个击杀切一次镜头，成片连得上 |
| 编码探测与回退（`ffmpegprofile`） | `c1`=libx264（CPU）、`n1`=NVENC、`a1`=AMF、`i1`=QSV；`auto` 按实测能力择优，失败自动降级 |
| 4:3 真实像素拉伸 | `-vf scale=trunc(ih*16/9/2)*2:ih` 真实改分辨率，兼容忽略 SAR 的播放器与异常捕获高度 |
| `brand.vpk` 每次必挂 | 品牌水印/画面统一由 VPK 保证，不受开关影响 |
| `pov.vpk` 可选生命周期 | 只在本会话创建时删除，绝不覆盖用户已有文件 |
| POV 雷达每 pass 预热 | 切视角后 `csdm_radar_pov 1` 并预留 ≥2s Demo 时间，避免雷达空图 |
| 会话互斥 + 环境回滚 | 录制中无法再启动；失败保留备份供重试，不误杀无关 CS2 进程 |
| 优雅退出 | `end_produce_session` → `session_exit_ack` → 游戏线程 quit，比强杀进程干净 |

### 1.6 剪辑（`internal/edit`）

| 能力 | 实际作用 |
| --- | --- |
| 拼接 / trim / 排序 | 把 take 合成一条成片 |
| fade 转场 | 唯一的转场类型（`NormalizeTransitions` 明确拒绝其他） |
| 音频对齐 | `aresample/atrim/apad` 保证音画不漂 |
| 输出名 `O_EXCL` 预留 + rename 提交 | 不会覆盖既有成片，被外部替换时拒绝覆盖 |
| `fastEdit` | 对 victim take 做同回合同击杀者的快节奏裁剪，形成"被害者视角快切" |

### 1.7 调试工作台（`internal/app/debug_*`）

自定义 CS2 启动参数、一次性启动指令、控制台快捷指令（画面/声音分组 + 显式恢复命令）、demo 播放（含暂停事实、会话绑定）、单段手动录制 + 异步合并 + 重试、插件 DLL 覆盖、demo 目录沙箱。**实际作用**：把"想试一下某个命令"从"改配置 → 重新生成 → 重新录制"缩短到"点一下开关"。

### 1.8 已接入的画面/HUD 手段（逐条对照）

| 已用命令 | 实际作用 |
| --- | --- |
| `mirv_streams record screen enabled 1` + `settings add ffmpeg` + `record name` | 录制主体 |
| `mirv_streams record fps N` | 采集帧率（当前 1–240） |
| `mirv_streams record startMovieWav 1` | 录游戏内音频到 `takeXXXX/audio.wav` |
| `spec_mode` / `spec_player` / `demo_gototick` | 视角与定位 |
| `csdm_radar_pov 1` | 本项目插件的 POV 雷达 |
| `hidehud 128` / `cl_draw_only_deathnotices 1` | 隐藏聊天 / 隐藏除击杀提示外全部 HUD |
| `cl_teamcounter_playercount_instead_of_avatars` | 隐藏玩家头像 |
| `r_drawskybox 0` | 天空变黑，突出人物 |
| `mirv_sky clouds draw 0` | 关云层 |
| `mirv_deathmsg lifetime N` / `filter add block 1` | 击杀提示留存时长 / 屏蔽击杀提示 |
| `spec_show_xray 0\|-1` | 关闭/开启 X 光 |
| 越肩镜头命令组 | 第三人称越肩视角 |
| `r_show_build_info 0`、`cl_trueview_show_status 0`、`engine_no_focus_sleep 0`、`cl_demo_predict 0`、`cl_spec_show_bindings 0` | 去噪：去掉调试信息、TrueView 状态字、失焦休眠、观战按键提示 |
| `mirv_endofmatch 0` | 隐藏比赛结束动画/结算画面；录制初始化无条件写入，无 UI 开关 |

### 1.9 现有边界（缺口清单）

以下能力当前**完全没有实现**（全仓检索确认为 0 命中）：`mirv_campath`、`mirv_camio`、`mirv_input`、`mirv_cmd`、sampler/motion blur、`host_framerate`、`mirv_panorama`、`mirv_glow`、`mirv_colors`、`mirv_fix`、`mirv_viewmodel`、`mirv_replace_name`、`mirv_cvar_unhide_all`、`mirv_listentities`、`mirv_script_*`、ReShade；以及应用层的 2D 回放、热力图、竖屏导出、音乐/节拍、字幕、自动选段打分。

---

## 2. HLAE 高级能力：这些功能到底做什么

### 2.1 录制引擎类

#### 2.1.1 运动模糊采样器（`mirv_streams settings add sampler`）📖

**做什么**：在**一个输出帧**内以高帧率（如 1080fps）采样多次，把采样帧按权重混合成一帧。等价于 Sony Vegas 的 Resample / AE 的 Frame Blending。

**实际作用**：
- 24/30fps 成片里，快速甩枪、转身、跳跃不再"顿"，而是带自然拖影 —— 这是"电影感"最廉价、收益最大的一个开关。
- 高采集帧率**不会让文件更大**（输出仍是 60fps），只增加渲染耗时。
- 与"高帧率慢动作"配合：1080fps 采集 → 后期放慢到 1/4 仍然平滑。

**社区事实参数**（`hlae-cfg-cs2`）：

```cfg
mirv_streams settings add sampler blur
mirv_streams settings edit blur settings mp4          // 复用已注册的 ffmpeg 预设名
mirv_streams settings edit blur strength 1
mirv_streams settings edit blur method rectangle
mirv_streams settings edit blur exposure 0.7          // 模糊强度
mirv_streams settings edit blur fps 60                // 输出帧率
mirv_streams record fps 1080                          // 采集帧率（输入）
mirv_streams record screen settings blur              // 让屏幕录制走这个 sampler
```

**接入要点 / 风险**：
- 现有 `record_fps` 语义会**分裂**为"采集帧率"与"输出帧率"两个概念，必须新增字段而不是改旧字段语义（旧字段当前是 `mirv_streams record fps`）。
- ⚠️ sampler 会把输出交给内层 ffmpeg 设置，`{AFX_STREAM_PATH}` 的展开方式与当前 `take%04d.mp4` + `take%04d/audio.wav` 的假设需要重新核对，`producemerge.WaitForTakeFilesReady` / `MergeTakeVideoAudioContext` 会受影响。
- ⚠️ 采集 1080fps 意味着录制耗时约 18 倍（相对 60fps），需要预检与耗时提示。

#### 2.1.2 多通道 / Alpha Matte（`mirv_streams add world|playersMatte|weaponsMatte|viewModelMatte|particlesBlack|particlesWhite`）📖（HLAE ≥ 2.192.2）

**做什么**：一次录制同时输出多条"图层"，每条图层只含指定内容；其中 `*Matte` 系列带 alpha 通道（ProRes 4444）。

**实际作用**：这是职业 edit 的工作流 ——
- 把**人物**抠出来做背景虚化/换背景/描边发光；
- 把**武器**和**视角模型**单独拿去做运动模糊或缩放强调；
- 用 `particlesWhite - particlesBlack` 复原只含粒子的通道，做分层合成。

官方合成公式：`(world | players | weapons | viewmodel) * (particlesWhite - particlesBlack) + particlesBlack`。

**接入要点 / 风险**：需要超集编码（`afxFfmpegProres` / `afxFfmpegProresAlpha`）+ 数倍磁盘与写入带宽；必须同时产出"合成说明"，否则用户拿到一堆图层不知道怎么办。建议做成"高级输出模式"而不是默认路径。

#### 2.1.3 Depth 流（`mirv_streams add depth`）📖

**做什么**：输出逐像素深度图（可 10-bit 或 OpenEXR 23-bit）。

**实际作用**：后期做**真景深**（按深度聚焦到人物）、雾效、体积光、抠像。比 ReShade 的屏幕空间景深更"真"，但需要在 AE/Nuke 里合成，属于进阶用户功能。

#### 2.1.4 录制时同步导出相机运动（`mirv_streams record cam enabled 1`）📖

**做什么**：录制的同时把当前相机位置/朝向/FOV 写成 `.cam` 文件。

**实际作用**：把游戏相机轨迹交给 Blender/AE/C4D 做 3D 元素对位（追踪文字、模型、光效）。

### 2.2 运镜类（当前工具完全缺失，收益最大）

#### 2.2.1 `mirv_campath`（相机路径/关键帧）📖

**做什么**：在时间轴上打相机关键帧（位置+角度+FOV），启用后相机按路径插值飞行。支持 `add` / `enabled` / `draw enabled` / `print` / `remove` / `clear` / `save` / `load`（XML）/ `edit start` / `edit duration <s>` / `select` / `hold`。

**实际作用**：
- 不再只有"第一人称 POV"。可以做出：击杀前**远景俯视**交代位置 → 击杀瞬间**推到特写** → 击杀后**绕人环绕**。
- `edit duration` 可以把同一条路径拉伸/压缩到不同片段长度，**一条运镜模板复用整局所有击杀**。
- `hold` 让路径在起止点保持静止，避免片头片尾相机自己动。
- `save/load` 让运镜可**离线编辑**（XML 可手改/程序生成），这是自动化的关键。

#### 2.2.2 `mirv_camio`（相机导入导出，`.cam`）📖

**做什么**：`mirv_camio export start <file>` / `import start <file>` 与 Blender、After Effects、Cinema 4D 插件互导相机动画（含 FOV）；也可由 `mirv_streams record cam enabled 1` 自动导出。

**实际作用**：**这是"程序化生成运镜"的唯一稳定通道**。因为 `.cam` 是纯数据文件，可以由后端根据击杀位置/朝向计算后直接生成，再在录制 pass 开始时 `import`。有了它，"自动环绕被杀者""从击杀者肩后推镜"才能批量生产，而不需要人手动打关键帧。

#### 2.2.3 `mirv_input camera` + `cfg smooth`（自由镜头 / 平滑手持）📖

**做什么**：接管游戏相机为自由飞行（WASD/鼠标/升降/变焦/翻滚），并支持 `ksens/msens`、`mouseMoveSupport`、`stepFactor`，以及 `smooth halfTime[Vec|Ang|Fov]` 的一阶平滑。

**实际作用**：
- 手工录制时的**手持运镜**：可以边看 Demo 边像玩摄影机一样拍。
- 平滑参数让快速甩镜不再"抽帧"，得到接近稳定器的观感。
- 观战 Demo 本身的抖动/海豚跳，可以用平滑参数压掉。

**落地形态**：更适合做"交互式模式"（类似现有调试工作台），而不是全自动流水线。

#### 2.2.4 `mirv_cmd addCurves` / `mirv_time drive`（时间与参数曲线）📖

**做什么**：按 tick 区间执行带插值曲线的命令，例如 `mirv_time drive {0}` 让时间流速沿曲线变化（可立方插值）。

**实际作用**：**子弹时间 / 加速冲刺**。比在 FFmpeg 端 `setpts` 变速保留更多真实帧（因为是在游戏内按曲线跑），也避免后期变速造成的音画错位。

### 2.3 画面清理与风格化（低成本、观感立竿见影）

| 命令 | 实际作用 | 备注 |
| --- | --- | --- |
| `mirv_noflash 0\|1`（新版支持 `0.0–1.0` 强度）📖 | 柔化/移除闪光白屏。被闪是集锦里最"毁观感"的元素，压到 30% 既保留信息又不刺眼 | 现已接入：`noflash_strength`（0–1，两位小数），0 不写命令；需 HLAE ≥ 2.176.0 的浮点强度 |
| `mirv_endofmatch 0` 📖 | 隐藏比赛结束动画/结算场景。比现在的 `match_end_tick` 截断更彻底（连过渡动画一起处理） | 默认值为 1；现已接入：`RecordInitCommands` 无条件写入 0 |
| `mirv_viewmodel enabled 1` + `set X Y Z FOV 0\|1` 📖 | 固定左右手、自定义视角模型位置与 FOV。多段素材"一会儿左手一会儿右手"是最常见的廉价感来源 | 支持 `*` 保留引擎默认值 |
| `mirv_panorama panelStyle panelId=… opacity=0` 📖 | 精确隐藏单个 HUD 元素，如 `panelId=trueview_row`（TrueView 遥测字）、`panelClassName=CSGOHudHintText`（下包提示） | 比 `cl_draw_only_deathnotices` 粒度细得多；重载 Demo 需重发 |
| `mirv_glow projectiles/players/entities …` 📖 | 关闭烟雾/手雷的发光与拖尾，或强制某人发光（突出被击杀者） | |
| `mirv_colors glow/trails/smokes/teamid <color>` 📖 | 统一 CT/T 配色（默认粒子颜色在不同 Demo 里不一致），让整条片子视觉统一 | 部分值需切视角/切回合才刷新 |
| `mirv_sky color/material` + `clouds color/draw` 📖 | 换天空颜色/材质、云层颜色。用于风格化（黑天、黄昏、替换成其他地图天空） | 换地图后需重发 |
| `mirv_deathmsg lifetimeMod N\|default` 📖 | 让"本人击杀"的提示在屏幕上停留更久，其余正常 | 集锦里常用 |
| `mirv_deathmsg localPlayer xTrace` + `filter add attackerMatch=!xTrace victimMatch=!xTrace block=1 lastRule=1` 📖 | **只显示本人参与的击杀**，屏蔽其他玩家的击杀提示 | 比当前"全屏蔽"更符合集锦需求 |
| `mirv_deathmsg colors ct/t/border/background[Local]` 📖 | 击杀提示配色统一 | |
| `mirv_replace_name filter add x<XUID> "名字"` 📖 | 把死亡提示/底栏里的游戏内昵称替换成真名/统一代号（也可用于匿名化） | 不影响左侧 GOTV 头像栏 |
| `mirv_deathmsg help players` / `mirv_listentities isPlayer=1` 📖 | 打印玩家 userID/XUID 表，供上面几条命令使用 | |
| `mirv_fov` 📖 | 自定义 FOV（可与 camio 里的 FOV 配合） | |
| `mirv_fix time`（录制时自动启用）、`mirv_fix forceClInterpRatio` 📖 | 修复录制时 Panorama/场景系统时间与低帧率下的玩家动画抖动 | 新版本默认启用 |
| `sc_setclassflags SmokeVolumeObject 8`（配 `mirv_cvar_unhide_all`）📖 | 一键去烟（战术复盘/教学向素材） | 属"信息增强"而非美化 |
| 引擎自带：`snd_setmixer Dialog vol 0`、`cl_drawhud_force_teamid_overhead -1`、`cl_player_ping_mute 2`、`cl_spec_show_bindings false`、`cl_show_observer_crosshair 0`、`mp_display_kill_assists false`、`cl_demo_predict 2`（TrueView） | 分别去掉**无线电语音**、队友头顶名字、玩家标记、观战按键提示、观察者准星、助攻提示；开启 TrueView（真实第一人称枪口视角） | TrueView 对 POV 素材观感提升明显，值得作为开关 |

> 这批命令的共同特点：**不改变录制结构，只改画面内容**，因此可以像现有 `GameSettings` 一样做成"画面预设"打包下发（正式 Build 与调试工作台双端同步）。

### 2.4 后处理与脚本

| 能力 | 实际作用 | 备注 |
| --- | --- | --- |
| **ReShade_advancedfx** 📖 | 游戏内直接应用屏幕空间景深、锐化、调色、环境光遮蔽等；HLAE 可控制其开关（`mirv_reshade enabled 0\|1`） | 需要固定 ReShade 版本 + addon，兼容性风险高，建议做成"风格档位"而不是自由安装 |
| **mIRV-script**（`mirv_script_load` / `mirv_script_exec`，JS/BoA + websocket + 实体/录制事件）📖 | 在游戏内跑脚本：读实体、读写 cvar、订阅 `onRecordStart/onRecordEnd`、`mirv.fs`、`mirv.getDemoFilePath`、campath 角度绑定。可用于**动态 HUD、动态运镜、录制事件联动** | 相当于"插件协议之外的第二执行面"，是差异化能力，但需要 JS 脚本随包分发与版本管理 |
| `mirv_loadlibrary` 📖 | 加载自定义 DLL 扩展 | 高风险，不做规划 |

### 2.5 能力矩阵（当前工具对照）

| HLAE 能力 | CS2 | 当前工具 | 实际作用关键词 |
| --- | --- | --- | --- |
| screen 录制 + ffmpeg | ✅ | 已用 | 录制主体 |
| sampler 运动模糊 | ✅ | ❌ | 电影感、不顿 |
| 多通道 / matte / alpha | ✅ (≥2.192.2) | ❌ | 分层合成、抠人、换背景 |
| depth 流 | ✅ | ❌ | 真景深、雾效 |
| `record cam` | ✅ | ❌ | 相机轨迹给 3D 软件 |
| campath | ✅ | ❌ | 运镜、环绕、推镜 |
| camio 导入导出 | ✅ | ❌ | 程序化生成运镜 |
| `mirv_input camera` + smooth | ✅ | ❌ | 手持运镜、平滑 |
| `mirv_cmd` 曲线 + `mirv_time` | ✅ | ❌ | 子弹时间 |
| `mirv_skip` | ✅ | ❌（用 `demo_gototick`） | 精确定位 |
| `mirv_noflash` | ✅ | 已用（`noflash_strength` 强度） | 压闪光 |
| `mirv_endofmatch` | ✅ | 已用（无条件写入） | 去结算画面 |
| `mirv_viewmodel` | ✅ | ❌ | 固定左右手 |
| `mirv_panorama` | ✅ | ❌ | 精确隐藏 HUD 元素 |
| `mirv_glow` / `mirv_colors` | ✅ | ❌ | 配色统一、突出目标 |
| `mirv_sky color/material` | ✅ | 部分（仅 clouds draw） | 天空风格化 |
| `mirv_deathmsg` | ✅ | 部分（lifetime / block） | 只显示本人击杀 |
| `mirv_replace_name` | ✅ | ❌ | 昵称替换/匿名 |
| `mirv_fov` / `mirv_fix` | ✅ | ❌ | FOV 控制、动画平滑 |
| `mirv_listentities` / `mirv_cvar_unhide_all` | ✅ | ❌ | 取 ID、解锁隐藏 cvar |
| ReShade | ✅ | ❌ | 后处理风格 |
| mIRV-script | ✅ | ❌ | 游戏内脚本化 |

---

## 3. 同类工具与生态调研

| 参照 | 现状 | 本工具可借鉴 / 缺口 |
| --- | --- | --- |
| **CS Demo Manager (akiver)** | 本工具插件的上游。功能：2D 雷达回放、热力图、投掷物查找、VAC 追踪、多平台 demo 自动下载、HLAE 集成（自定义死亡提示、指定输出目录） | 缺口：**2D 预览、回合/投掷物级检索、多平台源** |
| **5Stack** | 浏览器内 2D + 3D 回放、道具轨迹、下包点位、买枪阶段浮层 | 说明"数据可视化"与"视频"是并列的一等公民 |
| **hlae-cfg-cs2** | 社区录制 cfg：1080fps + sampler blur、`mirv_viewmodel`、`mirv_deathmsg attackerMatch=!xTrace`、`cl_show_observer_crosshair 0` 等 | 本工具的"画面预设包"应直接对标这份清单 |
| **demo-slap / cs2.video / FragCut / FlexClip / Streamlabs AI** | "录像 → 竖屏短视频"已商业化，卖点是 AI 选段 + 9:16 + 一键分发 | 本工具是**本地免费**替代；同一条路线是最直接的差异化 |
| **自动导播研究**（AusDM 2018 学习排序、StarCraft Observer、LoL viewport tracking、MOBA 智能运镜） | "自动选镜头 + 自动运镜"已有成熟方法论 | 规则化 MVP 不需要 ML，可先做规则版 |
| **ravy.pro / cs2-data-pipeline** | 用音频指纹 + kill feed 视觉校验从**直播录像**里切高光 | 本工具有 demo 事实，选段精度天然更高，不需要 CV |
| **2D 回放开源实现**（sparkoo/csgo-2d-demo-viewer、CSGO2DReplay、Demovoi） | Go + WASM 解析、雷达图渲染，已解决坐标映射 | 可直接作为坐标换算与渲染方式的参考 |

---

## 4. 建议落地的工作项（含"实际作用"详解）

工作量：**S** ≈ 1–3 天，**M** ≈ 1–2 周，**L** ≈ 2–4 周（含 Windows 实机验证）。

### 4.0 通用落地约定

- 新增开关一律进 `internal/clipsjson/gamesettings.go` 的 `GameSettings` + `BuildDebugGameSettingPlan`，让**正式 Build 与调试工作台共用同一批命令**；调试侧必须同时给出 on/off 变体与显式恢复命令。
- 配置字段要同步：`internal/config/config.go` → `internal/app/clip_settings.go` → `frontend/src/shared/types/` → `frontend/src/domains/settings/settings-schema.ts` → `frontend/src/shared/i18n/zh-CN.json`（`en-US.json` 由用户维护）。
- 关闭态行为必须与现状一致：默认不写入新命令、不反向重置，避免改变既有成片。

---

### P0 — 低风险、立刻提升成片质感

#### P0-1 运动模糊 / 高帧率采样（S–M）

- **实际作用**：24/30fps 成片里快速甩枪、转身、跳跃不再顿挫，带自然拖影；1080fps 采集让后期慢放仍然平滑。
- **用户可见效果**：同一段素材，"廉价录屏感"→"剪辑软件导出的感觉"。
- **实现**：`RecordInitOptions` 增加 sampler 名与参数；采集帧率与输出帧率拆成两个字段；`mirv_streams record screen settings <sampler>`。
- **代码落点**：`internal/clipsjson/gamesettings.go`、`internal/clipsjson/builder.go`、`internal/config/config.go`、`internal/app/clip_settings.go`、`frontend/src/domains/settings/`。
- **风险**：⚠️ `{AFX_STREAM_PATH}` 展开与 take 文件路径假设需重核，会影响 `producemerge` 的等待/合并逻辑；耗时约 10–20 倍，需预检提示。

#### P0-2 自动高光打分 + 一键选片（M）

- **实际作用**：用已有事实算"精彩度"（ACE/残局/首杀/多杀/穿墙/远距离/残血/空跳/爆头权重），自动选出全场最佳 N 段并预填选择列表。
- **用户可见效果**：从"翻 20 分钟 Demo 挑 8 枪"→"点一下，直接开始制作"。
- **实现**：纯函数放在 `internal/plugingen`（无副作用、不读配置、不碰磁盘），前端只消费排序结果；提供"权重预设"（稳健 / 刺激 / 教学）。
- **代码落点**：`internal/plugingen/selection.go`（新增评分函数）、`internal/app/plugin_generate.go`、`frontend/src/domains/clip-selection/`。
- **风险**：低。但权重必须可解释、可关闭，避免用户不理解"为什么没选我那枪"。

#### P0-3 画面风格包（视图模型 / 精确隐藏 HUD / 配色统一）（S–M）

- **实际作用**：
  - `mirv_viewmodel`：固定左右手，解决多段素材手部不一致；
  - `mirv_panorama panelStyle`：隐藏 TrueView 遥测字、下包提示等具体面板；
  - `mirv_colors` + `mirv_sky color/material` + `mirv_deathmsg colors`：CT/T 配色与天空统一，让整片看起来是"同一次拍摄"。
- **用户可见效果**："能看出是同一个作者做的片子"。
- **实现**：打包成 2–3 个"画面预设"（如「干净竞技」「深色电影」），一个开关切换整组命令。
- **代码落点**：`internal/clipsjson/gamesettings.go`（新增预设→命令列表的纯函数）、设置页新增预设选择。
- **风险**：`mirv_panorama` 重载 Demo 后需重发（当前每个 Demo 一份 JSON，天然满足）；面板 ID 需按游戏版本白名单化并容错。

#### P0-4 闪光柔化 / 结算场景隐藏 / 视图清理包（S）

- **实际作用**：`mirv_noflash` 压闪光白屏；`mirv_endofmatch 0` 彻底去掉结算动画；`mirv_deathmsg localPlayer xTrace` + `filter add attackerMatch=!xTrace …` 只保留本人击杀提示；再加上 `snd_setmixer Dialog vol 0`、`cl_drawhud_force_teamid_overhead -1`、`cl_player_ping_mute 2`、`cl_show_observer_crosshair 0`、`mp_display_kill_assists false`、`cl_demo_predict 2`（TrueView）。
- **用户可见效果**：画面里只剩"人 + 枪 + 击杀提示"。
- **代码落点**：同 P0-3。
- **风险**：⚠️ `mirv_noflash` 浮点强度依赖较新 HLAE（≥ 2.176.0）；当前安装版本已满足，故未做版本判定与 0/1 回退，旧版输出会报未知参数。

#### P0-5 转场扩展（S–M）

- **实际作用**：目前只有 fade。接入 FFmpeg `xfade`（wipe/slide/circle/dissolve…）与时长曲线，让剪辑段之间的衔接不再是"糊一下"。
- **代码落点**：`internal/edit/plan.go`（放开类型白名单）、`internal/edit/filtergraph.go`（生成 xfade 图）、`frontend/src/domains/edit/`。
- **风险**：低。注意 xfade 要求同分辨率/帧率/SAR，现有 filter graph 已经统一到目标分辨率与 `fps`，改动面可控。

---

### P1 — 中期，构成差异化

#### P1-1 自动运镜 MVP（L）

- **实际作用**：由击杀事件生成相机关键帧，产出"远景交代 → 推近特写 → 环绕"的镜头；一条路径可用 `mirv_campath edit duration` 复用给不同长度的片段。
- **前置**：**补齐逐 tick 位置/朝向采样**（`internal/demo` 新增采样 API），这是 2D 回放、热力图、竖屏跟随的共同前置。
- **实现路径**：demo 位置采样 → 生成 `.cam`（`mirv_camio import start <file>`）或 campath XML（`mirv_campath load <file>`）→ pass 开始时导入并 `enabled 1` → 录制 → pass 结束 `import end` / `enabled 0`。
- **代码落点**：`internal/demo`（采样）、`internal/plugingen`（路径规划，纯函数）、`internal/clipsjson`（命令序列）、`internal/app/plugin_generate.go`。
- **风险**：⚠️ 相机插值观感（速度曲线、穿墙、被遮挡）需要实机反复调参；建议先做 2–3 个固定预设而不是全自由。

#### P1-2 竖屏短视频导出（M–L）

- **实际作用**：9:16 输出，裁切窗口**跟随击杀事件与位置**（而不是永远居中），叠加击杀信息条与字幕，落到 `outputs/shorts`。
- **用户可见效果**：可直接发抖音/Reels/Shorts，这是当前商业化工具的主战场。
- **实现**：FFmpeg `crop` 表达式或分段 crop 关键帧（依赖 P1-1 的位置采样）；文字用 `drawtext`/`libass`。
- **代码落点**：`internal/edit/filtergraph.go`、`internal/app/app_edit.go`、`frontend/src/features/edit/`。
- **风险**：中。需要决定"跟随"的质量上限，否则会出现镜头疯狂横移。

#### P1-3 数据驱动 HUD 叠加层（M）

- **实际作用**：把已有事实渲染到画面上：击杀计数、"3K/ACE"标签、武器图标、玩家名、回合与比分、小雷达。
- **实现**：前端 canvas 生成 PNG 序列 → FFmpeg `overlay`；或 Go 侧生成 ASS 字幕（`libass`）做文字层。
- **代码落点**：`internal/edit/`（overlay 规划）、`frontend/src/shared/ui/`（通用渲染组件）。
- **风险**：低-中。注意与 POV HUD（游戏内 VPK）职责划分：**游戏内 HUD 更"真"，后期叠加更"可控"**，两者都要保留。

#### P1-4 2D 回放 / 雷达预览（L）

- **实际作用**：选片时用雷达图预览击杀发生位置与走位，替代"靠文字列表盲选"；也可输出角落 2D 小窗。
- **实现**：位置采样 + 地图雷达图坐标换算 + 前端 canvas 播放器（参考 sparkoo/csgo-2d-demo-viewer）。
- **风险**：⚠️ CS2 雷达图资源与坐标元数据来源需确认（与 CS:GO 的 `resource/overviews/*.txt` 不同），建议先做**单张静态热力图/击杀点分布**验证坐标换算，再上回放。

#### P1-5 音频工程（M）

- **实际作用**：背景音乐导入 + 自动 ducking（`sidechaincompress`）、语音/游戏音增益分离、按节拍对齐切换点。
- **代码落点**：`internal/edit/filtergraph.go`（`amix`/`sidechaincompress`）、`frontend/src/domains/edit/`。
- **风险**：低。节拍检测（若做）需要在 Go 或前端引入轻量 BPM 分析，属于增量成本。

#### P1-6 多平台 demo 源（M）

- **实际作用**：除完美/5E 外，支持 FACEIT、MatchZy、Esplay 等自动拉取最近比赛；纯本地解析，不上传。
- **代码落点**：新增 `internal/<platform>/`（对齐 `internal/wanmei`、`internal/fivee` 的形态）、`internal/app/import_coordinator.go` 复用。

#### P1-7 多通道 / Alpha Matte 输出模式（L）

- **实际作用**：一次录制产出 world/players/weapons/viewmodel/particles 分层，供 AE 做人物分离、背景替换、真景深。
- **定位**：进阶用户"高级输出"，默认关闭，附带合成说明与磁盘预检。
- **风险**：⚠️ 磁盘与写入带宽、超集编码预设（`afxFfmpegProres[Alpha]`）版本门槛、`record name` 下多图层目录结构需重新设计合并流程。

#### P1-8 ReShade 风格档（M–L）

- **实际作用**：游戏内直接应用景深/调色/环境光遮蔽，得到"电影调色"观感。
- **风险**：⚠️ 需锁定 ReShade + addon 版本，兼容性随 CS2 更新波动；建议只做"内置档位 + 一键启用/关闭"，不开放自由安装。

---

### P2 — 高阶与差异化

| 项 | 实际作用 | 关键点 |
| --- | --- | --- |
| **规则化自动导播** | 多机位自动切换：远景铺垫 → 击杀瞬间切 POV → 击杀后环绕/俯视；把"手动挑镜头"变成"一键成片" | 依赖 P1-1；先用规则（多杀用环绕、单杀用推镜）而不是 ML |
| **mIRV-script 集成** | 在游戏内跑 JS：动态 HUD、动态运镜、录制事件回调 | 相当于第二个执行面，需脚本随包分发与版本管理 |
| **`mirv_cmd addCurves` + `mirv_time` 变速** | 子弹时间/加速曲线，保留更多真实帧 | 与 sampler 叠加效果更佳 |
| **POV HUD 升级为自有 Panorama addon** | 游戏内直接渲染击杀计数/连杀/武器名，比后期叠加更"真" | 现有 `tools/hud-pov-vpk` 已证明注入链路可行 |
| **工程侧** | take 级断点续录、录制丢帧/音画同步自检报告、录制耗时与磁盘预估、分享文案生成 | 直接提升长会话可用性 |

---

## 5. 推进顺序建议

1. **P0-1 运动模糊/高帧率采样** —— 收益最大、改动集中，先做它同时暴露 `AFX_STREAM_PATH`/合并流程的隐患。
2. **P0-2 自动高光打分** —— 事实齐备、零外部依赖、用户最容易感知。
3. **P0-3 + P0-4 画面风格与清理包** —— 与 1、2 并行，合成一份"画面预设"。
4. **P1-1 自动运镜 MVP（先补逐 tick 位置采样）** —— 采样这一步同时解锁 2D 回放、热力图、竖屏跟随。
5. **P1-2 竖屏短视频导出** —— 在采样能力就绪后落地，形成"本地免费 + 短视频直达"的差异点。

---

## 6. 风险与前提

| 风险 | 说明 | 应对 |
| --- | --- | --- |
| **HLAE 版本门槛** | matte 流 ≥ 2.192.2、10-bit depth ≥ 2.188.0、ProRes/ProResAlpha 预设与 `mirv_endofmatch` ≥ 2.181.1、浮点 `mirv_noflash` ≥ 2.176.0 之后 | `envsetup` 提升最低版本要求，并在 UI 明确提示；功能做版本判定与降级 |
| **CS2 更新即失效** | HLAE 每次游戏更新都可能坏 | 新功能尽量"失败可降级"；复用现有 warning 语义而不是把组件判成 failed |
| **资源成本** | 1080fps 采集、多通道 ProRes 会数倍增加耗时与磁盘 | 录制前预检（磁盘、预计耗时）与明确提示；默认不开高级模式 |
| **反作弊/平台** | HLAE 仅适用于 Demo，不能用于在线对局 | 复用现有平台客户端检查，扩展为显式警告 |
| **验证边界** | Node 单测与前端构建**不能**证明 HLAE 注入、多通道输出、运镜插值在实机的效果 | 任何 P1/P2 项都必须排 Windows 实机验收清单 |
| **契约影响** | 新增设置/事件/字段需同步实现、消费者、测试与 AGENTS 文档 | 按 `AGENTS.md`「跨层契约」章节逐项更新 |

---

## 7. 附录：可粘贴到控制台验证的命令速查

```cfg
// —— 录制引擎 ——
mirv_streams settings add ffmpeg mp4 "-c:v libx264 -pix_fmt yuv420p -preset ultrafast -crf 9 {QUOTE}{AFX_STREAM_PATH}\video.mp4{QUOTE}"
mirv_streams settings add sampler blur
mirv_streams settings edit blur settings mp4
mirv_streams settings edit blur strength 1
mirv_streams settings edit blur method rectangle
mirv_streams settings edit blur exposure 0.7
mirv_streams settings edit blur fps 60
mirv_streams record fps 1080
mirv_streams record screen settings blur

// —— 运镜 ——
mirv_campath add                      // 打关键帧（当前时间+视角）
mirv_campath enabled 1                // 启用路径（至少 4 帧）
mirv_campath draw enabled 1
mirv_campath print                    // 打印关键帧信息
mirv_campath save "D:/cam/path.xml"
mirv_campath load "D:/cam/path.xml"
mirv_campath edit duration 4.0        // 整条路径缩放到 4 秒
mirv_camio export start "D:/cam/take.cam"
mirv_camio import start "D:/cam/take.cam"
mirv_input camera                     // 自由镜头（ESC 退出）
mirv_input cfg smooth halfTime 0.5

// —— 画面清理与风格 ——
mirv_noflash 1
mirv_endofmatch 0
mirv_viewmodel enabled 1; mirv_viewmodel set 2 0 -2 68 0
mirv_panorama panelStyle panelId=trueview_row opacity=0
mirv_sky color 20 20 30 255; mirv_sky clouds color 40 40 50; mirv_sky clouds draw 0
mirv_colors glow ct blue; mirv_colors glow t red
mirv_deathmsg localplayer xTrace
mirv_deathmsg lifetimeMod 10
mirv_deathmsg filter add attackerMatch=!xTrace victimMatch=!xTrace block=1 lastRule=1
mirv_deathmsg help players
snd_setmixer Dialog vol 0
cl_drawhud_force_teamid_overhead -1
cl_player_ping_mute 2
cl_spec_show_bindings false
cl_show_observer_crosshair 0
mp_display_kill_assists false
cl_demo_predict 2
cl_trueview_show_status 0

// —— 依赖隐藏 cvar 的命令 ——
mirv_cvar_unhide_all
sc_setclassflags SmokeVolumeObject 8      // 去烟（0 恢复）
```

> ⚠️ 以上命令的**参数拼写与可用性以游戏内命令帮助为准**（`mirv_streams`、`mirv_campath`、`mirv_deathmsg` 等直接回车会打印子命令帮助）。本文不作为接口契约。

---

## 8. 与仓库其他文档的关系

- 契约与长期约束：根 `AGENTS.md`、`internal/AGENTS.md`、`frontend/AGENTS.md`。
- 具体实施计划：`docs/plan-*.md`（如本调研中的某项进入实施阶段，应另开 `docs/plan-<feature>.md`）。
- 本文只承载**能力调研与优先级判断**，不承载命令时序、字段语义或验收标准。
