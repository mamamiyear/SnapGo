// app.go wires the application service stack together and exposes
// frontend-callable methods through Wails bindings.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/mmmy/snapgo/internal/application"
	"github.com/mmmy/snapgo/internal/domain"
	"github.com/mmmy/snapgo/internal/infrastructure/clipboard"
	"github.com/mmmy/snapgo/internal/infrastructure/config"
	"github.com/mmmy/snapgo/internal/infrastructure/display"
	"github.com/mmmy/snapgo/internal/infrastructure/hotkey"
	"github.com/mmmy/snapgo/internal/infrastructure/oss"
	"github.com/mmmy/snapgo/internal/infrastructure/screencapture"
	sshpkg "github.com/mmmy/snapgo/internal/infrastructure/ssh"
)

// App is the struct exposed to the Wails frontend through Bind.
//
// We deliberately keep this struct small: it owns long-lived collaborators
// (config store, hotkey manager, capturer, clipboard) but delegates the
// real work to the application service constructed on demand once an OSS
// provider is configured.
type App struct {
	ctx context.Context

	mu         sync.RWMutex
	cfg        domain.AppConfig
	configFile *config.FileStore
	hotkeyMgr  *hotkey.Manager
	capturer   screencapture.Capturer
	clip       clipboard.Writer

	// capturing prevents re-entrant capture sessions when the user mashes
	// the hotkey while a previous one is still running. It stays true for
	// the entire lifecycle of one capture: from overlay start until the
	// overlay is either confirmed (ConfirmRegion) or discarded (CancelRegion).
	capturing atomic.Bool

	// pendingMu guards `pending`. We keep this separate from `mu` so that
	// frontend-triggered RPC handlers (which may run concurrently with the
	// capture goroutine) can take it cheaply.
	pendingMu sync.Mutex
	pending   *pendingCapture
}

// pendingCapture holds the OSS provider chosen when capture started.
// Keeping the provider here prevents a config edit
// between "snap" and "confirm" from silently retargeting the upload.
type pendingCapture struct {
	Provider domain.OSSProvider
	Display  display.Info
}

// OverlayPayload is the JSON the Go side emits so the WebView can build the
// Snipaste-style overlay. It deliberately does not contain screenshot bytes:
// the window is transparent and the mask sits over the live desktop.
type OverlayPayload struct {
	CSSWidth  int     `json:"cssWidth"`
	CSSHeight int     `json:"cssHeight"`
	Scale     float64 `json:"scale"`
}

// RegionRect is the CSS-pixel selection rectangle the WebView reports.
// Origin is the top-left of the primary display.
type RegionRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// CaptureResult is returned by the overlay with the final selection and any
// annotations drawn inside the selected area.
type CaptureResult struct {
	Rect        RegionRect               `json:"rect"`
	Annotations []application.Annotation `json:"annotations"`
}

// CaptureActionResult tells the frontend whether an action was completed or
// the user cancelled a secondary choice such as the save destination dialog.
type CaptureActionResult struct {
	Completed bool   `json:"completed"`
	Path      string `json:"path,omitempty"`
}

// NewApp creates a new App with collaborators already initialised.
func NewApp() *App {
	store, err := config.NewFileStore()
	if err != nil {
		slog.Error("config store init", "err", err)
	}
	cfg := domain.DefaultAppConfig()
	if store != nil {
		if loaded, lerr := store.Load(); lerr == nil {
			cfg = loaded
		} else {
			slog.Warn("config load failed, using defaults", "err", lerr)
		}
	}
	return &App{
		cfg:        cfg,
		configFile: store,
		hotkeyMgr:  hotkey.NewManager(),
		capturer:   screencapture.New(),
		clip:       clipboard.New(),
	}
}

// startup is invoked by Wails once the runtime is available.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.registerHotkey(a.cfg.Hotkey); err != nil {
		slog.Warn("register hotkey failed", "err", err)
		wruntime.EventsEmit(a.ctx, "hotkey:error", err.Error())
	} else {
		wruntime.EventsEmit(a.ctx, "hotkey:ready", a.cfg.Hotkey)
	}
}

// shutdown releases OS resources.
func (a *App) shutdown(_ context.Context) {
	a.hotkeyMgr.Unregister()
}

// ---------------------------------------------------------------------------
// Hotkey management
// ---------------------------------------------------------------------------

func (a *App) registerHotkey(spec string) error {
	if spec == "" {
		spec = "cmd+shift+a"
	}
	return a.hotkeyMgr.Register(spec, func() {
		a.runInteractiveCapture()
	})
}

// runInteractiveCapture is the unified entry point for the global hotkey
// and the in-app "Capture now" button. The new flow is:
//
//  1. Hide the main window so it never flashes over the desktop.
//  2. Re-show it as a transparent, high-level overlay that paints only a
//     dimming mask plus selection chrome over the live screen.
//  3. Wait — the user finishes selecting in the WebView, which calls
//     ConfirmRegion(rect) or CancelRegion() back into us.
//
// We deliberately do NOT block the goroutine on a channel here: the
// "wait" is implicit. capturing stays true until the frontend resolves it.
func (a *App) runInteractiveCapture() {
	if a.ctx == nil {
		return
	}
	if !a.capturing.CompareAndSwap(false, true) {
		return
	}
	releaseOnExit := true
	defer func() {
		if releaseOnExit {
			a.capturing.Store(false)
		}
	}()

	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()

	var provider domain.OSSProvider
	if cfg.IsS3Configured() {
		var err error
		provider, err = oss.NewS3Provider(cfg.S3)
		if err != nil {
			wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		}
	}

	// Hide the settings window before transforming it into an overlay, so
	// the user only sees the final transparent mask state.
	wruntime.WindowHide(a.ctx)
	hideDockIcon()
	flushFrame()
	wruntime.EventsEmit(a.ctx, "capture:start", nil)

	info := display.Primary()

	// Park the provider and morph the window into a transparent overlay.
	a.pendingMu.Lock()
	a.pending = &pendingCapture{
		Provider: provider,
		Display:  info,
	}
	a.pendingMu.Unlock()

	if showNativeCaptureOverlay(a, info) {
		releaseOnExit = false // ownership passes to the native overlay callbacks
		return
	}

	payload := OverlayPayload{
		CSSWidth:  info.CSSWidth,
		CSSHeight: info.CSSHeight,
		Scale:     info.Scale,
	}
	a.showOverlayWindow(info)
	wruntime.EventsEmit(a.ctx, "capture:overlay", payload)
	flushFrame()
	wruntime.WindowShow(a.ctx)
	configureOverlayWindow()

	releaseOnExit = false // ownership of `capturing` passes to the overlay lifecycle
}

// ---------------------------------------------------------------------------
// Overlay window helpers
// ---------------------------------------------------------------------------

const (
	settingsWidth  = 1080
	settingsHeight = 720
)

// showOverlayWindow prepares the hidden main window as a borderless,
// always-on-top, transparent full-primary-display window. The caller emits
// the overlay event and only then shows it, avoiding an opaque grey flash.
func (a *App) showOverlayWindow(info display.Info) {
	if a.ctx == nil {
		return
	}
	wruntime.WindowSetBackgroundColour(a.ctx, 0, 0, 0, 0)
	wruntime.WindowSetAlwaysOnTop(a.ctx, true)
	wruntime.WindowSetSize(a.ctx, info.CSSWidth, info.CSSHeight)
	wruntime.WindowSetPosition(a.ctx, 0, 0)
	configureOverlayWindow()
}

// restoreSettingsWindow flips the window back from overlay form to its
// normal Settings size and unpins it. We do NOT call Show here — callers
// decide whether the user actually wanted Settings to appear.
func (a *App) restoreSettingsWindow() {
	if a.ctx == nil {
		return
	}
	restoreOverlayWindow()
	wruntime.WindowSetBackgroundColour(a.ctx, 246, 247, 250, 255)
	wruntime.WindowSetAlwaysOnTop(a.ctx, false)
	wruntime.WindowSetSize(a.ctx, settingsWidth, settingsHeight)
	wruntime.WindowCenter(a.ctx)
}

// surfaceWindow brings the main window back from a hidden state regardless
// of which mechanism we used to suppress it.
func (a *App) surfaceWindow() {
	if a.ctx == nil {
		return
	}
	showDockIcon(true)
	a.restoreSettingsWindow()
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
}

// dismissOverlay hides the overlay window and shrinks/centres it back to
// the Settings dimensions so a future ShowWindow lands in the right place.
func (a *App) dismissOverlay() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowHide(a.ctx)
	hideDockIcon()
	a.restoreSettingsWindow()
}

// runUploadPipeline is the post-capture half of the workflow. Shared by
// the hotkey path and the explicit "Upload now" RPC.
func (a *App) runUploadPipeline(provider domain.OSSProvider, pngBytes []byte) error {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()

	svc := &application.CaptureAndUploadService{
		Capturer:    a.capturer,
		Provider:    provider,
		Clipboard:   a.clip,
		Notifier:    &runtimeNotifier{ctx: a.ctx},
		FallbackDir: filepath.Join(userPicturesDir(), "SnapGo"),
		PathPrefix:  cfg.S3.PathPrefix,
	}
	return svc.ExecuteWithBytes(a.ctx, pngBytes)
}

func (a *App) runCopyImagePipeline(pngBytes []byte) error {
	svc := &application.CaptureActionsService{
		Clipboard: a.clip,
		Notifier:  &runtimeNotifier{ctx: a.ctx},
	}
	return svc.CopyImage(a.ctx, pngBytes)
}

func (a *App) runSaveImagePipeline(pngBytes []byte, dir string) (string, error) {
	svc := &application.CaptureActionsService{
		Clipboard: a.clip,
		Notifier:  &runtimeNotifier{ctx: a.ctx},
	}
	return svc.SaveImage(a.ctx, pngBytes, dir)
}

// runSaveRemotePipeline uploads the captured PNG to the configured SSH host
// via SCP and copies a sharable reference (URL or "user@host:~/path") to
// the clipboard.
//
// Why a dedicated method (vs. extending CaptureAndUploadService): SSH and
// S3 have different config + clipboard semantics, so keeping them separate
// avoids leaking provider-specific branches into the OSS pipeline.
func (a *App) runSaveRemotePipeline(pngBytes []byte) error {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()

	if !cfg.IsSSHConfigured() {
		err := fmt.Errorf("SSH host/user is not configured")
		slog.Warn("save-remote rejected: ssh not configured",
			"host", cfg.SSH.Host, "user", cfg.SSH.User)
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	slog.Info("save-remote dispatch",
		"host", cfg.SSH.Host, "user", cfg.SSH.User, "port", cfg.SSH.Port,
		"png_size", len(pngBytes))

	svc := &application.CaptureAndSSHService{
		Uploader:  sshpkg.NewUploader(cfg.SSH),
		Clipboard: a.clip,
		Notifier:  &runtimeNotifier{ctx: a.ctx},
		Cfg:       cfg.SSH,
	}
	return svc.ExecuteWithBytes(a.ctx, pngBytes)
}

func (a *App) consumePendingCapture() (*pendingCapture, error) {
	a.pendingMu.Lock()
	pc := a.pending
	a.pending = nil
	a.pendingMu.Unlock()

	if pc == nil {
		return nil, fmt.Errorf("no pending capture")
	}
	return pc, nil
}

func (a *App) captureSelectedPNG(result CaptureResult, pc *pendingCapture) ([]byte, error) {
	rect := result.Rect
	captureRect := image.Rect(rect.X, rect.Y, rect.X+rect.W, rect.Y+rect.H)
	cropped, err := a.capturer.CaptureRegion(captureRect)
	if err != nil {
		return nil, err
	}
	if len(result.Annotations) > 0 {
		cropped, err = application.ApplyAnnotations(cropped, result.Annotations, pc.Display.Scale)
		if err != nil {
			return nil, err
		}
	}
	return cropped, nil
}

func (a *App) chooseSaveDirectory() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:                "Save screenshot to folder",
		DefaultDirectory:     userPicturesDir(),
		CanCreateDirectories: true,
	})
}

// ---------------------------------------------------------------------------
// Bound methods (called from the frontend via Wails)
// ---------------------------------------------------------------------------

// GetConfig returns the current persisted configuration.
func (a *App) GetConfig() domain.AppConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// SaveConfig persists the supplied configuration and re-registers the hotkey
// if it changed.
func (a *App) SaveConfig(cfg domain.AppConfig) error {
	a.mu.Lock()
	prev := a.cfg
	a.cfg = cfg
	a.mu.Unlock()

	if a.configFile != nil {
		if err := a.configFile.Save(cfg); err != nil {
			return err
		}
	}
	if cfg.Hotkey != prev.Hotkey {
		if err := a.registerHotkey(cfg.Hotkey); err != nil {
			wruntime.EventsEmit(a.ctx, "hotkey:error", err.Error())
			return fmt.Errorf("register new hotkey: %w", err)
		}
		wruntime.EventsEmit(a.ctx, "hotkey:ready", cfg.Hotkey)
	}
	return nil
}

// RetryRegisterHotkey re-registers the configured hotkey on demand.
func (a *App) RetryRegisterHotkey() error {
	a.mu.RLock()
	spec := a.cfg.Hotkey
	a.mu.RUnlock()
	if err := a.registerHotkey(spec); err != nil {
		wruntime.EventsEmit(a.ctx, "hotkey:error", err.Error())
		return err
	}
	wruntime.EventsEmit(a.ctx, "hotkey:ready", spec)
	return nil
}

// TestConnection performs a put+delete probe against the supplied S3 config.
func (a *App) TestConnection(cfg domain.S3Config) error {
	provider, err := oss.NewS3Provider(cfg)
	if err != nil {
		return err
	}
	return provider.TestConnection(a.ctx)
}

// TestSSHConnection performs a minimal SSH handshake probe against the
// supplied configuration so the SettingsView's "Test connection" button
// can give the user immediate feedback.
func (a *App) TestSSHConnection(cfg domain.SSHConfig) error {
	slog.Info("RPC TestSSHConnection",
		"host", cfg.Host, "user", cfg.User, "port", cfg.Port,
		"strict_host_key", cfg.StrictHostKey, "has_password", cfg.Password != "")
	if err := sshpkg.TestConnection(a.ctx, cfg); err != nil {
		slog.Error("RPC TestSSHConnection failed", "err", err)
		return err
	}
	return nil
}

// CaptureNow is the in-app trigger.
func (a *App) CaptureNow() {
	go a.runInteractiveCapture()
}

// ConfirmRegion is invoked by the overlay UI when the user clicks
// "Upload & copy". The overlay is transparent and sits over the live
// desktop, so we hide it first, then ask the OS to capture the selected
// logical screen rectangle directly.
//
// Returning the error to the frontend lets the overlay decide whether to
// keep showing the screenshot (e.g. for a retry) — currently it just
// dismisses regardless and surfaces the error via the upload:failure toast.
func (a *App) ConfirmRegion(result CaptureResult) error {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return err
	}
	defer func() {
		a.capturing.Store(false)
		a.dismissOverlay()
	}()

	a.dismissOverlay()
	flushFrame()

	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	if err := a.runUploadPipeline(pc.Provider, cropped); err != nil {
		return err
	}
	return nil
}

// CopyRegionImage is invoked by the fallback Wails overlay when the user
// double-clicks the selected area.
func (a *App) CopyRegionImage(result CaptureResult) error {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return err
	}
	defer func() {
		a.capturing.Store(false)
		a.dismissOverlay()
	}()

	a.dismissOverlay()
	flushFrame()

	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	return a.runCopyImagePipeline(cropped)
}

// SaveRegionImage lets the fallback Wails overlay pick a native destination
// folder and writes the selected screenshot PNG there.
func (a *App) SaveRegionImage(result CaptureResult) (CaptureActionResult, error) {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return CaptureActionResult{}, err
	}

	a.dismissOverlay()
	flushFrame()

	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		a.capturing.Store(false)
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return CaptureActionResult{}, err
	}

	dir, err := a.chooseSaveDirectory()
	if err != nil {
		a.capturing.Store(false)
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return CaptureActionResult{}, err
	}
	if dir == "" {
		a.capturing.Store(false)
		a.dismissOverlay()
		return CaptureActionResult{Completed: false}, nil
	}

	path, err := a.runSaveImagePipeline(cropped, dir)
	a.capturing.Store(false)
	a.dismissOverlay()
	if err != nil {
		return CaptureActionResult{}, err
	}
	return CaptureActionResult{Completed: true, Path: path}, nil
}

// ConfirmNativeRegion mirrors ConfirmRegion for the macOS native overlay.
// The native AppKit panel has already been closed by the time this method is
// called, so we must not dismiss/restore the Wails overlay window here.
func (a *App) ConfirmNativeRegion(result CaptureResult) error {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return err
	}
	defer func() {
		a.capturing.Store(false)
		hideDockIcon()
	}()

	flushFrame()
	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	return a.runUploadPipeline(pc.Provider, cropped)
}

func (a *App) CopyNativeRegionImage(result CaptureResult) error {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return err
	}
	defer func() {
		a.capturing.Store(false)
		hideDockIcon()
	}()

	flushFrame()
	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	return a.runCopyImagePipeline(cropped)
}

func (a *App) SaveNativeRegionImage(result CaptureResult) (CaptureActionResult, error) {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return CaptureActionResult{}, err
	}
	defer func() {
		a.capturing.Store(false)
		hideDockIcon()
	}()

	flushFrame()
	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return CaptureActionResult{}, err
	}
	dir, err := a.chooseSaveDirectory()
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return CaptureActionResult{}, err
	}
	if dir == "" {
		return CaptureActionResult{Completed: false}, nil
	}
	path, err := a.runSaveImagePipeline(cropped, dir)
	if err != nil {
		return CaptureActionResult{}, err
	}
	return CaptureActionResult{Completed: true, Path: path}, nil
}

func (a *App) SaveNativeRegionImageToDir(result CaptureResult, dir string) (CaptureActionResult, error) {
	pc, err := a.consumePendingCapture()
	if err != nil {
		return CaptureActionResult{}, err
	}
	defer func() {
		a.capturing.Store(false)
		hideDockIcon()
	}()

	if dir == "" {
		return CaptureActionResult{Completed: false}, nil
	}
	flushFrame()
	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return CaptureActionResult{}, err
	}
	path, err := a.runSaveImagePipeline(cropped, dir)
	if err != nil {
		return CaptureActionResult{}, err
	}
	return CaptureActionResult{Completed: true, Path: path}, nil
}

// SaveRegionToRemote uploads the user-selected region via SCP to the
// configured SSH host. Driven by the Wails overlay's "save remote" button.
//
// The flow mirrors ConfirmRegion (S3 upload) but routes through
// runSaveRemotePipeline instead. We dismiss the overlay first so the
// upload progress (and any error toast) shows over the regular settings UI.
func (a *App) SaveRegionToRemote(result CaptureResult) error {
	pc, err := a.consumePendingCapture()
	if err != nil {
		slog.Warn("SaveRegionToRemote: no pending capture", "err", err)
		return err
	}
	defer func() {
		a.capturing.Store(false)
		a.dismissOverlay()
	}()

	a.dismissOverlay()
	flushFrame()

	slog.Info("SaveRegionToRemote: capturing region",
		"x", result.Rect.X, "y", result.Rect.Y,
		"w", result.Rect.W, "h", result.Rect.H,
		"annotations", len(result.Annotations))
	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		slog.Error("SaveRegionToRemote: capture failed", "err", err)
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	slog.Debug("SaveRegionToRemote: capture ok", "png_bytes", len(cropped))
	return a.runSaveRemotePipeline(cropped)
}

// SaveNativeRegionToRemote is the macOS-native overlay equivalent of
// SaveRegionToRemote. The AppKit panel is already closed by the time this
// runs (see saveRemoteSelection in native_overlay_darwin.go), so we only
// have to release the dock icon afterwards.
func (a *App) SaveNativeRegionToRemote(result CaptureResult) error {
	pc, err := a.consumePendingCapture()
	if err != nil {
		slog.Warn("SaveNativeRegionToRemote: no pending capture", "err", err)
		return err
	}
	defer func() {
		a.capturing.Store(false)
		hideDockIcon()
	}()

	flushFrame()
	slog.Info("SaveNativeRegionToRemote: capturing region",
		"x", result.Rect.X, "y", result.Rect.Y,
		"w", result.Rect.W, "h", result.Rect.H,
		"annotations", len(result.Annotations))
	cropped, err := a.captureSelectedPNG(result, pc)
	if err != nil {
		slog.Error("SaveNativeRegionToRemote: capture failed", "err", err)
		wruntime.EventsEmit(a.ctx, "upload:failure", err.Error())
		return err
	}
	slog.Debug("SaveNativeRegionToRemote: capture ok", "png_bytes", len(cropped))
	return a.runSaveRemotePipeline(cropped)
}

func parseNativeAnnotations(raw string) []application.Annotation {
	if raw == "" {
		return nil
	}
	var annotations []application.Annotation
	if err := json.Unmarshal([]byte(raw), &annotations); err != nil {
		slog.Warn("parse native annotations failed", "err", err)
		return nil
	}
	return annotations
}

// CancelRegion is invoked when the user dismisses the overlay (Esc /
// Cancel button / right-click). Frees the pending PNG, releases the
// capture lock, and hides the overlay.
func (a *App) CancelRegion() {
	a.pendingMu.Lock()
	a.pending = nil
	a.pendingMu.Unlock()
	a.capturing.Store(false)
	a.dismissOverlay()
}

// CancelNativeRegion releases native-overlay state without touching the
// hidden Wails settings window.
func (a *App) CancelNativeRegion() {
	a.pendingMu.Lock()
	a.pending = nil
	a.pendingMu.Unlock()
	a.capturing.Store(false)
	hideDockIcon()
}

// ShowWindow brings the main window back to the foreground in its normal
// Settings shape. Used by the tray "Settings…" menu item.
func (a *App) ShowWindow() {
	a.surfaceWindow()
	time.Sleep(20 * time.Millisecond)
}

// QuitApp terminates the entire process. Wired to the tray "Quit" menu.
func (a *App) QuitApp() {
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
		return
	}
	os.Exit(0)
}

// runtimeNotifier emits success / failure events via the Wails runtime.
type runtimeNotifier struct{ ctx context.Context }

func (n *runtimeNotifier) NotifySuccess(url string) {
	wruntime.EventsEmit(n.ctx, "upload:success", url)
}

func (n *runtimeNotifier) NotifyFailure(reason string) {
	wruntime.EventsEmit(n.ctx, "upload:failure", reason)
}

// userPicturesDir returns ~/Pictures (or HOME if unavailable).
func userPicturesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, "Pictures")
}
