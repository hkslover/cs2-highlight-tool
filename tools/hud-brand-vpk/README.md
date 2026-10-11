# HUD 品牌 VPK

本目录可独立复制、维护和构建，不依赖其他测试版本、`exported.zip` 或 Source 2 Viewer。

## 日常更新

1. 修改并保存 `hud.brand.css`，调整位置、尺寸、透明度等样式。
2. 需要更换 Logo 时替换 `logo.png`，保持 **384×128、RGBA、透明背景**。
3. macOS 双击 `build.command`，Windows 双击 `build.bat`。
4. 构建成功后使用本目录的 `brand.vpk`，`manifest.json` 记录大小、SHA-256 和资源 CRC。

需要安装 Python 3.10 或更新版本，并确保命令行可以找到它。双击脚本优先使用已有的 Pillow；缺少时自动创建本目录的 `.venv` 并安装 `requirements.txt`（首次需要联网）。macOS 如提示无法执行，可运行 `chmod +x build.command`。

也可以在本目录执行：

```bash
python3 -m pip install -r requirements.txt
python3 build_vpk.py
```

脚本始终从自身目录读取 CSS 和模板，输出到自身目录。可用 `--image /path/to/logo.png` 指定另一张符合要求的图片；不会覆盖本目录的 Logo。构建不写入 CSS、Logo 或模板，只更新 `brand.vpk` 和 `manifest.json`，不会生成 `package/` 或 `verified/`。要更新软件内嵌品牌资源，构建后将 `brand.vpk` 复制至 `internal/producegame/assets/brand.vpk` 并重新构建应用。

## 保留文件

| 文件 | 用途 |
| --- | --- |
| `hud.brand.css` | 手动维护的完整 HUD 样式，是构建输入 |
| `logo.png` | 透明 Logo，是构建输入 |
| `hud.template.vcss_c` | 保留 HUD 资源格式和依赖元数据的二进制模板，勿删除 |
| `build_vpk.py` | 独立构建脚本 |
| `build.command` / `build.bat` | macOS / Windows 双击构建入口 |
| `requirements.txt` | Python 图片处理依赖 |
| `brand.vpk` / `manifest.json` | 当前构建结果及校验信息 |
| `.gitignore` | 忽略本地 Python 环境和缓存 |

样式资源更新 DATA CRC，保留依赖元数据并移除旧 source map；纹理采用 RGBA8888 单 mip，避免重新引入此前 DXT5 版本的差异。二进制格式参考 [ValveResourceFormat](https://github.com/ValveResourceFormat/ValveResourceFormat)，不调用 Valve resourcecompiler，也不检查 Panorama CSS 的运行时语法。

更新游戏中的 VPK 前完全退出 CS2，替换已有挂载路径对应的文件，再重启验证。若游戏挂载的名称不是 `brand.vpk`，部署时沿用该名称即可。不要同时挂载多个品牌版本；本地构建和校验不能替代游戏内显示验证。
