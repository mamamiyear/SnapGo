// Package config provides JSON-file backed persistence for AppConfig.
//
// Design rationale:
// - JSON keeps the file human-readable, which is useful while the MVP has
//   no settings UI for every option.
// - Storage location follows os.UserConfigDir() so each platform places it
//   in the canonical spot (macOS: ~/Library/Application Support/SnapGo).
// - File mode 0600 / dir mode 0700 mitigate accidental leakage of access
//   keys before we wire up the OS keychain (deferred to a later spec).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mmmy/snapgo/internal/domain"
)

// FileStore implements load/save of AppConfig on the local filesystem.
//
// All methods are safe for concurrent use.
type FileStore struct {
	mu   sync.RWMutex
	path string
}

// NewFileStore resolves the on-disk path and creates the parent directory
// with permissions 0700 if missing.
func NewFileStore() (*FileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config dir: %w", err)
	}
	appDir := filepath.Join(dir, "SnapGo")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return nil, fmt.Errorf("ensure config dir: %w", err)
	}
	return &FileStore{path: filepath.Join(appDir, "config.json")}, nil
}

// Path exposes the absolute config file path (mainly for diagnostics / UI).
func (s *FileStore) Path() string {
	return s.path
}

// Load reads the config file. If the file does not exist OR cannot be parsed,
// the default config is returned with a non-nil error so the caller can decide
// whether to surface the parse failure to the user.
func (s *FileStore) Load() (domain.AppConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := domain.DefaultAppConfig()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return domain.DefaultAppConfig(), fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save persists the supplied config atomically (write-temp + rename) so
// crashes mid-write cannot leave a half-written file.
func (s *FileStore) Save(cfg domain.AppConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp config: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}
	return nil
}
