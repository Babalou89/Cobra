package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Session is the lifecycle marker for one work session.
type Session struct {
	Task      string    `json:"task,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// SessionPath returns the session file location for a project dir.
func SessionPath(dir string) string {
	return filepath.Join(dir, ".cage", "session.json")
}

// StartSession writes the session marker. An already-open session errors.
func StartSession(path, task string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("a session is already open (%s) — run `cage session end` first", path)
	}
	s := Session{Task: task, StartedAt: time.Now()}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// EndSession removes the session marker.
func EndSession(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
