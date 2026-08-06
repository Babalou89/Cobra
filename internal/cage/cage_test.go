package cage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── DOD Loading Tests ────────────────────────────────────────────────────

func TestLoadDODValid(t *testing.T) {
	dir := t.TempDir()
	dodPath := filepath.Join(dir, "test.yaml")
	content := `task: "test task"
name: "test"
description: "a test DOD"
criteria:
  - name: "file exists"
    verify:
      - type: exists
        file: "somefile.txt"
`
	os.WriteFile(dodPath, []byte(content), 0o644)
	dod, err := LoadDOD(dodPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dod.Task != "test task" {
		t.Errorf("expected task 'test task', got %q", dod.Task)
	}
	if len(dod.Criteria) != 1 {
		t.Fatalf("expected 1 criterion, got %d", len(dod.Criteria))
	}
	if dod.Criteria[0].Name != "file exists" {
		t.Errorf("expected criterion name 'file exists', got %q", dod.Criteria[0].Name)
	}
}

func TestLoadDODZeroCriteria(t *testing.T) {
	dir := t.TempDir()
	dodPath := filepath.Join(dir, "empty.yaml")
	content := `task: "empty"
name: "empty"
description: "no criteria"
criteria: []
`
	os.WriteFile(dodPath, []byte(content), 0o644)
	_, err := LoadDOD(dodPath)
	if err == nil {
		t.Fatal("expected error for zero criteria")
	}
	if !strings.Contains(err.Error(), "zero criteria") {
		t.Errorf("expected 'zero criteria' in error, got: %v", err)
	}
}

func TestLoadDODNoVerifyChecks(t *testing.T) {
	dir := t.TempDir()
	dodPath := filepath.Join(dir, "no-verify.yaml")
	content := `task: "bad"
name: "bad"
description: "criterion with no verify"
criteria:
  - name: "empty criterion"
    verify: []
`
	os.WriteFile(dodPath, []byte(content), 0o644)
	_, err := LoadDOD(dodPath)
	if err == nil {
		t.Fatal("expected error for empty verify")
	}
}

func TestLoadDODNonexistent(t *testing.T) {
	_, err := LoadDOD("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadDODInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	dodPath := filepath.Join(dir, "bad.yaml")
	os.WriteFile(dodPath, []byte("not: [valid: yaml"), 0o644)
	_, err := LoadDOD(dodPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestCriteriaNames(t *testing.T) {
	dod := &DOD{
		Criteria: []Criterion{
			{Name: "alpha"},
			{Name: "beta"},
		},
	}
	names := dod.CriteriaNames()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("unexpected names: %v", names)
	}
}

// ── DOD Evaluation Tests ─────────────────────────────────────────────────

func TestEvaluateExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "present.txt"), []byte("hello"), 0o644)

	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "file present",
				Verify: []Check{
					{Type: "exists", File: "present.txt"},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestEvaluateExistsMissing(t *testing.T) {
	dir := t.TempDir()
	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "file missing",
				Verify: []Check{
					{Type: "exists", File: "nope.txt"},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
	if !strings.Contains(failures[0], "does not exist") {
		t.Errorf("unexpected failure: %s", failures[0])
	}
}

func TestEvaluateCommand(t *testing.T) {
	dir := t.TempDir()
	zero := 0
	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "command passes",
				Verify: []Check{
					{Type: "command", Run: "true", ExpectExit: &zero},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestEvaluateCommandFails(t *testing.T) {
	dir := t.TempDir()
	zero := 0
	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "command fails",
				Verify: []Check{
					{Type: "command", Run: "false", ExpectExit: &zero},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
}

func TestEvaluateGrepContains(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.py"), []byte("import os\nprint(os.getcwd())\n"), 0o644)

	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "contains import",
				Verify: []Check{
					{Type: "grep", File: "code.py", Contains: "import os"},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestEvaluateGrepMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.py"), []byte("print('hello')\n"), 0o644)

	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "contains import",
				Verify: []Check{
					{Type: "grep", File: "code.py", Contains: "import os"},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
}

func TestEvaluateGrepRegex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "has func main",
				Verify: []Check{
					{Type: "grep", File: "code.go", Regex: `func main\(`},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestEvaluateLines(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Title\n\nSome content\nMore\n"), 0o644)

	minLines := 3
	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "has enough lines",
				Verify: []Check{
					{Type: "lines", File: "doc.md", Min: minLines},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestEvaluateUnknownCheckType(t *testing.T) {
	dir := t.TempDir()
	dod := &DOD{
		Criteria: []Criterion{
			{
				Name: "bad check",
				Verify: []Check{
					{Type: "nonexistent"},
				},
			},
		},
	}
	failures := dod.Evaluate(dir)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure for unknown type, got %d", len(failures))
	}
}

// ── Scrape Tests ─────────────────────────────────────────────────────────

func TestScrapeGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	content := "package main\n\nimport \"fmt\"\n\n// A greeting function\nfunc greet() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	if !f.Exists {
		t.Fatal("expected file to exist")
	}
	if f.Lang != "go" {
		t.Errorf("expected lang 'go', got %q", f.Lang)
	}
	if !f.SyntaxOK {
		t.Error("expected syntax OK")
	}
	if f.Functions != 1 {
		t.Errorf("expected 1 function, got %d", f.Functions)
	}
	if f.CommentLines != 1 {
		t.Errorf("expected 1 comment line, got %d", f.CommentLines)
	}
}

func TestScrapePythonFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.py")
	content := "import os\n\ndef hello():\n    print('hi')\n\nclass Foo:\n    pass\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	if f.Lang != "python" {
		t.Errorf("expected lang 'python', got %q", f.Lang)
	}
	if f.Functions != 1 {
		t.Errorf("expected 1 function, got %d", f.Functions)
	}
	if f.Classes != 1 {
		t.Errorf("expected 1 class, got %d", f.Classes)
	}
	if f.EmptyBodies != 1 {
		t.Errorf("expected 1 empty body (pass), got %d", f.EmptyBodies)
	}
	if len(f.UnusedImports) != 1 {
		t.Errorf("expected 1 unused import (os), got %d: %v", len(f.UnusedImports), f.UnusedImports)
	}
}

func TestScrapeNonexistent(t *testing.T) {
	f := Scrape("/nonexistent/file.go")
	if f.Exists {
		t.Error("expected Exists=false for missing file")
	}
}

func TestScrapeTodoMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wip.go")
	content := "package main\n// TODO: fix this\nfunc main() {}\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	if f.TodoCount != 1 {
		t.Errorf("expected 1 TODO, got %d", f.TodoCount)
	}
}

func TestScrapeSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "leaky.py")
	content := "api_key = \"AKIA1234567890ABCDEF\"\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	if f.SecretHits != 1 {
		t.Errorf("expected 1 secret hit, got %d", f.SecretHits)
	}
}

// ── RunChecks Tests ──────────────────────────────────────────────────────

func TestRunChecksAllPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.go")
	content := "package main\n\nfunc main() {}\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	f.Path = "clean.go"
	findings := RunChecks(f, map[string]bool{})
	for _, fi := range findings {
		if !fi.Passed {
			t.Errorf("check %s failed: %s", fi.Code, fi.Detail)
		}
	}
}

func TestRunChecksStubDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stub.py")
	content := "def foo():\n    pass\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	f.Path = "stub.py"
	findings := RunChecks(f, map[string]bool{})
	for _, fi := range findings {
		if fi.Code == "stub" && fi.Passed {
			t.Error("expected stub check to fail on pass-only body")
		}
	}
}

func TestRunChecksDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.go")
	content := "package main\n// TODO: fix\nfunc main() {}\n"
	os.WriteFile(path, []byte(content), 0o644)

	f := Scrape(path)
	f.Path = "todo.go"
	// With hold disabled, should have no hold findings
	findings := RunChecks(f, map[string]bool{"hold": true})
	for _, fi := range findings {
		if fi.Code == "hold" {
			t.Error("hold check should be disabled")
		}
	}
}

// ── Helper Tests ─────────────────────────────────────────────────────────

func TestCountLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello\n", 1},
		{"hello\nworld", 2},
		{"hello\nworld\n", 2},
		{"a\nb\nc\n", 3},
	}
	for _, tt := range tests {
		got := countLines(tt.input)
		if got != tt.want {
			t.Errorf("countLines(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestJoin(t *testing.T) {
	// Relative path
	got := join("/project", "file.txt")
	if got != "/project/file.txt" {
		t.Errorf("expected /project/file.txt, got %s", got)
	}
	// Absolute path stays absolute
	got = join("/project", "/absolute/file.txt")
	if got != "/absolute/file.txt" {
		t.Errorf("expected /absolute/file.txt, got %s", got)
	}
}

func TestLanguageDetection(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"file.ts", "javascript"},
		{"data.json", "json"},
		{"config.yaml", "yaml"},
		{"run.sh", "bash"},
		{"README.md", "markdown"},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		got := langOf(tt.path)
		if got != tt.want {
			t.Errorf("langOf(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestCheckSyntaxGo(t *testing.T) {
	dir := t.TempDir()
	// Valid Go
	path := filepath.Join(dir, "good.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644)
	f := Scrape(path)
	if !f.SyntaxOK {
		t.Error("expected valid Go to pass syntax check")
	}

	// Invalid Go
	path2 := filepath.Join(dir, "bad.go")
	os.WriteFile(path2, []byte("package main\nfunc {\n"), 0o644)
	f2 := Scrape(path2)
	if f2.SyntaxOK {
		t.Error("expected invalid Go to fail syntax check")
	}
}

func TestCheckSyntaxJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.json")
	os.WriteFile(path, []byte(`{"key": "value"}`), 0o644)
	f := Scrape(path)
	if !f.SyntaxOK {
		t.Error("expected valid JSON to pass")
	}

	path2 := filepath.Join(dir, "bad.json")
	os.WriteFile(path2, []byte(`{"key": "value"`), 0o644)
	f2 := Scrape(path2)
	if f2.SyntaxOK {
		t.Error("expected invalid JSON to fail")
	}
}

// ── Integration: full DOD evaluation with Scrape + RunChecks ─────────────

func TestFullDODEvaluation(t *testing.T) {
	dir := t.TempDir()

	// Create a clean Python file
	pyFile := filepath.Join(dir, "clean.py")
	os.WriteFile(pyFile, []byte("import os\n\nos.getcwd()  # use os\n\ndef greet(name: str) -> str:\n    return f\"hello {name}\"\n"), 0o644)

	// Create a DOD that checks the file exists and compiles
	zero := 0
	dod := &DOD{
		Task: "test full evaluation",
		Criteria: []Criterion{
			{
				Name: "file exists",
				Verify: []Check{
					{Type: "exists", File: "clean.py"},
				},
			},
			{
				Name: "python compiles",
				Verify: []Check{
					{Type: "command", Run: "cd " + dir + " && python3 -m py_compile clean.py", ExpectExit: &zero},
				},
			},
			{
				Name: "has function",
				Verify: []Check{
					{Type: "grep", File: "clean.py", Contains: "def greet"},
				},
			},
		},
	}

	failures := dod.Evaluate(dir)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}

	// Also test quality officer on this file
	facts := Scrape(pyFile)
	if facts.Lang != "python" {
		t.Errorf("expected python, got %s", facts.Lang)
	}
	if facts.Functions != 1 {
		t.Errorf("expected 1 function, got %d", facts.Functions)
	}
	findings := RunChecks(facts, map[string]bool{})
	for _, f := range findings {
		if !f.Passed {
			t.Errorf("check %s failed on clean file: %s", f.Code, f.Detail)
		}
	}
}

// ── Pluggable Tests ──────────────────────────────────────────────────────

func TestRunPluggablePass(t *testing.T) {
	dir := t.TempDir()
	tools := []PlugTool{{Name: "echo", Run: "echo hello"}}
	failures := RunPluggable(dir, tools)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestRunPluggableFail(t *testing.T) {
	dir := t.TempDir()
	tools := []PlugTool{{Name: "fail", Run: "false"}}
	failures := RunPluggable(dir, tools)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if !strings.Contains(failures[0], "fail") {
		t.Errorf("expected 'fail' in failure message, got: %s", failures[0])
	}
}

func TestRunPluggableEmptyRun(t *testing.T) {
	dir := t.TempDir()
	tools := []PlugTool{{Name: "empty", Run: "  "}}
	failures := RunPluggable(dir, tools)
	if len(failures) != 0 {
		t.Errorf("empty run should be skipped, got: %v", failures)
	}
}
