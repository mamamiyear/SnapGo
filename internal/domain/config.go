// Package domain — configuration types.
//
// S3Config and SSHConfig are intentionally split into their own structs so
// that future providers can introduce their own configuration types side-
// by-side without polluting the core domain types file.
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

// SSHConfig describes the parameters required to upload a screenshot via
// SSH/SCP to a remote host.
//
// Field design notes:
//   - Host / Port form the destination endpoint. Port defaults to 22 when 0.
//   - User is the SSH login name.
//   - Password is optional; when empty the implementation falls back to the
//     local SSH agent or ~/.ssh/id_* keys (i.e. the user is responsible for
//     having password-less access already configured on this machine).
//   - PathPrefix is interpreted *relative to the remote user's $HOME*. The
//     UI presents it as "~/<PathPrefix>" so the user cannot escape the home
//     directory by writing absolute paths. Leading "/" or "~" markers are
//     stripped before the path is used.
//   - StrictHostKey toggles host key verification. When false the client
//     uses ssh.InsecureIgnoreHostKey() to mimic `scp -o StrictHostKeyChecking=no`,
//     trading some security for first-launch usability on personal LANs.
//   - KnownHostsPath defaults to ~/.ssh/known_hosts when empty and is only
//     used while StrictHostKey is true.
//   - ConnectTimeoutSecs caps the network handshake duration so a wrong
//     endpoint does not hang the capture flow indefinitely.
type SSHConfig struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	User               string `json:"user"`
	Password           string `json:"password"`
	PathPrefix         string `json:"pathPrefix"`
	StrictHostKey      bool   `json:"strictHostKey"`
	KnownHostsPath     string `json:"knownHostsPath"`
	ConnectTimeoutSecs int    `json:"connectTimeoutSecs"`
}

// AppConfig is the top-level on-disk configuration document.
//
// We keep S3 / SSH nested so that adding more providers later (e.g. AliyunOSS,
// COS, FTP) only requires a new sibling field rather than a schema rewrite.
type AppConfig struct {
	// Hotkey describes the global shortcut that triggers a capture.
	// Stored as a human-readable string like "cmd+shift+a"; parsing happens
	// in the infrastructure hotkey adapter.
	Hotkey string `json:"hotkey"`

	// S3 holds the active S3-compatible storage configuration.
	S3 S3Config `json:"s3"`

	// SSH holds the configuration for the optional "save to remote via scp"
	// destination triggered by the save-remote toolbar button.
	SSH SSHConfig `json:"ssh"`
}

// DefaultAppConfig returns sane zero-value defaults used on first launch.
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Hotkey: "cmd+shift+a",
		S3: S3Config{
			PathPrefix:   "snapgo/",
			UsePathStyle: true,
		},
		SSH: SSHConfig{
			Port:               22,
			PathPrefix:         "snapgo/",
			ConnectTimeoutSecs: 10,
			StrictHostKey:      false,
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

// IsSSHConfigured reports whether the SSH destination has the minimum
// fields required to attempt a connection. Password is intentionally NOT
// checked because empty password means "use agent / key auth".
func (c AppConfig) IsSSHConfigured() bool {
	return c.SSH.Host != "" && c.SSH.User != ""
}
