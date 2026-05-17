//go:build !darwin

package main

import "github.com/mmmy/snapgo/internal/infrastructure/display"

// showNativeCaptureOverlay is only implemented on macOS. Other platforms
// keep using the Wails WebView overlay fallback.
func showNativeCaptureOverlay(_ *App, _ display.Info) bool {
	return false
}
