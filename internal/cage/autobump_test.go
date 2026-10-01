package cage

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cobra/internal/config"
)

func bumpRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ai"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".ai", "brain.md"), []byte(strings.Repeat("l\n", 12)), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".ai", "VERSION"), []byte("1.2.9\n"), 0o644)
	for _, a := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "i"}} {
		c := exec.Command("git", a...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "work.txt"), []byte("x"), 0o644)
	return dir
}

func hasBumpFailure(f []string) bool {
	for _, s := range f {
		if strings.Contains(s, "VERSION") {
			return true
		}
	}
	return false
}

func TestAutoVersionBumpCoreCheck(t *testing.T) {
	dir := bumpRepo(t)
	cfg := config.Defaults()
	if hasBumpFailure(CoreChecks(dir, cfg)) {
		t.Fatal("auto bump enabled: missing bump must not fail the attempt")
	}
	cfg.AutoVersionBump = false
	if !hasBumpFailure(CoreChecks(dir, cfg)) {
		t.Fatal("auto bump disabled: missing bump must still fail")
	}
}

func TestAutoVersionBumpBumpsPatchOnce(t *testing.T) {
	dir := bumpRepo(t)
	v, bumped, err := BumpVersion(dir)
	if err != nil || !bumped || v != "1.2.10" {
		t.Fatalf("v=%q bumped=%v err=%v", v, bumped, err)
	}
	// Second call: file already differs from HEAD, no double bump.
	v, bumped, err = BumpVersion(dir)
	if err != nil || bumped || v != "1.2.10" {
		t.Fatalf("double bump: v=%q bumped=%v err=%v", v, bumped, err)
	}
}

func TestAutoVersionBumpNoChangeNoBump(t *testing.T) {
	dir := bumpRepo(t)
	_ = os.Remove(filepath.Join(dir, "work.txt"))
	if _, bumped, _ := BumpVersion(dir); bumped {
		t.Fatal("no meaningful change: must not bump")
	}
}
