package main

import (
	application "cs2-highlight-tool-v2/internal/app"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed wails.json
var wailsConfigData []byte

func main() {
	// 单例检查：如果已有实例在运行，静默退出
	cleanup, err := application.EnsureSingleInstance()
	if err != nil {
		os.Exit(0)
	}
	defer cleanup()

	backend := application.New(wailsConfigData)

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "CS2 Highlight Tool",
		Width:  920,
		Height: 720,
		// Below this the main steps bar plus the ad banner leave too little room
		// for the step views. Wails clamps Width/Height into this range.
		MinWidth:  920,
		MinHeight: 680,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 17, G: 19, B: 18, A: 1},
		OnStartup:        backend.Startup,
		OnShutdown:       backend.Shutdown,
		Bind: []interface{}{
			backend,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
