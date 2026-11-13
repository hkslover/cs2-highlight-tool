module cs2-highlight-tool-v2

go 1.27.0

require (
	github.com/bodgit/sevenzip v1.6.1
	github.com/gorilla/websocket v1.5.3
	// 固定到 master 伪版本：v6.0.0-rc.1 之后才修复了 2026-11-11 CS2 更新引入的 fixed8/fixed16/fixed32
	// 实体字段编码（旧版本按 varint 读取导致包错位，新 Demo 从第 31 帧起解析失败）。上游打 tag 后换成正式版本。
	github.com/markus-wa/demoinfocs-golang/v6 v6.0.0-rc.1.0.20261112163323-7ce5901fc9df
	github.com/wailsapp/wails/v2 v2.11.0
	golang.org/x/sys v0.30.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/bep/debounce v1.2.1 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/golang/geo v0.0.0-20230421003525-6adc56603217 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jchv/go-winloader v0.0.0-20210711035445-715c2860da7e // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/labstack/echo/v4 v4.13.3 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/leaanthony/go-ansi-parser v1.6.1 // indirect
	github.com/leaanthony/gosod v1.0.4 // indirect
	github.com/leaanthony/slicer v1.6.0 // indirect
	github.com/leaanthony/u v1.1.1 // indirect
	github.com/markus-wa/go-unassert v0.1.3 // indirect
	github.com/markus-wa/gobitread v0.2.5 // indirect
	github.com/markus-wa/godispatch v1.5.1 // indirect
	github.com/markus-wa/ice-cipher-go v1.0.0 // indirect
	github.com/markus-wa/quickhull-go/v2 v2.2.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/samber/lo v1.49.1 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/tkrajina/go-reflector v0.5.8 // indirect
	github.com/ulikunitz/xz v0.5.12 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/wailsapp/go-webview2 v1.0.22 // indirect
	github.com/wailsapp/mimetype v1.4.1 // indirect
	go4.org v0.0.0-20200411211856-f5505b9728dd // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

// replace github.com/wailsapp/wails/v2 v2.11.0 => /Users/snow/go/pkg/mod
