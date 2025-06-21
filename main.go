package main

import (
	"embed"

	"email-pdf-downloader/backend"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2"
)

//go:embed all:frontend/dist
var assets embed.FS

// main 程序入口点
func main() {
	// 创建应用实例
	app := backend.NewApp()

	// 创建应用配置
	err := wails.Run(&options.App{
		Title:  "邮件附件下载器",
		Width:  1200,
		Height: 800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.OnStartup,
		OnDomReady:       app.OnDomReady,
		OnShutdown:       app.OnShutdown,
		Bind: []interface{}{
			app, // 将App实例绑定到前端
		},
		Fullscreen:       false,
		StartHidden:      false,
		HideWindowOnClose: true, // 关闭时隐藏窗口而不是退出
		DisableResize:    false,
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
} 