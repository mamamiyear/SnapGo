// Package application contains use-case orchestrators that the presentation
// layer (Wails bindings, frontend) calls into.
//
// Design rationale:
// - Application code only depends on domain interfaces; concrete adapters
//   are injected from main.go. This keeps the layer easy to unit test and
//   future-proofs us against swapping providers.
package application

import (
	"context"
	"fmt"
	"image"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/mmmy/snapgo/internal/domain"
	"github.com/mmmy/snapgo/internal/infrastructure/clipboard"
	"github.com/mmmy/snapgo/internal/infrastructure/screencapture"
)

// Notifier sends user-facing UI notifications. We define it as an interface
// so the application package does not depend on the Wails runtime.
type Notifier interface {
	NotifySuccess(url string)
	NotifyFailure(reason string)
}

// CaptureAndUploadService wires capture → upload → clipboard → notify.
type CaptureAndUploadService struct {
	Capturer  screencapture.Capturer
	Provider  domain.OSSProvider
	Clipboard clipboard.Writer
	Notifier  Notifier
	// FallbackDir is the directory where PNGs are saved when upload fails.
	FallbackDir string
	// PathPrefix is prepended to every generated object key.
	PathPrefix string
}

// Execute runs the full pipeline for a single screenshot region.
//
// On any failure after capture, the PNG is written to FallbackDir and the
// local file path is copied to the clipboard so the user can still recover
// the image manually.
func (s *CaptureAndUploadService) Execute(ctx context.Context, rect image.Rectangle) (*domain.UploadResult, error) {
	if s.Provider == nil {
		s.Notifier.NotifyFailure("OSS not configured")
		return nil, fmt.Errorf("oss provider is nil")
	}

	pngBytes, err := s.Capturer.CaptureRegion(rect)
	if err != nil {
		s.Notifier.NotifyFailure("capture failed: " + err.Error())
		return nil, err
	}
	return s.uploadAndCopy(ctx, pngBytes)
}

// ExecuteWithBytes runs only the upload-clipboard-notify portion of the
// pipeline using PNG bytes that have already been produced by an upstream
// capture step (e.g. macOS's interactive picker). Splitting this out keeps
// the service free of any knowledge about how the pixels were obtained.
func (s *CaptureAndUploadService) ExecuteWithBytes(ctx context.Context, pngBytes []byte) error {
	if s.Provider == nil {
		s.Notifier.NotifyFailure("OSS not configured")
		return fmt.Errorf("oss provider is nil")
	}
	if len(pngBytes) == 0 {
		s.Notifier.NotifyFailure("empty screenshot")
		return fmt.Errorf("empty screenshot")
	}
	_, err := s.uploadAndCopy(ctx, pngBytes)
	return err
}

// uploadAndCopy is the shared tail of Execute / ExecuteWithBytes. Extracting
// this avoids duplicating the failure-handling and clipboard logic.
func (s *CaptureAndUploadService) uploadAndCopy(ctx context.Context, pngBytes []byte) (*domain.UploadResult, error) {
	key := buildObjectKey(s.PathPrefix)
	start := time.Now()
	url, uploadErr := s.Provider.Upload(ctx, key, pngBytes, "image/png")
	if uploadErr != nil {
		path := s.fallback(pngBytes)
		_ = s.Clipboard.WriteText(path)
		s.Notifier.NotifyFailure("upload failed: " + uploadErr.Error())
		return nil, uploadErr
	}

	if err := s.Clipboard.WriteText(url); err != nil {
		s.Notifier.NotifyFailure("clipboard write failed: " + err.Error())
		return nil, err
	}

	s.Notifier.NotifySuccess(url)

	return &domain.UploadResult{
		URL:      url,
		Key:      key,
		Provider: s.Provider.Name(),
		Elapsed:  time.Since(start),
	}, nil
}

// fallback persists the PNG locally and returns the absolute path.
// Errors are swallowed because the caller has already failed once and we
// must not mask the original failure with a secondary error.
func (s *CaptureAndUploadService) fallback(data []byte) string {
	if s.FallbackDir == "" {
		s.FallbackDir = filepath.Join(os.TempDir(), "SnapGo")
	}
	_ = os.MkdirAll(s.FallbackDir, 0o755)
	name := fmt.Sprintf("%s.png", time.Now().Format("20060102-150405"))
	path := filepath.Join(s.FallbackDir, name)
	_ = os.WriteFile(path, data, 0o644)
	return path
}

// buildObjectKey produces a date-grouped object key with a 6-char random
// suffix to avoid collisions when many screenshots happen in the same second.
//
// The template is intentionally hard-coded for the MVP; making it user-
// configurable is left to a follow-up spec.
func buildObjectKey(prefix string) string {
	now := time.Now()
	return fmt.Sprintf("%s%s/%s/%s-%s.png",
		prefix,
		now.Format("2006"),
		now.Format("01"),
		now.Format("20060102-150405"),
		randomSuffix(6),
	)
}

const suffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// randomSuffix produces a short non-cryptographic identifier sufficient for
// avoiding key collisions among consecutive captures.
func randomSuffix(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = suffixAlphabet[rand.Intn(len(suffixAlphabet))]
	}
	return string(b)
}
