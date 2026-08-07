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
	if !strings.Contains(directive.FixTarget, "DOD criteria:") {
		t.Errorf("first turn should have DOD criteria checklist, got: %q", directive.FixTarget)
	}
	if !strings.Contains(directive.FixTarget, "[?] source files exist") {
		t.Errorf("first turn should show unknown status for criteria, got: %q", directive.FixTarget)
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
	// Use the actual format from Evaluate() + Report()
	verifyReport := "VERDICT: FAIL — 2 failure(s)\n  ✗ DOD \"source files exist\": file dedup.py does not exist\n  ✗ DOD \"tests pass\": command \"pytest\" exited 1\n"
	directive := dod.Direct(2, verifyReport)

	if !strings.Contains(directive.FixTarget, verifyReport) {
		t.Errorf("fix target should contain verify report, got: %q", directive.FixTarget)
	}
	if directive.Turn != 2 {
		t.Errorf("expected turn 2, got: %d", directive.Turn)
	}
	if !strings.Contains(directive.FixTarget, "[FAIL] source files exist") {
		t.Errorf("fix target should show FAIL status, got: %q", directive.FixTarget)
	}
	if !strings.Contains(directive.FixTarget, "[FAIL] tests pass") {
		t.Errorf("fix target should show FAIL status for tests, got: %q", directive.FixTarget)
	}
}

func TestDirectThirdTurn(t *testing.T) {
	dod := &DOD{
		Task: "build a port scanner",
		Criteria: []Criterion{
			{Name: "source exists", Verify: []Check{{Type: "exists", File: "scanner.py"}}},
		},
	}
	verifyReport := "VERDICT: FAIL — 1 failure(s)\n  ✗ DOD \"source exists\": scanner.py does not exist\n"
	directive := dod.Direct(3, verifyReport)

	if directive.Turn != 3 {
		t.Errorf("expected turn 3, got: %d", directive.Turn)
	}
	if directive.FixTarget == "" {
		t.Error("expected fix target on third turn")
	}
	if !strings.Contains(directive.FixTarget, "DOD criteria:") {
		t.Error("fix target should include DOD criteria checklist")
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
	if !strings.Contains(directive.FixTarget, "DOD criteria:") {
		t.Errorf("should include DOD criteria even without verify report, got: %q", directive.FixTarget)
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
			report = "VERDICT: FAIL — 1 failure(s)\n  ✗ DOD \"file exists\": parser.py does not exist\n"
		}
		directive := dod.Direct(turn, report)
		if directive.Task != "build a config parser" {
			t.Errorf("turn %d: expected task, got: %q", turn, directive.Task)
		}
	}
}

// -- DOD Checklist Tests --

func TestDODChecklistShowsStatus(t *testing.T) {
	dod := &DOD{
		Task: "build a tool",
		Criteria: []Criterion{
			{Name: "files exist", Verify: []Check{{Type: "exists", File: "a.py"}}},
			{Name: "compiles", Verify: []Check{{Type: "command", Run: "py_compile a.py"}}},
		},
	}

	// No verify report — all statuses should be "?"
	dir := dod.Direct(1, "")
	if !strings.Contains(dir.FixTarget, "[?] files exist") {
		t.Errorf("expected unknown status, got: %q", dir.FixTarget)
	}
	if !strings.Contains(dir.FixTarget, "[?] compiles") {
		t.Errorf("expected unknown status, got: %q", dir.FixTarget)
	}

	// With failures — should show FAIL/PASS
	report := "VERDICT: FAIL — 1 failure(s)\n  ✗ DOD \"files exist\": file a.py does not exist\n"
	dir2 := dod.Direct(2, report)
	if !strings.Contains(dir2.FixTarget, "[FAIL] files exist") {
		t.Errorf("expected FAIL status, got: %q", dir2.FixTarget)
	}
	if !strings.Contains(dir2.FixTarget, "[PASS] compiles") {
		t.Errorf("expected PASS status, got: %q", dir2.FixTarget)
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

	// Turn 1: cage gives ralph the task + DOD criteria
	directive := dod.Direct(1, "")
	if directive.Task != "write hello.py" {
		t.Errorf("expected task, got: %q", directive.Task)
	}
	if !strings.Contains(directive.FixTarget, "DOD criteria:") {
		t.Error("first turn should have DOD criteria checklist")
	}

	// Turn 2: cage tells ralph what's broken
	verifyReport := "VERDICT: FAIL — 1 failure(s)\n  ✗ DOD \"file exists\": hello.py does not exist\n"
	directive = dod.Direct(2, verifyReport)
	if directive.FixTarget == "" {
		t.Error("expected fix target from verify report")
	}
	if !strings.Contains(directive.FixTarget, "hello.py does not exist") {
		t.Error("fix target should contain the failure")
	}
	if !strings.Contains(directive.FixTarget, "[FAIL] file exists") {
		t.Error("fix target should show FAIL status")
	}

	// Turn 3: cage tells ralph to fix again
	verifyReport2 := "VERDICT: FAIL — 1 failure(s)\n  ✗ DOD \"file exists\": hello.py is empty\n"
	directive = dod.Direct(3, verifyReport2)
	if !strings.Contains(directive.FixTarget, "hello.py is empty") {
		t.Error("fix target should contain the new failure")
	}
}
