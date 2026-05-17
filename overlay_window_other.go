//go:build !darwin

package main

// configureOverlayWindow is a macOS-only AppKit refinement. Other platforms
// rely on Wails' own frameless/always-on-top window behaviour.
func configureOverlayWindow() {}

// restoreOverlayWindow is a macOS-only AppKit refinement.
func restoreOverlayWindow() {}
