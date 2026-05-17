//go:build darwin

package tray

/*
#cgo darwin LDFLAGS: -framework Foundation
#include <dispatch/dispatch.h>

// runOnMainQueue submits the Go-side callback (identified by handle) to the
// main dispatch queue. We use dispatch_async so the caller — typically a
// Wails OnStartup goroutine — does not block the cocoa runloop while waiting
// for the main thread to drain.
extern void trayDispatchMainCallback(unsigned long handle);

static void runOnMainQueue(unsigned long handle) {
    dispatch_async(dispatch_get_main_queue(), ^{
        trayDispatchMainCallback(handle);
    });
}
*/
import "C"

import (
	"sync"
	"sync/atomic"
)

// dispatchRegistry stores Go closures keyed by an integer handle so that the
// C side can call back into Go without leaking unsafe.Pointer-cast function
// pointers (which CGo disallows).
var (
	dispatchMu     sync.Mutex
	dispatchTable  = map[uint64]func(){}
	dispatchNextID uint64
)

// dispatchOnMain schedules fn to run on the macOS main thread. This is the
// Cocoa contract for any API that touches NSWindow / NSStatusBar — calling
// them from a goroutine triggers the "should only be instantiated on the
// main thread" assertion.
func dispatchOnMain(fn func()) {
	if fn == nil {
		return
	}
	id := atomic.AddUint64(&dispatchNextID, 1)
	dispatchMu.Lock()
	dispatchTable[id] = fn
	dispatchMu.Unlock()
	C.runOnMainQueue(C.ulong(id))
}

//export trayDispatchMainCallback
func trayDispatchMainCallback(handle C.ulong) {
	id := uint64(handle)
	dispatchMu.Lock()
	fn := dispatchTable[id]
	delete(dispatchTable, id)
	dispatchMu.Unlock()
	if fn != nil {
		fn()
	}
}
