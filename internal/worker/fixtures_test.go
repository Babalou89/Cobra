package worker

// Fixture index. Every file under testdata/ was extracted verbatim from
// .cage/events.jsonl (the 2026-08-06 dedup-tool.yaml run, 7 attempts; the
// "line" fields are 1-based line numbers in that file). Each record is one
// model reply plus the outcome the cage logged for that turn. The events log
// stores replies clamped to 400 chars, so long file_write replies in the
// fixtures are truncated exactly as logged.
//
//	fixture-parse-toolname.jsonl  lines 53-60    attempt 1: {"note": ...} replies, 3x protocol error -> loop abort
//	fixture-loop-read.jsonl       lines 285-359  attempt 5: 24x alternating file_read test_dedup.py / dedup.py
//	fixture-loop-version.jsonl    lines 135-209  attempt 3: .ai/VERSION edit/write/note/read ping-pong
//	fixture-loop-edit.jsonl       lines 210-284  attempt 4: file_edit "import os"/"import pytest" removal ping-pong
//	fixture-dedup.jsonl           lines 62-134   attempt 2: tool-name-keyed {"file_write": {...}} then 21 file_write rewrites
//
// Outcomes logged for all dedup attempts: "worker used all 24 turns without
// signalling done" (except attempt 1: loop detected after 3 protocol errors).

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fixtureRecord is one line of a fixture file. Status/verify lines carry
// Kind+Text; reply lines carry Reply+Outcome.
type fixtureRecord struct {
	Line    int    `json:"line"`
	Attempt int    `json:"attempt"`
	Turn    int    `json:"turn"`
	Reply   string `json:"reply"`
	Outcome *struct {
		Line int    `json:"line"`
		Tool string `json:"tool"`
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	} `json:"outcome"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

var fixtureNames = []string{
	"fixture-parse-toolname.jsonl",
	"fixture-loop-read.jsonl",
	"fixture-loop-version.jsonl",
	"fixture-loop-edit.jsonl",
	"fixture-dedup.jsonl",
}

func loadFixture(t testing.TB, name string) []fixtureRecord {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	var out []fixtureRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r fixtureRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("%s: bad line: %v", name, err)
		}
		out = append(out, r)
	}
	return out
}

// fixtureReplies returns the model replies of a fixture in order.
func fixtureReplies(t testing.TB, name string) []string {
	var out []string
	for _, r := range loadFixture(t, name) {
		if r.Reply != "" {
			out = append(out, r.Reply)
		}
	}
	return out
}

func TestFixturesLoad(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			recs := loadFixture(t, name)
			if len(recs) == 0 {
				t.Fatal("empty fixture")
			}
			replies := fixtureReplies(t, name)
			if len(replies) == 0 {
				t.Fatal("fixture has no model replies")
			}
			for _, r := range recs {
				if r.Reply != "" && r.Outcome == nil {
					t.Fatalf("line %d: reply without logged outcome", r.Line)
				}
			}
		})
	}
}
