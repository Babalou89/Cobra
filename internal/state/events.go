package state

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Event is one live-feed entry for the cage watch dashboard: model
// replies, dispatched actions, measured results, verify verdicts, and
// run-level status changes. Everything here is binary-observed.
type Event struct {
	Time      time.Time `json:"time"`
	Attempt   int       `json:"attempt,omitempty"`
	Turn      int       `json:"turn,omitempty"`
	Kind      string    `json:"kind"` // reply | action | result | verify | status
	Tool      string    `json:"tool,omitempty"`
	OK        *bool     `json:"ok,omitempty"`
	Text      string    `json:"text,omitempty"`
	CtxTokens int       `json:"ctx_tokens,omitempty"`
	CtxBudget int       `json:"ctx_budget,omitempty"`
}

// EventsPath returns the live-feed log location for a project dir.
func EventsPath(dir string) string {
	return filepath.Join(dir, ".cage", "events.jsonl")
}

// AppendEvent appends one event to the live feed.
func AppendEvent(path string, e Event) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	line, err := json.Marshal(e)
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

// ReadEventsTail returns the last n events.
func ReadEventsTail(path string, n int) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var all []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			all = append(all, e)
		}
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
