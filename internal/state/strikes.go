// Package state holds the cage's durable run state: strikes, cooldown,
// audit trail, session lifecycle, and quality reports.
//
// Strikes fire on no-progress, not on every failure. Each verify run's
// failures are hashed into a fingerprint; an identical fingerprint to the
// previous run means the agent is spinning — that is a strike. Fewer or
// different failures mean progress — no strike, counter reset. Three
// consecutive no-progress runs lock the cage until a human resets it.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxStrikes is the lock threshold: three consecutive no-progress runs.
const MaxStrikes = 3

// State is persisted to .cage/state.json.
type State struct {
	Strikes         int       `json:"strikes"`
	LastFingerprint string    `json:"last_fingerprint"`
	Locked          bool      `json:"locked"`
	UpdatedAt       time.Time `json:"updated_at"`

	path string
}

// StatePath returns the state file location for a project dir.
func StatePath(dir string) string {
	return filepath.Join(dir, ".cage", "state.json")
}

// Load reads state from path; a missing file yields zero state.
func Load(path string) (*State, error) {
	s := &State{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		// Corrupt state never blocks the cage silently — start clean.
		return &State{path: path}, nil
	}
	s.path = path
	return s, nil
}

// Save writes state back to its file.
func (s *State) Save() error {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Fingerprint reduces a failure list to an order-independent hash so two
// runs with the same failures compare equal regardless of report order.
func Fingerprint(failures []string) string {
	sorted := make([]string, len(failures))
	copy(sorted, failures)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\x00")))
	return hex.EncodeToString(sum[:])
}

// RecordRun applies the no-progress strike logic to one verify run and
// returns true when a strike was recorded. A clean run resets everything.
func (s *State) RecordRun(failures []string) bool {
	if len(failures) == 0 {
		s.Strikes = 0
		s.LastFingerprint = ""
		return false
	}
	fp := Fingerprint(failures)
	if fp == s.LastFingerprint {
		// Identical failures as last run: no progress — strike.
		s.Strikes++
		if s.Strikes >= MaxStrikes {
			s.Locked = true
		}
		return true
	}
	// Different failure set: progress. Remember it, no strike.
	s.LastFingerprint = fp
	s.Strikes = 0
	return false
}

// Reset clears strikes and the lock. Human-only: wired to `cage strikes
// reset`, never called by the worker loop.
func (s *State) Reset() {
	s.Strikes = 0
	s.LastFingerprint = ""
	s.Locked = false
}
