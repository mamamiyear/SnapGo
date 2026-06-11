// uploader.go — small adapter that satisfies application.SSHUploader by
// dialing a one-shot connection per upload. This keeps the application
// layer free of any direct dependency on golang.org/x/crypto/ssh while
// still exposing a clean, unit-testable contract.
package ssh

import (
	"context"
	"os"
	"time"

	"github.com/mmmy/snapgo/internal/domain"
)

// Uploader is the production implementation of application.SSHUploader.
//
// We dial fresh per-upload because:
//   - Captures happen sparsely (a few per minute at peak), so the cost of
//     a TCP+SSH handshake is dwarfed by user think-time.
//   - Holding a long-lived connection would force us to add keepalives,
//     reconnection logic, and config-change invalidation — not worth it
//     for the current usage profile.
type Uploader struct {
	cfg domain.SSHConfig
}

// NewUploader returns a ready-to-use uploader. The configuration is
// captured by value so a simultaneous in-flight upload cannot be re-
// targeted by a settings save.
func NewUploader(cfg domain.SSHConfig) *Uploader {
	return &Uploader{cfg: cfg}
}

// Upload satisfies application.SSHUploader. We log entry/exit at INFO so
// the operator sees a single line per upload in the normal happy path,
// and ERROR with elapsed time when something fails.
func (u *Uploader) Upload(ctx context.Context, remoteRelPath string, data []byte, mode os.FileMode) error {
	start := time.Now()
	sshLog().Info("uploader.Upload start",
		"host", u.cfg.Host, "user", u.cfg.User, "port", u.cfg.Port,
		"remote_path", remoteRelPath, "size", len(data))

	client, err := Dial(ctx, u.cfg)
	if err != nil {
		sshLog().Error("uploader.Upload dial failed",
			"host", u.cfg.Host, "elapsed", time.Since(start), "err", err)
		return err
	}
	defer client.Close()
	if err := client.CopyFile(ctx, remoteRelPath, data, mode); err != nil {
		sshLog().Error("uploader.Upload copy failed",
			"remote_path", remoteRelPath, "elapsed", time.Since(start), "err", err)
		return err
	}
	sshLog().Info("uploader.Upload done",
		"remote_path", remoteRelPath, "size", len(data),
		"total_elapsed", time.Since(start))
	return nil
}
