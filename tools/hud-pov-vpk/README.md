# POV HUD VPK

`pov.template.vpk` 保留修复前的三份 HUD 资源；`hudteamcounter.pov.css` 只追加顶部头像血条的阵营显示规则，不改变其他 POV HUD 样式。

## 构建与校验

需要 Python 3.10+，无第三方依赖，在仓库根目录执行：

```bash
python3 tools/hud-pov-vpk/build_vpk.py
python3 -m unittest discover -s tools/hud-pov-vpk -v
```

默认输出 `internal/producegame/assets/pov.vpk`，可用 `--output /path/to/pov.vpk` 指定其他位置。脚本始终从本目录模板构建，不累加重复补丁；保留图片引用和依赖块，重算 Panorama DATA CRC、VPK 条目 CRC 与格式规定的 MD5 完整性字段。除 `hudteamcounter.vcss_c` 外其他两份资源字节不变。

## 显示规则

- 整个 `.healthbar-container` 默认透明，包括红色背景；头像和布局占位不变。
- `.HUD--team--ct` 只显示 `#TeamLargeCT` 血条。
- `.HUD--team--terrorist` 只显示 `#TeamLargeT` 血条。
- 未识别当前视角阵营时不显示任一队血条，不回退到全队可见。
- 使用 HUD 的视角阵营标记，不使用本地 Demo 观察者的队伍，也不将敌方血条显示为假满血。

选择器依据：[游戏 HUD 布局](https://github.com/SteamDatabase/GameTracking-CS2/blob/master/game/csgo/pak01_dir/panorama/layout/hud/hudteamcounter.xml) 中的两队容器和 `HealthBarContainer`，以及游戏 `hudhealthammocenter` 已使用的 `HUD--team--ct/terrorist`；二进制格式参考 [ValveResourceFormat Panorama](https://github.com/ValveResourceFormat/ValveResourceFormat/blob/master/ValveResourceFormat/Resource/ResourceTypes/Panorama.cs)。

## 游戏内验收（本地自动测试不能替代）

完全退出 CS2 后更新 VPK 并重启。分别以 CT、T 第一视角查看：己方血条正常变化、敌方没有白条或红色背景、十个头像仍保留；再核对 CT↔T 切视角、半场换边、死亡、seek 与装备信息展开时没有敌方血条泄漏。

重新构建应用后，新内嵌资源随 POV HUD 开关部署。应用不会覆盖 CS2 `game/csgo/pov.vpk` 中已存在的用户文件；如果该路径残留旧 VPK，需要先备份并替换，不能只重启应用就假定已使用新版本。不改变现有 VPK 挂载和恢复生命周期。
