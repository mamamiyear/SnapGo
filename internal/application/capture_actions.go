package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mmmy/snapgo/internal/infrastructure/clipboard"
)

// CaptureActionsService handles non-upload actions on already-produced PNG
// screenshots. Keeping these actions here makes save/copy available across
// overlay implementations without binding them to Wails or a specific OS.
type CaptureActionsService struct {
	Clipboard clipboard.Writer
	Notifier  Notifier
}

func (s *CaptureActionsService) CopyImage(_ context.Context, pngBytes []byte) error {
	if len(pngBytes) == 0 {
		err := fmt.Errorf("empty screenshot")
		s.notifyFailure(err.Error())
		return err
	}
	if s.Clipboard == nil {
		err := fmt.Errorf("clipboard is not configured")
		s.notifyFailure(err.Error())
		return err
	}
	if err := s.Clipboard.WriteImage(pngBytes); err != nil {
		s.notifyFailure("clipboard write failed: " + err.Error())
		return err
	}
	if s.Notifier != nil {
		s.Notifier.NotifySuccess("image copied to clipboard")
	}
	return nil
}

func (s *CaptureActionsService) SaveImage(_ context.Context, pngBytes []byte, dir string) (string, error) {
	if len(pngBytes) == 0 {
		err := fmt.Errorf("empty screenshot")
		s.notifyFailure(err.Error())
		return "", err
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		err := fmt.Errorf("save directory is empty")
		s.notifyFailure(err.Error())
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.notifyFailure("create save directory failed: " + err.Error())
		return "", err
	}
	path := uniquePNGPath(dir, time.Now())
	if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
		s.notifyFailure("save failed: " + err.Error())
		return "", err
	}
	if s.Notifier != nil {
		s.Notifier.NotifySuccess(path)
	}
	return path, nil
}

func (s *CaptureActionsService) notifyFailure(reason string) {
	if s.Notifier != nil {
		s.Notifier.NotifyFailure(reason)
	}
}

func uniquePNGPath(dir string, now time.Time) string {
	base := now.Format("20060102-150405")
	path := filepath.Join(dir, base+".png")
	for i := 1; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%02d.png", base, i))
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
