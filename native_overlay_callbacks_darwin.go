//go:build darwin

package main

import "C"

//export nativeOverlayConfirm
func nativeOverlayConfirm(x, y, w, h C.int, annotationsJSON *C.char) {
	nativeOverlayState.Lock()
	app := nativeOverlayState.app
	nativeOverlayState.app = nil
	nativeOverlayState.Unlock()
	if app == nil {
		return
	}
	rawAnnotations := C.GoString(annotationsJSON)
	go func() {
		_ = app.ConfirmNativeRegion(CaptureResult{
			Rect: RegionRect{
				X: int(x),
				Y: int(y),
				W: int(w),
				H: int(h),
			},
			Annotations: parseNativeAnnotations(rawAnnotations),
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
