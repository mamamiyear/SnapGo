//go:build !darwin

package cursor

// Get is a no-op stub on non-macOS targets. The toolbar will fall back to
// the centre of the screen on these platforms.
func Get() (Point, error) {
	return Point{X: 0, Y: 0}, nil
}
