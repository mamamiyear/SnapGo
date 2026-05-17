//go:build darwin

package tray

import _ "embed"

// templateIconBytes embeds the macOS template icon as a multi-resolution TIFF
// generated from the 1x and 2x source PNGs in assets/.
//
//go:embed assets/statusbarTemplate.tiff
var templateIconBytes []byte

// regularIconBytes keeps a PNG fallback for APIs that expect a regular raster icon.
//
//go:embed assets/statusbarTemplate.png
var regularIconBytes []byte
