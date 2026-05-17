//go:build darwin

package screencapture

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// macCapturer shells out to /usr/sbin/screencapture, which is bundled with
// macOS, requires no cgo, and is fully compatible with macOS 15/26 SDKs
// where CoreGraphics's display-capture API has been removed.
//
// Trade-off: a tiny process-spawn cost per capture (~30 ms). Acceptable for
// an interactive screenshot tool — users cannot perceive it.
type macCapturer struct{}

// New returns a macOS-native Capturer.
func New() Capturer { return &macCapturer{} }

// VirtualBounds returns the union of all displays measured in CSS pixels
// (i.e. logical, not Retina physical pixels). We query system_profiler via
// AppleScript-free shell utilities to avoid extra cgo dependencies.
//
// For simplicity we currently return the main display only; this matches
// what the WebView/window can actually paint without a separate transparent
// window per display. Multi-monitor support is a follow-up spec.
func (c *macCapturer) VirtualBounds() image.Rectangle {
	out, err := exec.Command(
		"system_profiler", "SPDisplaysDataType",
	).Output()
	if err == nil {
		// Look for "Resolution: 3024 x 1964" lines and take the first one.
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Resolution:") {
				continue
			}
			parts := strings.Fields(line)
			// Expected pattern: ["Resolution:", "3024", "x", "1964", ...]
			if len(parts) >= 4 {
				w, errW := strconv.Atoi(parts[1])
				h, errH := strconv.Atoi(parts[3])
				if errW == nil && errH == nil {
					// Resolution reported is physical; convert to CSS px by
					// halving for Retina (assume 2× when w >= 2560).
					if w >= 2560 {
						w /= 2
						h /= 2
					}
					return image.Rect(0, 0, w, h)
				}
			}
		}
	}
	// Conservative fallback used by ~all modern Macs.
	return image.Rect(0, 0, 1440, 900)
}

// CaptureRegion grabs the requested rectangle via the OS tool and returns
// the resulting PNG bytes.
//
// Rect coords here are logical screen coordinates, matching macOS
// screencapture's -R contract and the CSS-point coordinates reported by the
// transparent overlay.
func (c *macCapturer) CaptureRegion(rect image.Rectangle) ([]byte, error) {
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return nil, fmt.Errorf("invalid capture rectangle: %v", rect)
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("snapgo-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmp)

	region := fmt.Sprintf("%d,%d,%d,%d",
		rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(),
	)

	// -x: silent (no shutter sound)
	// -R: rectangle
	// -t png
	cmd := exec.Command("/usr/sbin/screencapture", "-x", "-R", region, "-t", "png", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("screencapture: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		return nil, fmt.Errorf("read screencapture output: %w", err)
	}
	return data, nil
}

// CaptureInteractive launches `screencapture -i` which presents the macOS
// native region selector (the same crosshair that Cmd+Shift+4 produces).
//
// Design rationale:
//   - The native picker is pixel-perfect across Retina / external displays
//     and supports multi-monitor out of the box.
//   - It also handles Esc / right-click to cancel without requiring us to
//     run a transparent overlay window — which previously caused the main
//     window to flash a grey full-screen and trap the user.
//   - When the user cancels, screencapture exits 0 but writes no file; we
//     detect that case via os.Stat and return ErrCancelled.
func (c *macCapturer) CaptureInteractive() ([]byte, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("snapgo-i-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmp)

	// -i: interactive region selection
	// -x: silent (suppress the camera shutter sound)
	// -t png: png output
	cmd := exec.Command("/usr/sbin/screencapture", "-i", "-x", "-t", "png", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("screencapture -i: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// User cancelled (Esc / right-click): tool exits 0 with no file written.
	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		return nil, ErrCancelled
	}
	return os.ReadFile(tmp)
}

// CaptureFullScreen runs `screencapture -x` to grab the primary display in
// a single shot. We use -m so only the main display is captured even if
// external monitors are attached — matches the "primary screen only" UX
// chosen for the in-app overlay.
func (c *macCapturer) CaptureFullScreen() ([]byte, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("snapgo-full-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmp)

	// -x: silent
	// -m: main display only
	// -t png: png output
	cmd := exec.Command("/usr/sbin/screencapture", "-x", "-m", "-t", "png", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("screencapture -x -m: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(tmp)
}
