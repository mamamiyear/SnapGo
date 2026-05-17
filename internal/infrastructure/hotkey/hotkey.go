// Package hotkey wraps golang.design/x/hotkey to provide a process-wide
// global shortcut. The MVP only supports a single hard-coded combination
// (Cmd+Shift+A on macOS) but the abstraction below allows the binding to
// be replaced at runtime once we add per-platform parsing of user input.
package hotkey

import (
	"fmt"
	"strings"
	"sync"

	hk "golang.design/x/hotkey"
)

// Manager owns the lifecycle of one global hotkey.
type Manager struct {
	mu      sync.Mutex
	current *hk.Hotkey
	stop    chan struct{}
}

// NewManager returns a fresh manager with no hotkey registered yet.
func NewManager() *Manager { return &Manager{} }

// Register parses the spec (e.g. "cmd+shift+a") and binds the callback.
// If a previous hotkey is registered, it is unregistered first so callers
// can swap shortcuts at runtime when the user changes the setting.
func (m *Manager) Register(spec string, onTrigger func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil {
		_ = m.current.Unregister()
		close(m.stop)
		m.current = nil
		m.stop = nil
	}

	mods, key, err := parseSpec(spec)
	if err != nil {
		return err
	}

	hot := hk.New(mods, key)
	if err := hot.Register(); err != nil {
		return fmt.Errorf("register hotkey %q: %w", spec, err)
	}

	stop := make(chan struct{})
	m.current = hot
	m.stop = stop

	// Listen on a dedicated goroutine. We never run the callback inline
	// because the keydown channel is unbuffered.
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-hot.Keydown():
				if onTrigger != nil {
					go onTrigger()
				}
			}
		}
	}()
	return nil
}

// Unregister releases the active hotkey, if any.
func (m *Manager) Unregister() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		_ = m.current.Unregister()
		close(m.stop)
		m.current = nil
		m.stop = nil
	}
}

// parseSpec parses a "+"-separated hotkey description into (modifiers, key).
//
// Supported tokens (case-insensitive): cmd, command, ctrl, control,
// option, alt, shift, plus a single letter A–Z or digit 0–9.
//
// We keep the parser intentionally tiny — full key mapping is out of scope
// for the MVP.
func parseSpec(spec string) ([]hk.Modifier, hk.Key, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), "+")
	if len(parts) == 0 {
		return nil, 0, fmt.Errorf("empty hotkey")
	}
	var mods []hk.Modifier
	var key hk.Key
	hasKey := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "cmd", "command", "meta", "super":
			mods = append(mods, modCmd())
		case "ctrl", "control":
			mods = append(mods, modCtrl())
		case "option", "alt":
			mods = append(mods, modOption())
		case "shift":
			mods = append(mods, hk.ModShift)
		default:
			k, ok := lookupKey(p)
			if !ok {
				return nil, 0, fmt.Errorf("unsupported token: %q", p)
			}
			key = k
			hasKey = true
		}
	}
	if !hasKey {
		return nil, 0, fmt.Errorf("hotkey %q is missing a key", spec)
	}
	return mods, key, nil
}
