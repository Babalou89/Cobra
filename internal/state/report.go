package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// QualityReport is one verify run's outcome, persisted for the mirror.
type QualityReport struct {
	Time     time.Time `json:"time"`
	Passed   bool      `json:"passed"`
	Failures []string  `json:"failures"`
}

// ReportPath returns the current report location for a project dir.
func ReportPath(dir string) string {
	return filepath.Join(dir, ".cage", "report.json")
}

// WriteReport persists the latest verify outcome.
func WriteReport(dir string, r QualityReport) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cage"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ReportPath(dir), data, 0o644)
}

// ArchiveReport moves the current report into .cage/reports/ with a
// timestamped name. Used at session end.
func ArchiveReport(dir string) error {
	src := ReportPath(dir)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	archive := filepath.Join(dir, ".cage", "reports")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(archive, fmt.Sprintf("report-%s.json", time.Now().Format("20060102-150405")))
	return os.Rename(src, dst)
}
