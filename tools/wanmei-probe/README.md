# Wanmei Demo 全链路诊断工具 (`wanmei-probe`)

独立于主程序的本地诊断 Web 工具，用于逐段排查 `internal/wanmei` 7 步获取与下载 Demo 链路是否正常，并在接口字段、版本号或压缩格式变化时快速定位故障点。

## 启动方式

在仓库根目录执行：

```bash
go run ./tools/wanmei-probe
```

常用参数：

- `-addr 127.0.0.1:18999`：HTTP 监听地址（设为 `127.0.0.1:0` 自动分配空闲端口）
- `-open=true`：启动后自动打开默认浏览器（无桌面环境可传 `-open=false`）
- `-workdir ""`：Step 7 临时下载/解压目录（默认自动创建临时目录并在 `Ctrl+C` 退出时自动清理）

在 macOS 上交叉编译供 Windows 直接运行的单文件可执行程序：

```bash
GOOS=windows GOARCH=amd64 go build -o wanmei-probe.exe ./tools/wanmei-probe
```

## 覆盖步骤

1. **本地登录态** (`http://127.0.0.1:55555/` + `Origin`/`Referer`)：支持修改端口/URL、请求头，显示完整 HTTP 状态、CORS 响应头与原始响应体。
2. **解码 Token** (`decodeEncodeLog` + `parseDecodedToken`)：展示 `^<hex>$` 校验、XOR 解码明文、`token` / `serverTime` / `steamID` 分段。
3. **战绩列表** (`POST https://api.wmpvp.com/api/csgo/home/match/list`)：默认 `appversion` 为 `4.2.2`（支持一键切换 `4.2.2` / `3.7.7` / `1.0.26111211`），支持独立修改 `toSteamId`（留空默认同 `mySteamId`）以查询其他玩家战绩，支持调整 `platform`、分页与自定义头，支持一键选用场次到后续步骤。
4. **公网 IP** (`https://api-ipv4.ip.sb/ip`)：验证出口 IPv4 并支持手动覆盖用于 Step 6 签名。
5. **签名 Demo URL** (`pwaweblogin.wmpvp.com`)：支持调整 `appid`/`secret`/`cup_id`/`r`/`t`，展开查看 `MD5` 与 `SHA1` 签名中间值。
6. **换 OSS 直链** (`X-PWA-SteamId` / `X-PWA-Signature`)：默认不跟随重定向，直接展示 302 `Location` 或非 3xx（如 200 JSON）完整响应体并自动提取候选 URL。
7. **下载与解压** (`download.FileWithContext` → `UnzipWithContext` → `FindFirstByExt(".dem")`)：支持仅探测响应头与前 4KB 魔数，以及完整下载解压；若 `UnzipWithContext` 失败且检测到 `7z` 或直接返回 `.dem`，会自动运行兼容诊断以确认格式变化。
8. **原生 `internal/wanmei` 回归对比**：直接调用当前未修改的 `wanmei.ListRecentMatches` 与 `wanmei.ImportDemoContext`。
