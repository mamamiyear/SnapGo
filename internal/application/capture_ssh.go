// capture_ssh.go — application-level service that uploads a screenshot to
// a remote host via SSH/SCP.
//
// Design rationale:
//   - Mirrors CaptureAndUploadService so the UI layer can call a single
//     well-defined entrypoint per destination kind.
//   - Depends on a small Uploader interface rather than the concrete SSH
//     adapter so the service is unit-testable without a real SSH server.
//   - The remote object key is generated through the same date-based
//     scheme used for S3 uploads (see buildObjectKey in capture_upload.go)
//     so users get a consistent layout regardless of destination.
package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/mmmy/snapgo/internal/domain"
	"github.com/mmmy/snapgo/internal/infrastructure/clipboard"
)

// sshSvcLog returns an application-layer SSH logger bound to the CURRENT
// default handler.
//
// 为什么是函数而非包级 slog.With 变量: 包级变量会在 main() 调用
// logging.Init() 之前被初始化, 捕获到 bootstrap handler, 之后 SetDefault
// 切换 handler 时会产生双前缀日志. 惰性读取 slog.Default() 可避免该问题.
func sshSvcLog() *slog.Logger { return slog.Default().With("component", "application.ssh") }

// SSHUploader abstracts the SSH/SCP adapter so this layer does not depend
// on golang.org/x/crypto/ssh directly. The infrastructure package wires a
// concrete implementation through a small adapter below.
type SSHUploader interface {
	// Upload writes `data` to remoteRelPath (relative to the user's $HOME)
	// with the given file mode and returns once the remote has acknowledged
	// the transfer.
	Upload(ctx context.Context, remoteRelPath string, data []byte, mode os.FileMode) error
}

// CaptureAndSSHService wires capture-bytes → SCP → clipboard → notify.
type CaptureAndSSHService struct {
	Uploader  SSHUploader
	Clipboard clipboard.Writer
	Notifier  Notifier

	// Cfg is the active SSH configuration; we keep a copy on the service
	// so PathPrefix and PublicURLBase are explicitly part of the contract
	// rather than reaching into a shared global.
	Cfg domain.SSHConfig
}

// ExecuteWithBytes uploads previously-captured PNG bytes via SCP and copies
// either a public URL (if configured) or the remote-relative path to the
// clipboard. Either is useful: a URL for sharing, a path so the user can
// inspect / scp it manually.
func (s *CaptureAndSSHService) ExecuteWithBytes(ctx context.Context, pngBytes []byte) error {
	if s.Uploader == nil {
		sshSvcLog().Warn("ExecuteWithBytes aborted: uploader nil")
		s.notifyFailure("SSH not configured")
		return fmt.Errorf("ssh uploader is nil")
	}
	if len(pngBytes) == 0 {
		sshSvcLog().Warn("ExecuteWithBytes aborted: empty screenshot")
		s.notifyFailure("empty screenshot")
		return fmt.Errorf("empty screenshot")
	}
	relPath := buildRemoteRelPath(s.Cfg.PathPrefix)
	sshSvcLog().Info("save-remote pipeline start",
		"host", s.Cfg.Host, "user", s.Cfg.User, "port", s.Cfg.Port,
		"size", len(pngBytes), "remote_path", relPath,
		"strict_host_key", s.Cfg.StrictHostKey)
	start := time.Now()
	if err := s.Uploader.Upload(ctx, relPath, pngBytes, 0o644); err != nil {
		sshSvcLog().Error("save-remote upload failed",
			"remote_path", relPath, "elapsed", time.Since(start), "err", err)
		s.notifyFailure("ssh upload failed: " + err.Error())
		return err
	}
	sshSvcLog().Info("save-remote upload ok",
		"remote_path", relPath, "elapsed", time.Since(start))

	clipText := remoteShareText(relPath)
	if s.Clipboard != nil {
		if err := s.Clipboard.WriteText(clipText); err != nil {
			sshSvcLog().Error("save-remote clipboard write failed", "err", err)
			s.notifyFailure("clipboard write failed: " + err.Error())
			return err
		}
		sshSvcLog().Debug("save-remote clipboard write ok", "text", clipText)
	}
	if s.Notifier != nil {
		s.Notifier.NotifySuccess(clipText)
	}
	sshSvcLog().Info("save-remote pipeline done",
		"remote_path", relPath, "total_elapsed", time.Since(start))
	return nil
}

// remoteShareText returns the clipboard string for an SSH upload.
//
// 设计理由: 用户反馈只需要远端机器上的路径 (方便登录后直接定位文件),
// 不需要 user@host 前缀, 也不再支持 Public URL Base. 这里统一返回
// "~/<relPath>", 即相对远端 $HOME 的可读路径.
func remoteShareText(relPath string) string {
	return "~/" + relPath
}

// buildRemoteRelPath produces the remote-relative target path with the
// same date-grouped layout used by the S3 pipeline.
func buildRemoteRelPath(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimPrefix(prefix, "~")
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	now := time.Now()
	return fmt.Sprintf("%s%s/%s/%s-%s.png",
		prefix,
		now.Format("2006"),
		now.Format("01"),
		now.Format("20060102-150405"),
		randomSuffix(6),
	)
}

func (s *CaptureAndSSHService) notifyFailure(reason string) {
	if s.Notifier != nil {
		s.Notifier.NotifyFailure(reason)
	}
}
