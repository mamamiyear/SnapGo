package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeClipboard struct {
	text  string
	image []byte
}

func (f *fakeClipboard) WriteText(s string) error {
	f.text = s
	return nil
}

func (f *fakeClipboard) WriteImage(pngBytes []byte) error {
	f.image = append([]byte(nil), pngBytes...)
	return nil
}

type fakeNotifier struct {
	success string
	failure string
}

func (f *fakeNotifier) NotifySuccess(value string)  { f.success = value }
func (f *fakeNotifier) NotifyFailure(reason string) { f.failure = reason }

func TestCaptureActionsCopyImageWritesPNGToClipboard(t *testing.T) {
	clip := &fakeClipboard{}
	notifier := &fakeNotifier{}
	svc := &CaptureActionsService{Clipboard: clip, Notifier: notifier}

	err := svc.CopyImage(context.Background(), []byte("png"))
	if err != nil {
		t.Fatalf("copy image: %v", err)
	}
	if string(clip.image) != "png" {
		t.Fatalf("expected image bytes copied, got %q", string(clip.image))
	}
	if notifier.success != "image copied to clipboard" {
		t.Fatalf("expected copy success notification, got %q", notifier.success)
	}
}

func TestCaptureActionsSaveImageWritesUniquePNG(t *testing.T) {
	dir := t.TempDir()
	svc := &CaptureActionsService{Notifier: &fakeNotifier{}}

	path, err := svc.SaveImage(context.Background(), []byte("png"), dir)
	if err != nil {
		t.Fatalf("save image: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("expected save in %q, got %q", dir, path)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "png" {
		t.Fatalf("expected saved bytes, data=%q err=%v", string(data), err)
	}

	next, err := svc.SaveImage(context.Background(), []byte("png2"), dir)
	if err != nil {
		t.Fatalf("save second image: %v", err)
	}
	if next == path {
		t.Fatalf("expected unique path for second save")
	}
}
