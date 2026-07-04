package jail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStaysInside(t *testing.T) {
	w, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	abs, err := w.Resolve("sub/dir/file.txt")
	if err != nil {
		t.Fatalf("resolve inside jail: %v", err)
	}
	if !strings.HasPrefix(abs, w.Root) {
		t.Fatalf("resolved path %q not under jail root %q", abs, w.Root)
	}
}

func TestResolveRejectsEscape(t *testing.T) {
	w, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../outside.txt", "../../etc/passwd", "a/../../b", "/etc/passwd"} {
		if _, err := w.Resolve(p); err == nil {
			t.Errorf("path %q should have been rejected", p)
		}
	}
}

func TestWritesLandInJailNotHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	w, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	abs, err := w.WriteFile("notes/todo.txt", []byte("inside the jail\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(abs, w.Root) {
		t.Fatalf("write landed at %q, outside jail %q", abs, w.Root)
	}
	if abs == filepath.Join(home, "notes/todo.txt") {
		t.Fatalf("write landed in HOME")
	}
	data, err := w.ReadFile("notes/todo.txt")
	if err != nil || string(data) != "inside the jail\n" {
		t.Fatalf("read back failed: %v %q", err, data)
	}
}

func TestTeardownRemovesJail(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jail")
	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteFile("x.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := w.Teardown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Root); !os.IsNotExist(err) {
		t.Fatalf("jail still exists after teardown")
	}
}
