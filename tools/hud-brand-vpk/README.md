# HUD 品牌 VPK

本目录不依赖其他测试版本、`exported.zip` 或 Source 2 Viewer；默认将 VPK 直接输出到仓库的内嵌资源目录。独立复制使用时可通过 `--output-dir` 指定输出位置。

## 日常更新

1. 修改并保存 `hud.brand.css`，调整位置、尺寸、透明度等样式。
2. 需要更换 Logo 时替换 `logo.png`，保持 **384×128、RGBA、透明背景**。
3. macOS 双击 `build.command`，Windows 双击 `build.bat`。
4. 构建成功后，仓库的 `internal/producegame/assets/brand.vpk` 会直接更新；本目录的 `manifest.json` 记录大小、SHA-256 和资源 CRC。重新构建应用即可使用更新后的内嵌品牌资源。

需要安装 Python 3.10 或更新版本，并确保命令行可以找到它。双击脚本优先使用已有的 Pillow；缺少时自动创建本目录的 `.venv` 并安装 `requirements.txt`（首次需要联网）。macOS 如提示无法执行，可运行 `chmod +x build.command`。

也可以在本目录执行：

```bash
python3 -m pip install -r requirements.txt
python3 build_vpk.py
```

脚本始终从自身目录读取 CSS 和模板；VPK 的默认输出目录按脚本位置定位，不受启动命令所在目录影响。可用 `--image /path/to/logo.png` 指定另一张符合要求的图片；不会覆盖本目录的 Logo。构建不写入 CSS、Logo 或模板，只更新目标目录的 `brand.vpk` 和本工具目录的 `manifest.json`，不会生成 `package/` 或 `verified/`。

需要输出到其他目录时，执行 `python3 build_vpk.py --output-dir /path/to/output`；指定目录不存在时自动创建。要输出到当前目录，使用 `python3 build_vpk.py --output-dir .`。两个双击构建脚本使用相同的默认输出位置，并支持透传命令行参数。

## 保留文件

| 文件 | 用途 |
| --- | --- |
| `hud.brand.css` | 手动维护的完整 HUD 样式，是构建输入 |
| `logo.png` | 透明 Logo，是构建输入 |
| `hud.template.vcss_c` | 保留 HUD 资源格式和依赖元数据的二进制模板，勿删除 |
| `build_vpk.py` | 独立构建脚本 |
| `build.command` / `build.bat` | macOS / Windows 双击构建入口 |
| `requirements.txt` | Python 图片处理依赖 |
| `../../internal/producegame/assets/brand.vpk` | 默认 VPK 构建结果 |
| `manifest.json` | 当前构建的校验信息，保留在工具目录 |
| `.gitignore` | 忽略本地 Python 环境和缓存 |

样式资源更新 DATA CRC，保留依赖元数据并移除旧 source map；纹理采用 RGBA8888 单 mip，避免重新引入此前 DXT5 版本的差异。二进制格式参考 [ValveResourceFormat](https://github.com/ValveResourceFormat/ValveResourceFormat)，不调用 Valve resourcecompiler，也不检查 Panorama CSS 的运行时语法。

更新游戏中的 VPK 前完全退出 CS2，替换已有挂载路径对应的文件，再重启验证。若游戏挂载的名称不是 `brand.vpk`，部署时沿用该名称即可。不要同时挂载多个品牌版本；本地构建和校验不能替代游戏内显示验证。
