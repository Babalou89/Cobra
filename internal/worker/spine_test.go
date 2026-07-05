package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cobra/internal/backend"
	"cobra/internal/ctxdiet"
)

// fakeBackend replays canned responses.
type fakeBackend struct {
	replies []string
	calls   int
}

func (f *fakeBackend) Chat(msgs []backend.Msg, temp float64, maxTokens int) (string, error) {
	i := f.calls
	f.calls++
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return f.replies[i], nil
}
func (f *fakeBackend) Health() bool    { return true }
func (f *fakeBackend) Name() string    { return "fake" }
func (f *fakeBackend) MaxContext() int { return 32768 }

func stubRegistry() *Registry {
	r := NewRegistry()
	r.Register(&Tool{
		Name: "ping", Usage: "{}", Desc: "test tool",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "pong"} },
	})
	return r
}

func TestRepairJSONLiteralNewlines(t *testing.T) {
	raw := "{\"tool\":\"file_write\",\"args\":{\"path\":\"a.txt\",\"content\":\"line1\nline2\tend\"}}"
	act, err := parseAction(raw)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if act.Tool != "file_write" || !strings.Contains(act.Args["content"].(string), "line1\nline2") {
		t.Fatalf("repaired action wrong: %+v", act)
	}
}

func TestRepairJSONLeavesValidAlone(t *testing.T) {
	act, err := parseAction(`{"tool":"ping","args":{"x":"a\nb"}}`)
	if err != nil || act.Tool != "ping" {
		t.Fatalf("valid JSON mishandled: %v %+v", err, act)
	}
}

func TestLoopAbortsRepeatedProtocolErrors(t *testing.T) {
	fb := &fakeBackend{replies: []string{"I think I should look around first."}}
	a := &Agent{Backend: fb, Tools: stubRegistry(), Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000}, MaxTurns: 50}
	err := a.Run("task", "", "")
	if err == nil || !strings.Contains(err.Error(), "loop detected") {
		t.Fatalf("want loop abort, got: %v", err)
	}
	if fb.calls >= 10 {
		t.Fatalf("loop detector too slow: %d calls", fb.calls)
	}
}

func TestLoopAbortsRepeatedIdenticalAction(t *testing.T) {
	fb := &fakeBackend{replies: []string{`{"tool":"ping","args":{"x":1}}`}}
	a := &Agent{Backend: fb, Tools: stubRegistry(), Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000}, MaxTurns: 50}
	err := a.Run("task", "", "")
	if err == nil || !strings.Contains(err.Error(), "loop detected") {
		t.Fatalf("want loop abort, got: %v", err)
	}
}

func TestLoopAllowsVariedActions(t *testing.T) {
	fb := &fakeBackend{replies: []string{
		`{"tool":"ping","args":{"x":1}}`,
		`{"tool":"ping","args":{"x":2}}`,
		`{"tool":"ping","args":{"x":3}}`,
		`{"done":true,"summary":"finished"}`,
	}}
	a := &Agent{Backend: fb, Tools: stubRegistry(), Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000}, MaxTurns: 10}
	if err := a.Run("task", "", ""); err != nil {
		t.Fatalf("varied actions must not trip the detector: %v", err)
	}
}

func TestDeadMansSwitch(t *testing.T) {
	fb := &fakeBackend{replies: []string{
		`{"tool":"ping","args":{"x":1}}`,
		`{"tool":"ping","args":{"x":2}}`,
		`{"tool":"ping","args":{"x":3}}`,
		`{"tool":"ping","args":{"x":4}}`,
	}}
	a := &Agent{Backend: fb, Tools: stubRegistry(), Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 1000, AttemptTimeout: 1 * time.Nanosecond}
	err := a.Run("task", "", "")
	if err == nil || !strings.Contains(err.Error(), "dead-man") {
		t.Fatalf("want dead-man abort, got: %v", err)
	}
}

func TestRalphPromptCarriesNotesAndResets(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "NOTES.md")
	if err := os.WriteFile(notes, []byte("- [12:00] surveyed 10 dirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Agent{Ralph: true, NotesPath: notes, Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000}}
	user := a.buildUser("the task", "the dod", "the report")
	for _, want := range []string{"the task", "the dod", "the report", "surveyed 10 dirs", "durable memory"} {
		if !strings.Contains(user, want) {
			t.Errorf("ralph prompt missing %q", want)
		}
	}
	// Ralph trim: history never exceeds one exchange pair.
	steps := []ctxdiet.Step{{Role: "assistant", Content: "1"}, {Role: "tool", Content: "1"},
		{Role: "assistant", Content: "2"}, {Role: "tool", Content: "2"},
		{Role: "assistant", Content: "3"}, {Role: "tool", Content: "3"}}
	trimmed := a.trim(steps)
	if len(trimmed) != 2 || trimmed[1].Content != "3" {
		t.Fatalf("ralph trim wrong: %+v", trimmed)
	}
}

func TestRalphContextStaysConstant(t *testing.T) {
	// 30 turns of varied actions: the payload must not grow with turns.
	replies := make([]string, 0, 31)
	for i := 0; i < 30; i++ {
		replies = append(replies, `{"tool":"ping","args":{"x":`+strings.Repeat("9", i%5+1)+`}}`)
	}
	replies = append(replies, `{"done":true,"summary":"ok"}`)
	fb := &fakeBackend{replies: replies}

	var sizes []int
	a := &Agent{Backend: fb, Tools: stubRegistry(), Ralph: true,
		Budget:   ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 40,
		Observe: func(kind string, fields map[string]any) {
			if kind == "reply" {
				sizes = append(sizes, fields["ctx_tokens"].(int))
			}
		}}
	if err := a.Run("task", "dod", ""); err != nil {
		t.Fatal(err)
	}
	if len(sizes) < 20 {
		t.Fatalf("expected many turns, got %d", len(sizes))
	}
	first, last := sizes[2], sizes[len(sizes)-1]
	if last > first*2 {
		t.Fatalf("ralph context grew: turn3=%d final=%d", first, last)
	}
}
