//go:build !darwin

package hotkey

import hk "golang.design/x/hotkey"

// On Windows / Linux we map "cmd" to Ctrl so the same config string works
// across platforms; "option" is treated as Alt.
func modCmd() hk.Modifier    { return hk.ModCtrl }
func modCtrl() hk.Modifier   { return hk.ModCtrl }
func modOption() hk.Modifier { return hk.ModAlt }
