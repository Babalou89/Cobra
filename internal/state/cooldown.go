package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CooldownPath returns the cooldown marker location for a project dir.
func CooldownPath(dir string) string {
	return filepath.Join(dir, ".cage", "cooldown")
}

// CheckCooldown errors when the last attempt was more recent than min.
// Zero or negative min disables the cooldown.
func CheckCooldown(path string, min time.Duration) error {
	if min <= 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // no marker yet — first attempt
	}
	last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return nil
	}
	elapsed := time.Since(time.Unix(last, 0))
	if elapsed < min {
		return fmt.Errorf("cooldown: %s remaining before next attempt", (min - elapsed).Round(time.Second))
	}
	return nil
}

// TouchCooldown records now as the last attempt time.
func TouchCooldown(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"), 0o644)
}
