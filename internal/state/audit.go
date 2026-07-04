package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// AuditPath returns the append-only audit log location for a project dir.
func AuditPath(dir string) string {
	return filepath.Join(dir, ".cage", "audit.jsonl")
}

// Audit appends one event to the audit log. The log is append-only JSONL;
// nothing in the binary ever rewrites or truncates it.
func Audit(path, event string, fields map[string]any) error {
	entry := map[string]any{
		"time":  time.Now().Format(time.RFC3339),
		"event": event,
	}
	for k, v := range fields {
		entry[k] = v
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
