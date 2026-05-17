// Package tray runs the menu-bar agent UI for SnapGo on macOS / Linux /
// Windows. It is intentionally decoupled from the main App struct: the App
// supplies a small set of callbacks (Capture / Settings / Quit) so that this
// package never imports the application service stack.
//
// We use fyne.io/systray (a fork of getlantern/systray) because it is
// designed to coexist with other GUI runloops — important when the host
// process is also driving a Wails / Cocoa main loop.
package tray

import (
	"fyne.io/systray"
)

// Callbacks bundles the menu actions the host application wants to expose.
// Splitting these into a struct (rather than 3 positional arguments) keeps
// future extension cheap — we can add e.g. OnCheckForUpdates without
// touching the call sites.
type Callbacks struct {
	OnCapture  func()
	OnSettings func()
	OnQuit     func()
}

// Start installs the menu-bar status item and returns immediately. The
// fyne.io/systray fork exposes RunWithExternalLoop precisely for cases like
// ours where the host process already owns the main thread (Wails).
//
// The returned `stop` function tears down the status item; call it from the
// host's shutdown path.
func Start(cbs Callbacks) (start, stop func()) {
	onReady := func() {
		systray.SetTemplateIcon(templateIconBytes, regularIconBytes)
		systray.SetTooltip("SnapGo")

		mCapture := systray.AddMenuItem("Capture screenshot", "Take a region screenshot")
		mSettings := systray.AddMenuItem("Settings…", "Open settings window")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit SnapGo", "Quit the application")

		// Pump menu clicks on a dedicated goroutine. Channel sends from
		// systray are non-blocking, so a slow user callback does not back
		// up the UI thread.
		go func() {
			for {
				select {
				case <-mCapture.ClickedCh:
					if cbs.OnCapture != nil {
						cbs.OnCapture()
					}
				case <-mSettings.ClickedCh:
					if cbs.OnSettings != nil {
						cbs.OnSettings()
					}
				case <-mQuit.ClickedCh:
					if cbs.OnQuit != nil {
						cbs.OnQuit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}

	onExit := func() { /* nothing to clean up */ }

	// RunWithExternalLoop registers the status item without blocking the
	// caller — the returned `start` should be invoked once the host's
	// runloop is up so the icon appears. fyne.io/systray returns a
	// (start, end) pair; we delegate `end` to systray.Quit for symmetry.
	rawStart, _ := systray.RunWithExternalLoop(onReady, onExit)

	// IMPORTANT: on macOS, `nativeStart` ends up calling
	//   [NSStatusBar systemStatusBar] -> [-NSStatusBar _statusItemWithLength:...]
	// which constructs an NSWindow. Cocoa hard-asserts that NSWindow can
	// only be instantiated on the main thread. Wails v2 invokes OnStartup
	// from a worker goroutine, so calling rawStart() directly from there
	// crashes with NSInternalInconsistencyException. We wrap it in
	// dispatchOnMain so the call is forwarded to the main dispatch queue.
	start = func() { dispatchOnMain(rawStart) }
	stop = systray.Quit
	return start, stop
}
