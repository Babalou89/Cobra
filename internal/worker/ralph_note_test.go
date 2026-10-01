package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cobra/internal/jail"
)

func TestRalphPromptMatchesRegistry(t *testing.T) {
	jl, _ := jail.New(t.TempDir())
	r := RalphRegistry(jl)
	prompt := SystemPrompt(r.Describe(), true)
	for _, name := range r.names() {
		if !strings.Contains(prompt, "\n- "+name+" ") {
			t.Errorf("registry tool %q missing from prompt", name)
		}
	}
	if !r.Has("note") || !strings.Contains(prompt, "note tool") {
		t.Error("ralph registry must have note and prompt must mention it")
	}
	// A registry without note must not be told to use it.
	bare := NewRegistry()
	registerShellExecute(bare, jl)
	if p := SystemPrompt(bare.Describe(), true); strings.Contains(p, "note tool") {
		t.Error("prompt mentions note tool although it is not registered")
	}
	if strings.Contains(prompt, "definition of done") {
		t.Error("prompt refers to a definition of done that is not injected")
	}
}

func TestRalphNoteToolWritesOnlyNotes(t *testing.T) {
	root := t.TempDir()
	jl, _ := jail.New(root)
	r := RalphRegistry(jl)
	res := r.Dispatch("note", map[string]any{"text": "found it"})
	if !res.OK {
		t.Fatalf("note failed: %s", res.Output)
	}
	data, err := os.ReadFile(filepath.Join(root, "NOTES.md"))
	if err != nil || !strings.Contains(string(data), "found it") {
		t.Fatalf("NOTES.md = %q, %v", data, err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("note created extra files: %v", entries)
	}
}

func TestBareNoteReplyIsNotProtocolError(t *testing.T) {
	act, err := parseAction(`{"note": "already done"}`)
	if err != nil || act.Tool != "note" {
		t.Fatalf("act=%+v err=%v", act, err)
	}
	jl, _ := jail.New(t.TempDir())
	if res := RalphRegistry(jl).Dispatch(act.Tool, act.Args); !res.OK {
		t.Fatalf("note dispatch: %s", res.Output)
	}
}
