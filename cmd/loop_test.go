package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cobra/internal/backend/fake"
	"cobra/internal/cage"
	"cobra/internal/config"
	"cobra/internal/ctxdiet"
	"cobra/internal/jail"
	"cobra/internal/state"
	"cobra/internal/worker"
)

const testDOD = `task: "dod-task: create hello.txt"
name: "hello"
criteria:
  - name: "hello exists"
    verify:
      - type: exists
        file: "hello.txt"
      - type: grep
        file: "hello.txt"
        contains: "hello"
`

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// newProject builds a committed git project with brain.md, VERSION and a DOD.
func newProject(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, ".ai"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".ai", "brain.md"), []byte(strings.Repeat("line\n", 12)), 0o644))
	must(os.WriteFile(filepath.Join(dir, ".ai", "VERSION"), []byte("0.1.0\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "dod.yaml"), []byte(testDOD), 0o644))
	must(os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".cage/\nNOTES.md\n"), 0o644))
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func act(tool string, args map[string]any) string {
	b, _ := json.Marshal(map[string]any{"tool": tool, "args": args})
	return string(b)
}

// newLoop wires the real Agent to a fake backend and returns loop params.
func newLoop(t *testing.T, dir string, fb *fake.Backend, mutate func(*config.Config)) loopParams {
	t.Helper()
	cfg := config.Defaults()
	cfg.CooldownSeconds = 0
	if mutate != nil {
		mutate(cfg)
	}
	jl, err := jail.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	dod, err := cage.LoadDOD(filepath.Join(dir, "dod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.Load(state.StatePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	ag := &worker.Agent{
		Backend:        fb,
		Tools:          worker.RalphRegistry(jl),
		Budget:         ctxdiet.Budget{MaxTokens: 24000, PromptCeiling: 8000},
		MaxTurns:       cfg.Worker.MaxTurns,
		AttemptTimeout: time.Minute,
		Ralph:          true,
		NotesPath:      filepath.Join(dir, "NOTES.md"),
		BrainPath:      filepath.Join(dir, ".ai", "brain.md"),
	}
	return loopParams{Dir: dir, DODPath: "dod.yaml", DOD: dod, Agent: ag, Task: "cli-task: make hello", Cfg: cfg, State: st, JailRoot: jl.Root}
}

func TestRunAttemptsPassCommits(t *testing.T) {
	dir := newProject(t)
	fb := fake.New(
		act("file_write", map[string]any{"path": "hello.txt", "content": "hello\n"}),
		act("file_write", map[string]any{"path": ".ai/VERSION", "content": "0.1.1\n"}),
		`{"done": true, "summary": "ok"}`,
	)
	p := newLoop(t, dir, fb, nil)
	if err := runAttempts(p); err != nil {
		t.Fatalf("runAttempts: %v", err)
	}
	log := gitIn(t, dir, "log", "--oneline")
	if !strings.Contains(log, "cage: cli-task") {
		t.Fatalf("no cage commit in log:\n%s", log)
	}
}
