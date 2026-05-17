//go:build !darwin

package tray

import _ "embed"

// Keep a regular PNG for non-macOS builds where template icons are unsupported.
//
//go:embed assets/statusbarTemplate.png
var templateIconBytes []byte

//go:embed assets/statusbarTemplate.png
var regularIconBytes []byte
