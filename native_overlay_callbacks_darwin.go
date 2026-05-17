//go:build darwin

package main

import "C"

//export nativeOverlayConfirm
func nativeOverlayConfirm(x, y, w, h C.int) {
	nativeOverlayState.Lock()
	app := nativeOverlayState.app
	nativeOverlayState.app = nil
	nativeOverlayState.Unlock()
	if app == nil {
		return
	}
	go func() {
		_ = app.ConfirmNativeRegion(RegionRect{
			X: int(x),
			Y: int(y),
			W: int(w),
			H: int(h),
		})
	}()
}

//export nativeOverlayCancel
func nativeOverlayCancel() {
	nativeOverlayState.Lock()
	app := nativeOverlayState.app
	nativeOverlayState.app = nil
	nativeOverlayState.Unlock()
	if app == nil {
		return
	}
	go app.CancelNativeRegion()
}
