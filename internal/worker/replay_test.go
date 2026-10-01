package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cobra/internal/backend/fake"
	"cobra/internal/ctxdiet"
	"cobra/internal/jail"
)

// replayOutcome is what one fixture replay through the real Agent produced.
type replayOutcome struct {
	Err            error
	Turns          int // model calls made
	ProtocolErrors int
	Root           string
}

// replayFixture drives the real Agent loop (ralph registry, real jail in a
// temp dir) with a fixture's model replies and a fake backend.
func replayFixture(t testing.TB, name string, maxTurns int) replayOutcome {
	t.Helper()
	root := t.TempDir()
	jl, err := jail.New(root)
	if err != nil {
		t.Fatal(err)
	}
	fb := fake.New(fixtureReplies(t, name)...)
	out := replayOutcome{Root: root}
	ag := &Agent{
		Backend:        fb,
		Tools:          RalphRegistry(jl),
		Budget:         ctxdiet.Budget{MaxTokens: 24000, PromptCeiling: 8000},
		MaxTurns:       maxTurns,
		AttemptTimeout: time.Minute,
		Ralph:          true,
		NotesPath:      filepath.Join(root, "NOTES.md"),
		Observe: func(kind string, f map[string]any) {
			if kind == "result" && f["tool"] == "protocol" {
				out.ProtocolErrors++
			}
		},
	}
	out.Err = ag.Run("task", "", "")
	out.Turns = fb.Calls()
	return out
}

// Current behavior: tool-name-keyed {"note": ...} decodes to the note tool,
// which the ralph registry lacks, so each turn is an "unknown tool" result
// (not a protocol error). The script then runs dry (the 4th model call).
func TestReplayParseToolnameCurrentBehavior(t *testing.T) {
	o := replayFixture(t, "fixture-parse-toolname.jsonl", 24)
	if o.Err == nil {
		t.Fatal("expected an attempt error")
	}
	if o.ProtocolErrors != 0 {
		t.Fatalf("protocol errors=%d, want 0 (decodes as unknown tool)", o.ProtocolErrors)
	}
	if o.Turns != 4 {
		t.Fatalf("turns=%d want 4", o.Turns)
	}
}

func TestReplayAllFixturesTerminate(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			o := replayFixture(t, name, 24)
			if o.Err == nil {
				t.Fatal("fixtures never signal done; expected an attempt error")
			}
			if o.Turns == 0 || o.Turns > 24 {
				t.Fatalf("turns=%d", o.Turns)
			}
			_ = os.Remove(filepath.Join(o.Root, "NOTES.md"))
		})
	}
}
