// Package domain defines the core business types of SnapGo.
//
// Design rationale:
// - Keep this layer free of any third-party SDK or OS API.
// - Higher layers (application, infrastructure) depend on these types,
//   but this package depends on nothing except the Go standard library.
// - Encapsulating data here makes the upgrade path to multiple OSS providers
//   (Aliyun OSS, Qiniu, Tencent COS, etc.) trivial — only a new infra adapter
//   is needed.
package domain

import (
	"context"
	"image"
	"time"
)

// Screenshot represents a single captured image in PNG-encoded byte form.
//
// We hold the encoded bytes (not image.Image) because:
// 1. Uploaders need a byte stream;
// 2. Local fallback writes the same bytes to disk;
// 3. Avoids re-encoding cost.
type Screenshot struct {
	PNG       []byte          // PNG-encoded payload
	Region    image.Rectangle // The captured region in virtual screen coords
	Width     int             // Logical width of the region
	Height    int             // Logical height of the region
	CreatedAt time.Time       // When the capture happened
}

// UploadResult describes the outcome of a successful upload.
type UploadResult struct {
	URL      string        // Public URL that was copied to the clipboard
	Key      string        // Object key on the remote storage
	Provider string        // Provider name, e.g. "s3"
	Elapsed  time.Duration // How long the upload took
}

// OSSProvider is the abstraction every storage adapter MUST implement.
//
// Why an interface:
// - Allows the application layer to remain agnostic of the concrete SDK.
// - Future providers (Aliyun OSS / COS / Qiniu) only need a new adapter
//   without touching the service layer.
type OSSProvider interface {
	// Upload pushes data with the given key and content-type to remote storage,
	// returning the publicly accessible URL on success.
	Upload(ctx context.Context, key string, data []byte, contentType string) (publicURL string, err error)

	// Name returns a stable identifier of the provider implementation
	// (used in logs, telemetry, history records).
	Name() string
}
