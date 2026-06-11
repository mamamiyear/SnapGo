package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/mmmy/snapgo/internal/infrastructure/logging"
	"github.com/mmmy/snapgo/internal/infrastructure/tray"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Init logging FIRST so the rest of startup (config load, hotkey
	// registration, tray bootstrap) is captured to ~/Library/Logs/SnapGo
	// even when the binary was launched via `open` and stderr is /dev/null.
	closeLog := logging.Init()
	defer closeLog()

	app := NewApp()

	// Wire up the menu-bar agent. Start() returns a `start` thunk that we
	// invoke from OnStartup so the icon appears AFTER Wails has spun up the
	// Cocoa runloop, and a `stop` thunk we call on shutdown.
	startTray, stopTray := tray.Start(tray.Callbacks{
		OnCapture:  func() { app.CaptureNow() },
		OnSettings: func() { app.ShowWindow() },
		OnQuit:     func() { app.QuitApp() },
	})

	err := wails.Run(&options.App{
		Title:     "SnapGo",
		Width:     1080,
		Height:    720,
		MinWidth:  720,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Keep the standard macOS title bar for the Settings window. The
		// capture UI uses a separate native NSPanel on macOS, so the main
		// Wails window no longer needs to be frameless.
		Frameless:         false,
		StartHidden:       true,
		HideWindowOnClose: true,
		BackgroundColour:  &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			installActivationPolicyHooks()
			hideDockIcon()
			startTray()
		},
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			hideDockIcon()
			return false
		},
		OnShutdown: func(ctx context.Context) {
			stopTray()
			app.shutdown(ctx)
		},
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			// The capture overlay is native AppKit on macOS; the Wails
			// Settings window should behave like a normal macOS window.
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "SnapGo",
				Message: "Cross-platform screenshot tool with one-click upload to S3.",
			},
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

	// Defensive: ensure the tray is torn down even if Wails exited via an
	// error path that bypassed OnShutdown.
	stopTray()
}
