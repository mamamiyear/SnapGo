//go:build darwin

package hotkey

import hk "golang.design/x/hotkey"

// macOS uses ⌘ as the primary modifier; Option corresponds to Alt.
func modCmd() hk.Modifier    { return hk.ModCmd }
func modCtrl() hk.Modifier   { return hk.ModCtrl }
func modOption() hk.Modifier { return hk.ModOption }
