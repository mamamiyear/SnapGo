// Package clipboard wraps golang.design/x/clipboard with lazy initialization.
//
// Why a wrapper:
//   - The upstream library requires a one-time clipboard.Init() call. Hiding it
//     here keeps the application service free of init concerns.
//   - Allows future swap to a different backend (e.g. atotto/clipboard) without
//     changing callers.
package clipboard

import (
	"fmt"
	"sync"

	"golang.design/x/clipboard"
)

// Writer abstracts clipboard writes for testability.
type Writer interface {
	WriteText(s string) error
	WriteImage(pngBytes []byte) error
}

type clipWriter struct {
	once    sync.Once
	initErr error
}

// New returns a process-wide clipboard writer. Initialization is lazy.
func New() Writer {
	return &clipWriter{}
}

func (w *clipWriter) ensureInit() error {
	w.once.Do(func() {
		w.initErr = clipboard.Init()
	})
	return w.initErr
}

// WriteText replaces the clipboard contents with the supplied UTF-8 string.
func (w *clipWriter) WriteText(s string) error {
	if err := w.ensureInit(); err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}
	clipboard.Write(clipboard.FmtText, []byte(s))
	return nil
}

// WriteImage replaces the clipboard contents with PNG-encoded image data.
func (w *clipWriter) WriteImage(pngBytes []byte) error {
	if err := w.ensureInit(); err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}
	clipboard.Write(clipboard.FmtImage, pngBytes)
	return nil
}
