package worker

import (
	"strings"
	"testing"
	"time"

	"cobra/internal/ctxdiet"
)

func TestSlidingWindowCatchesAlternatingReads(t *testing.T) {
	replies := []string{
		`{"tool":"file_read","args":{"path":"dedup.py"}}`,
		`{"tool":"file_read","args":{"path":"test_dedup.py"}}`,
		`{"tool":"file_read","args":{"path":"dedup.py"}}`,
		`{"tool":"file_read","args":{"path":"test_dedup.py"}}`,
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{
		Name: "file_read", Usage: `{"path":"..."}`, Desc: "read file",
		Fn: func(args map[string]any) ToolResult {
			return ToolResult{OK: true, Output: "file contents here"}
		},
	})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 50, Ralph: true,
	}
	err := a.Run("build a dedup tool", "", "")
	if err == nil {
		t.Fatal("expected loop abort, got nil")
	}
	if !strings.Contains(err.Error(), "file_read") {
		t.Fatalf("expected file_read loop error, got: %v", err)
	}
	t.Logf("correctly aborted: %v", err)
}

func TestSlidingWindowAllowsMixedActions(t *testing.T) {
	replies := []string{
		`{"tool":"file_read","args":{"path":"a.py"}}`,
		`{"tool":"file_write","args":{"path":"a.py","content":"x"}}`,
		`{"tool":"file_read","args":{"path":"b.py"}}`,
		`{"tool":"file_write","args":{"path":"b.py","content":"y"}}`,
		`{"tool":"file_read","args":{"path":"c.py"}}`,
		`{"tool":"file_write","args":{"path":"c.py","content":"z"}}`,
		`{"done":true,"summary":"done"}`,
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_read", Usage: "{}", Desc: "read",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "ok"} }})
	r.Register(&Tool{Name: "file_write", Usage: "{}", Desc: "write",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "ok"} }})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 50, Ralph: true,
	}
	if err := a.Run("task", "", ""); err != nil {
		t.Fatalf("mixed actions should not trip detector: %v", err)
	}
}

func TestFileEditBlockedInRalphMode(t *testing.T) {
	replies := []string{
		`{"tool":"file_edit","args":{"path":"a.py","old":"x","new":"y"}}`,
		`{"tool":"file_write","args":{"path":"a.py","content":"fixed"}}`,
		`{"done":true,"summary":"done"}`,
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_write", Usage: "{}", Desc: "write",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "wrote"} }})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 50, Ralph: true,
	}
	err := a.Run("task", "", "")
	if err != nil {
		t.Fatalf("file_edit block + recovery should succeed: %v", err)
	}
	if fb.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", fb.calls)
	}
}

func TestFileEditAllowedInConversationalMode(t *testing.T) {
	replies := []string{
		`{"tool":"file_edit","args":{"path":"a.py","old":"x","new":"y"}}`,
		`{"done":true,"summary":"done"}`,
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_edit", Usage: "{}", Desc: "edit",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "edited"} }})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 50, Ralph: false,
	}
	if err := a.Run("task", "", ""); err != nil {
		t.Fatalf("file_edit should work in conversational mode: %v", err)
	}
}

func TestExactQwenFailurePattern(t *testing.T) {
	replies := make([]string, 50)
	for i := range replies {
		if i%2 == 0 {
			replies[i] = `{"tool":"file_read","args":{"path":"dedup.py"}}`
		} else {
			replies[i] = `{"tool":"file_read","args":{"path":"test_dedup.py"}}`
		}
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_read", Usage: "{}", Desc: "read",
		Fn: func(args map[string]any) ToolResult {
			return ToolResult{OK: true, Output: "#!/usr/bin/env python3\n"}
		}})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 50, Ralph: true,
	}
	err := a.Run("build a file deduplication CLI tool", "", "")
	if err == nil {
		t.Fatal("expected abort on Qwen failure pattern")
	}
	if fb.calls > 10 {
		t.Fatalf("detector too slow: %d calls", fb.calls)
	}
	t.Logf("aborted after %d calls: %v", fb.calls, err)
}

func TestWriteThenDoneSucceeds(t *testing.T) {
	replies := []string{
		`{"tool":"file_write","args":{"path":"dedup.py","content":"#!/usr/bin/env python3\nprint('ok')"}}`,
		`{"tool":"file_write","args":{"path":"test_dedup.py","content":"def test_ok(): assert True"}}`,
		`{"done":true,"summary":"built dedup tool"}`,
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_write", Usage: "{}", Desc: "write",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "wrote"} }})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 50, Ralph: true,
	}
	if err := a.Run("build dedup tool", "", ""); err != nil {
		t.Fatalf("write-then-done should succeed: %v", err)
	}
	if fb.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", fb.calls)
	}
}

func TestDeadMansSwitchWithNewTrim(t *testing.T) {
	replies := []string{
		`{"tool":"file_write","args":{"path":"a.py","content":"x"}}`,
	}
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_write", Usage: "{}", Desc: "write",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "wrote"} }})
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 1000, AttemptTimeout: 1 * time.Nanosecond, Ralph: true,
	}
	err := a.Run("task", "", "")
	if err == nil || !strings.Contains(err.Error(), "dead-man") {
		t.Fatalf("want dead-man abort, got: %v", err)
	}
}

func TestRalphContextBoundedWithNewTrim(t *testing.T) {
	replies := make([]string, 31)
	for i := 0; i < 30; i++ {
		replies[i] = `{"tool":"file_write","args":{"path":"f` + strings.Repeat("x", i%5+1) + `.py","content":"content"}}`
	}
	replies[30] = `{"done":true,"summary":"ok"}`
	fb := &fakeBackend{replies: replies}
	r := stubRegistry()
	r.Register(&Tool{Name: "file_write", Usage: "{}", Desc: "write",
		Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true, Output: "wrote 100 bytes"} }})
	var sizes []int
	a := &Agent{
		Backend: fb, Tools: r,
		Budget: ctxdiet.Budget{MaxTokens: 20000, PromptCeiling: 4000},
		MaxTurns: 40, Ralph: true,
		Observe: func(kind string, fields map[string]any) {
			if kind == "reply" {
				sizes = append(sizes, fields["ctx_tokens"].(int))
			}
		},
	}
	if err := a.Run("task", "", ""); err != nil {
		t.Fatal(err)
	}
	if len(sizes) < 20 {
		t.Fatalf("expected many turns, got %d", len(sizes))
	}
	early := sizes[4]
	late := sizes[len(sizes)-2]
	if late > early*3 {
		t.Fatalf("context grew too much: turn5=%d turn%d=%d", early, len(sizes)-1, late)
	}
	t.Logf("context stable: turn5=%d turn%d=%d", early, len(sizes)-1, late)
}

func TestRalphRegistryToolCount(t *testing.T) {
	expectedTools := []string{"file_read", "file_write", "code_execute"}
	r := stubRegistry()
	for _, name := range expectedTools {
		r.Register(&Tool{Name: name, Usage: "{}", Desc: "test",
			Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true} }})
	}
	if len(expectedTools) != 3 {
		t.Fatalf("expected 3 ralph tools, got %d", len(expectedTools))
	}
	for _, blocked := range []string{"list_dir", "file_edit", "web_fetch", "web_search", "system_info", "shell", "skill_run", "note"} {
		if r.Has(blocked) {
			t.Fatalf("ralph registry should NOT have %s", blocked)
		}
	}
}

func TestInstructRegistryToolCount(t *testing.T) {
	expectedTools := []string{"file_write", "code_execute"}
	r := stubRegistry()
	for _, name := range expectedTools {
		r.Register(&Tool{Name: name, Usage: "{}", Desc: "test",
			Fn: func(args map[string]any) ToolResult { return ToolResult{OK: true} }})
	}
	if len(expectedTools) != 2 {
		t.Fatalf("expected 2 instruct tools, got %d", len(expectedTools))
	}
	for _, blocked := range []string{"file_read", "list_dir", "file_edit", "web_fetch", "web_search", "system_info", "shell", "skill_run", "note"} {
		if r.Has(blocked) {
			t.Fatalf("instruct registry should NOT have %s", blocked)
		}
	}
}
