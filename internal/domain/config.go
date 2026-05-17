// Package domain — configuration types.
//
// S3Config is intentionally split into its own file so that future providers
// can introduce their own configuration types side-by-side without polluting
// the core domain types file.
package domain

// S3Config describes the connection parameters for any S3-compatible
// endpoint (AWS S3, MinIO, Cloudflare R2, Backblaze B2, Aliyun OSS S3
// endpoint, etc.).
//
// Field design notes:
// - PathPrefix supports object name templating so users can group screenshots
//   by date.
// - PublicURLBase is optional to support CDN-fronted buckets; otherwise the
//   adapter falls back to "{Endpoint}/{Bucket}/{Key}" (path-style).
// - UsePathStyle defaults to true because many self-hosted MinIO / R2
//   deployments do not support virtual-hosted-style addressing.
type S3Config struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	PathPrefix      string `json:"pathPrefix"`
	PublicURLBase   string `json:"publicUrlBase"`
	UsePathStyle    bool   `json:"usePathStyle"`
}

// AppConfig is the top-level on-disk configuration document.
//
// We keep S3 nested so that adding more providers later (e.g. AliyunOSS, COS)
// only requires a new sibling field rather than a schema rewrite.
type AppConfig struct {
	// Hotkey describes the global shortcut that triggers a capture.
	// Stored as a human-readable string like "cmd+shift+a"; parsing happens
	// in the infrastructure hotkey adapter.
	Hotkey string `json:"hotkey"`

	// S3 holds the active S3-compatible storage configuration.
	S3 S3Config `json:"s3"`
}

// DefaultAppConfig returns sane zero-value defaults used on first launch.
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Hotkey: "cmd+shift+a",
		S3: S3Config{
			PathPrefix:   "snapgo/",
			UsePathStyle: true,
		},
	}
}

// IsS3Configured reports whether the user has filled the mandatory S3 fields.
func (c AppConfig) IsS3Configured() bool {
	return c.S3.Endpoint != "" &&
		c.S3.Bucket != "" &&
		c.S3.AccessKeyID != "" &&
		c.S3.SecretAccessKey != ""
}
