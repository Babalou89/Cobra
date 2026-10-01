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

func TestConfigAutoVersionBump(t *testing.T) {
	if !Defaults().AutoVersionBump {
		t.Fatal("auto_version_bump must default to true")
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".cage"), 0o755)
	_ = os.WriteFile(Path(dir), []byte("auto_version_bump: false\n"), 0o644)
	cfg, err := Load(dir)
	if err != nil || cfg.AutoVersionBump {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
	_ = os.WriteFile(Path(dir), []byte("dod: x.yaml\n"), 0o644)
	cfg, _ = Load(dir)
	if !cfg.AutoVersionBump {
		t.Fatal("omitted key must keep default true")
	}
}
