package fake

import "testing"

func TestFakeReplaysInOrderThenErrors(t *testing.T) {
	b := New("a", "b")
	for _, want := range []string{"a", "b"} {
		got, err := b.Chat(nil, 0, 0)
		if err != nil || got != want {
			t.Fatalf("got %q,%v want %q", got, err, want)
		}
	}
	if _, err := b.Chat(nil, 0, 0); err == nil {
		t.Fatal("expected exhausted error")
	}
	if b.Calls() != 3 {
		t.Fatalf("calls=%d", b.Calls())
	}
}

func TestFakeRepeat(t *testing.T) {
	b := &Backend{Replies: []string{"x"}, Repeat: true}
	for i := 0; i < 5; i++ {
		if got, err := b.Chat(nil, 0, 0); err != nil || got != "x" {
			t.Fatalf("got %q,%v", got, err)
		}
	}
}
