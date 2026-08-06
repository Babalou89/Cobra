package cage

import (
	"strings"
	"testing"
)

// -- Directive Tests --

func TestDirectFirstTurn(t *testing.T) {
	dod := &DOD{
		Task: "build a file deduplication CLI tool",
		Criteria: []Criterion{
			{Name: "source files exist", Verify: []Check{{Type: "exists", File: "dedup.py"}}},
			{Name: "tests pass", Verify: []Check{{Type: "command", Run: "pytest"}}},
		},
	}
	directive := dod.Direct(1, "")

	if directive.Task != "build a file deduplication CLI tool" {
		t.Errorf("expected task from DOD, got: %q", directive.Task)
	}
	if directive.Turn != 1 {
		t.Errorf("expected turn 1, got: %d", directive.Turn)
	}
	if directive.FixTarget != "" {
		t.Errorf("expected empty fix target on first turn, got: %q", directive.FixTarget)
	}
}

func TestDirectSubsequentTurn(t *testing.T) {
	dod := &DOD{
		Task: "build a file deduplication CLI tool",
		Criteria: []Criterion{
			{Name: "source files exist", Verify: []Check{{Type: "exists", File: "dedup.py"}}},
			{Name: "tests pass", Verify: []Check{{Type: "command", Run: "pytest"}}},
		},
	}
	verifyReport := "VERDICT: FAIL -- 2 failure(s)\n  X source files exist: file dedup.py does not exist\n  X tests pass: command exited 1"
	directive := dod.Direct(2, verifyReport)

	if directive.FixTarget != verifyReport {
		t.Errorf("expected verify report as fix target, got: %q", directive.FixTarget)
	}
	if directive.Turn != 2 {
		t.Errorf("expected turn 2, got: %d", directive.Turn)
	}
	if directive.Task != "build a file deduplication CLI tool" {
		t.Errorf("expected task from DOD, got: %q", directive.Task)
	}
}

func TestDirectThirdTurn(t *testing.T) {
	dod := &DOD{
		Task: "build a port scanner",
		Criteria: []Criterion{
			{Name: "source exists", Verify: []Check{{Type: "exists", File: "scanner.py"}}},
		},
	}
	verifyReport := "VERDICT: FAIL -- 1 failure(s)\n  X source exists: scanner.py does not exist"
	directive := dod.Direct(3, verifyReport)

	if directive.Turn != 3 {
		t.Errorf("expected turn 3, got: %d", directive.Turn)
	}
	if directive.FixTarget == "" {
		t.Error("expected fix target on third turn")
	}
}

func TestDirectEmptyVerifyReport(t *testing.T) {
	dod := &DOD{
		Task: "write hello.py",
		Criteria: []Criterion{
			{Name: "file exists", Verify: []Check{{Type: "exists", File: "hello.py"}}},
		},
	}
	directive := dod.Direct(1, "")
	if directive.FixTarget != "" {
		t.Errorf("expected empty fix target, got: %q", directive.FixTarget)
	}
}

func TestDirectAlwaysIncludesTask(t *testing.T) {
	dod := &DOD{
		Task: "build a config parser",
		Criteria: []Criterion{
			{Name: "file exists", Verify: []Check{{Type: "exists", File: "parser.py"}}},
		},
	}
	for turn := 1; turn <= 5; turn++ {
		report := ""
		if turn > 1 {
			report = "VERDICT: FAIL -- 1 failure(s)"
		}
		directive := dod.Direct(turn, report)
		if directive.Task != "build a config parser" {
			t.Errorf("turn %d: expected task, got: %q", turn, directive.Task)
		}
	}
}

// -- Context Clamping Tests --

func TestClampContextTo200Lines(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.WriteString("line " + strings.Repeat("x", 50) + "\n")
	}
	bigContext := sb.String()

	clamped := ClampContext(bigContext, 200)
	lines := strings.Split(clamped, "\n")

	if len(lines) > 200 {
		t.Errorf("expected max 200 lines, got: %d", len(lines))
	}
}

func TestClampContextKeepsNewest(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.WriteString("line " + string(rune('A'+i%26)) + "\n")
	}
	bigContext := sb.String()

	clamped := ClampContext(bigContext, 50)
	if !strings.Contains(clamped, "line") {
		t.Error("clamped context should contain lines")
	}
}

func TestClampContextSmallInput(t *testing.T) {
	smallContext := "line 1\nline 2\nline 3"
	clamped := ClampContext(smallContext, 200)

	if clamped != smallContext {
		t.Error("small context should be unchanged")
	}
}

// -- Integration: Cage Directs Ralph --

func TestCageDirectsRalphIntegration(t *testing.T) {
	dod := &DOD{
		Task: "write hello.py",
		Criteria: []Criterion{
			{Name: "file exists", Verify: []Check{
				{Type: "exists", File: "hello.py"},
			}},
		},
	}

	// Turn 1: cage gives ralph the task
	directive := dod.Direct(1, "")
	if directive.Task != "write hello.py" {
		t.Errorf("expected task, got: %q", directive.Task)
	}
	if directive.FixTarget != "" {
		t.Error("first turn should have no fix target")
	}

	// Turn 2: cage tells ralph what's broken
	verifyReport := "VERDICT: FAIL -- 1 failure(s)\n  X file exists: hello.py does not exist"
	directive = dod.Direct(2, verifyReport)
	if directive.FixTarget == "" {
		t.Error("expected fix target from verify report")
	}
	if !strings.Contains(directive.FixTarget, "hello.py does not exist") {
		t.Error("fix target should contain the failure")
	}

	// Turn 3: cage tells ralph to fix again
	verifyReport2 := "VERDICT: FAIL -- 1 failure(s)\n  X file exists: hello.py is empty"
	directive = dod.Direct(3, verifyReport2)
	if !strings.Contains(directive.FixTarget, "hello.py is empty") {
		t.Error("fix target should contain the new failure")
	}
}
