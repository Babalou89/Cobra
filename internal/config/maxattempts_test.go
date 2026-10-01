package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigMaxAttempts(t *testing.T) {
	if Defaults().MaxAttempts != 5 {
		t.Fatalf("default max_attempts = %d, want 5", Defaults().MaxAttempts)
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".cage"), 0o755)
	_ = os.WriteFile(Path(dir), []byte("max_attempts: 3\n"), 0o644)
	cfg, err := Load(dir)
	if err != nil || cfg.MaxAttempts != 3 {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
	_ = os.WriteFile(Path(dir), []byte("max_attempts: 0\n"), 0o644)
	cfg, err = Load(dir)
	if err != nil || cfg.MaxAttempts != 5 {
		t.Fatalf("zero must fall back to 5: %v %v", cfg, err)
	}
}
