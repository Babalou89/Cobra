package state

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// TraceStep is one worker action, logged by the binary (not self-reported
// summaries — the tool name and outcome come from the dispatch itself).
// The trajectory file is the evolver's primary input.
type TraceStep struct {
	Time    time.Time `json:"time"`
	Task    string    `json:"task,omitempty"`
	Turn    int       `json:"turn"`
	Tool    string    `json:"tool"`
	OK      bool      `json:"ok"`
	Summary string    `json:"summary"`
}

// TracePath returns the trajectory log location for a project dir.
func TracePath(dir string) string {
	return filepath.Join(dir, ".cage", "trajectory.jsonl")
}

// AppendTrace appends one step to the trajectory log.
func AppendTrace(path string, step TraceStep) error {
	if step.Time.IsZero() {
		step.Time = time.Now()
	}
	line, err := json.Marshal(step)
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

// ReadTraceTail returns the last n steps from the trajectory log.
func ReadTraceTail(path string, n int) ([]TraceStep, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var all []TraceStep
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var s TraceStep
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			all = append(all, s)
		}
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
