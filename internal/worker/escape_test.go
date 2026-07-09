package worker

import "testing"

// Models embedding code in a JSON string often escape single quotes as \'
// (valid Python, invalid JSON). The repair must recover the exact bytes.
func TestRepairSingleQuoteEscape(t *testing.T) {
	reply := `{"tool":"file_write","path":"x.py","content":"d = {\'a\': 1}"}`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("expected \\' repair to succeed, got: %v", err)
	}
	if act.Tool != "file_write" {
		t.Fatalf("tool = %q, want file_write", act.Tool)
	}
	if got, _ := act.Args["content"].(string); got != "d = {'a': 1}" {
		t.Fatalf("content = %q, want d = {'a': 1}", got)
	}
}
