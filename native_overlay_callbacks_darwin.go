//go:build darwin

package main

import "C"

func nativeCaptureResult(x, y, w, h C.int, annotationsJSON *C.char) CaptureResult {
	rawAnnotations := C.GoString(annotationsJSON)
	return CaptureResult{
		Rect: RegionRect{
			X: int(x),
			Y: int(y),
			W: int(w),
			H: int(h),
		},
		Annotations: parseNativeAnnotations(rawAnnotations),
	}
}

func consumeNativeOverlayApp() *App {
	nativeOverlayState.Lock()
	app := nativeOverlayState.app
	nativeOverlayState.app = nil
	nativeOverlayState.Unlock()
	return app
}

//export nativeOverlayConfirm
func nativeOverlayConfirm(x, y, w, h C.int, annotationsJSON *C.char) {
	app := consumeNativeOverlayApp()
	if app == nil {
		return
	}
	result := nativeCaptureResult(x, y, w, h, annotationsJSON)
	go func() {
		_ = app.ConfirmNativeRegion(result)
	}()
}

//export nativeOverlayCopy
func nativeOverlayCopy(x, y, w, h C.int, annotationsJSON *C.char) {
	app := consumeNativeOverlayApp()
	if app == nil {
		return
	}
	result := nativeCaptureResult(x, y, w, h, annotationsJSON)
	go func() {
		_ = app.CopyNativeRegionImage(result)
	}()
}

//export nativeOverlaySave
func nativeOverlaySave(x, y, w, h C.int, annotationsJSON *C.char, dir *C.char) {
	app := consumeNativeOverlayApp()
	if app == nil {
		return
	}
	result := nativeCaptureResult(x, y, w, h, annotationsJSON)
	saveDir := C.GoString(dir)
	go func() {
		_, _ = app.SaveNativeRegionImageToDir(result, saveDir)
	}()
}

//export nativeOverlaySaveRemote
func nativeOverlaySaveRemote(x, y, w, h C.int, annotationsJSON *C.char) {
	app := consumeNativeOverlayApp()
	if app == nil {
		return
	}
	result := nativeCaptureResult(x, y, w, h, annotationsJSON)
	go func() {
		_ = app.SaveNativeRegionToRemote(result)
	}()
}

//export nativeOverlayCancel
func nativeOverlayCancel() {
	app := consumeNativeOverlayApp()
	if app == nil {
		return
	}
	go app.CancelNativeRegion()
}
