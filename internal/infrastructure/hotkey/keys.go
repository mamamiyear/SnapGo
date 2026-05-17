package hotkey

import hk "golang.design/x/hotkey"

// lookupKey maps a token from the user-supplied hotkey spec to a hk.Key.
//
// The map is intentionally exhaustive over the alphanumeric set so we never
// rely on an assumption that hk.KeyA..hk.KeyZ are contiguous values — the
// upstream package gives no such guarantee.
func lookupKey(token string) (hk.Key, bool) {
	switch token {
	case "a":
		return hk.KeyA, true
	case "b":
		return hk.KeyB, true
	case "c":
		return hk.KeyC, true
	case "d":
		return hk.KeyD, true
	case "e":
		return hk.KeyE, true
	case "f":
		return hk.KeyF, true
	case "g":
		return hk.KeyG, true
	case "h":
		return hk.KeyH, true
	case "i":
		return hk.KeyI, true
	case "j":
		return hk.KeyJ, true
	case "k":
		return hk.KeyK, true
	case "l":
		return hk.KeyL, true
	case "m":
		return hk.KeyM, true
	case "n":
		return hk.KeyN, true
	case "o":
		return hk.KeyO, true
	case "p":
		return hk.KeyP, true
	case "q":
		return hk.KeyQ, true
	case "r":
		return hk.KeyR, true
	case "s":
		return hk.KeyS, true
	case "t":
		return hk.KeyT, true
	case "u":
		return hk.KeyU, true
	case "v":
		return hk.KeyV, true
	case "w":
		return hk.KeyW, true
	case "x":
		return hk.KeyX, true
	case "y":
		return hk.KeyY, true
	case "z":
		return hk.KeyZ, true
	case "0":
		return hk.Key0, true
	case "1":
		return hk.Key1, true
	case "2":
		return hk.Key2, true
	case "3":
		return hk.Key3, true
	case "4":
		return hk.Key4, true
	case "5":
		return hk.Key5, true
	case "6":
		return hk.Key6, true
	case "7":
		return hk.Key7, true
	case "8":
		return hk.Key8, true
	case "9":
		return hk.Key9, true
	}
	return 0, false
}
