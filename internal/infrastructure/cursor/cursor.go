// Package cursor exposes the current mouse pointer position in a
// platform-agnostic way. We use it to anchor the post-capture toolbar near
// the location where the user released the mouse — on macOS that point is
// effectively the bottom-right corner of the screencapture selection.
package cursor

// Point is a screen-space pointer location, in *logical* pixels (i.e. the
// coordinate system Wails uses for window placement).
type Point struct {
	X int
	Y int
}
