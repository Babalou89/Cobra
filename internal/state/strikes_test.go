package state

import (
	"path/filepath"
	"testing"
)

func TestStrikeOnIdenticalFailures(t *testing.T) {
	s := &State{path: filepath.Join(t.TempDir(), "state.json")}
	failures := []string{"DOD: README missing", "quality: hold probe.py:2"}

	if struck := s.RecordRun(failures); struck {
		t.Fatalf("first failing run must not strike (baseline fingerprint)")
	}
	if struck := s.RecordRun(failures); !struck {
		t.Fatalf("second identical failing run must strike (no progress)")
	}
	if s.Strikes != 1 {
		t.Fatalf("strikes = %d, want 1", s.Strikes)
	}
}

func TestNoStrikeOnReducedFailures(t *testing.T) {
	s := &State{path: filepath.Join(t.TempDir(), "state.json")}
	s.RecordRun([]string{"a", "b", "c"})
	s.RecordRun([]string{"a", "b", "c"}) // strike 1
	if struck := s.RecordRun([]string{"a"}); struck {
		t.Fatalf("reduced failure set is progress — must not strike")
	}
	if s.Strikes != 0 {
		t.Fatalf("progress must reset the strike counter, got %d", s.Strikes)
	}
}

func TestStrikeLockAfterThreeNoProgressRuns(t *testing.T) {
	s := &State{path: filepath.Join(t.TempDir(), "state.json")}
	failures := []string{"same failure forever"}
	s.RecordRun(failures) // baseline
	for i := 0; i < MaxStrikes; i++ {
		s.RecordRun(failures)
	}
	if !s.Locked {
		t.Fatalf("three consecutive no-progress runs must lock the cage")
	}
}

func TestStrikeResetOnCleanRun(t *testing.T) {
	s := &State{path: filepath.Join(t.TempDir(), "state.json")}
	s.RecordRun([]string{"x"})
	s.RecordRun([]string{"x"})
	s.RecordRun(nil)
	if s.Strikes != 0 || s.LastFingerprint != "" {
		t.Fatalf("clean run must reset strikes and fingerprint")
	}
}

func TestStrikeFingerprintOrderIndependent(t *testing.T) {
	if Fingerprint([]string{"a", "b"}) != Fingerprint([]string{"b", "a"}) {
		t.Fatalf("fingerprint must ignore failure order")
	}
	if Fingerprint([]string{"a"}) == Fingerprint([]string{"b"}) {
		t.Fatalf("different failures must fingerprint differently")
	}
}

func TestStrikeStatePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.RecordRun([]string{"x"})
	s.RecordRun([]string{"x"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Strikes != 1 || loaded.LastFingerprint == "" {
		t.Fatalf("persisted state mismatch: %+v", loaded)
	}
}
