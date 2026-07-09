package critic

import (
	"strings"
	"testing"
)

func TestParseErrors(t *testing.T) {
	fixture := `main.py:10:5: error: Incompatible types in assignment (expression has type "str", variable has type "int")  [assignment]
main.py:22:1: error: Missing return statement  [return]
utils.py:3:12: error: Name "foo" is not defined  [name-defined]
Success: no issues found in 3 source files
`
	res := ParseErrors(fixture)
	if len(res.Errors) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(res.Errors))
	}
	if res.Errors[0].File != "main.py" || res.Errors[0].Line != 10 {
		t.Errorf("expected main.py:10, got %s:%d", res.Errors[0].File, res.Errors[0].Line)
	}
	if !strings.Contains(res.Errors[0].Message, "Incompatible types") {
		t.Errorf("unexpected message: %s", res.Errors[0].Message)
	}
	if res.Errors[2].File != "utils.py" || res.Errors[2].Line != 3 {
		t.Errorf("expected utils.py:3, got %s:%d", res.Errors[2].File, res.Errors[2].Line)
	}
}

func TestParseErrorsEmpty(t *testing.T) {
	res := ParseErrors("")
	if len(res.Errors) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(res.Errors))
	}
}

func TestRemediateAllowlist(t *testing.T) {
	res := &MypyResult{
		Errors: []MypyError{
			{File: "a.py", Line: 1, Message: "m1"},
			{File: "b.py", Line: 2, Message: "m2"},
			{File: "a.py", Line: 3, Message: "m3"},
		},
	}
	_, allowlist, _ := Remediate(res, "")
	if len(allowlist) != 2 {
		t.Fatalf("expected 2 allowlisted files, got %d", len(allowlist))
	}
}

func TestBoundedRemediate(t *testing.T) {
	// mypy may not be installed, so just test the round limit.
	_, _, err := BoundedRemediate([]string{}, MaxRounds+1)
	if err == nil || !strings.Contains(err.Error(), "max rounds") {
		t.Fatalf("expected max rounds error, got %v", err)
	}
}

func TestCheckAllowlist(t *testing.T) {
	if err := CheckAllowlist([]string{"a.py"}, []string{"a.py"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := CheckAllowlist([]string{"b.py"}, []string{"a.py"}); err == nil {
		t.Fatal("expected error for out-of-allowlist file")
	}
}

func TestHasNewTypeIgnore(t *testing.T) {
	if !HasNewTypeIgnore("x = 1  # type: ignore") {
		t.Fatal("expected true")
	}
	if HasNewTypeIgnore("x = 1") {
		t.Fatal("expected false")
	}
}
