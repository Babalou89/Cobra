package worker

import "testing"

func TestDecodeActionDeterministic(t *testing.T) {
	raw := `{"zeta": {"path": "z"}, "alpha": {"path": "a"}, "mid": "text"}`
	for i := 0; i < 100; i++ {
		act, err := parseAction(raw)
		if err != nil {
			t.Fatal(err)
		}
		if act.Tool != "alpha" {
			t.Fatalf("iteration %d: tool=%q, want alpha (sorted key order)", i, act.Tool)
		}
	}
}

func TestDecodeActionPrecedence(t *testing.T) {
	// tool+args beats done.
	act, err := parseAction(`{"tool":"file_write","args":{"path":"a","content":"b"},"done":true,"summary":"s"}`)
	if err != nil || act.Done || act.Tool != "file_write" || act.Args["path"] != "a" {
		t.Fatalf("tool+args must beat done: %+v %v", act, err)
	}
	// done beats tool-name-keyed.
	act, err = parseAction(`{"done":true,"summary":"s","note":{"text":"x"}}`)
	if err != nil || !act.Done {
		t.Fatalf("done must beat keyed: %+v %v", act, err)
	}
	// plain tool without args + done is a done signal.
	act, err = parseAction(`{"tool":"x","done":true}`)
	if err != nil || !act.Done {
		t.Fatalf("done without args: %+v %v", act, err)
	}
	// tool+args beats a keyed sibling.
	act, err = parseAction(`{"aaa":{"p":1},"tool":"file_read","args":{"path":"q"}}`)
	if err != nil || act.Tool != "file_read" {
		t.Fatalf("tool+args must beat keyed: %+v %v", act, err)
	}
}
