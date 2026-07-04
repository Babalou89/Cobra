// Package gitx is the binary's own view of the repository. Changed files,
// numstat, and the previous version all come from git itself — never from
// agent self-report. Commit authority lives here and only here.
package gitx

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// HasHead reports whether the repository has at least one commit.
func HasHead(dir string) bool {
	_, err := run(dir, "rev-parse", "--verify", "HEAD")
	return err == nil
}

// ChangedFiles returns files modified vs HEAD plus untracked files. With no
// HEAD yet, every tracked and untracked file counts as changed.
func ChangedFiles(dir string) ([]string, error) {
	var names []string
	seen := map[string]bool{}
	add := func(out string) {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !seen[line] {
				seen[line] = true
				names = append(names, line)
			}
		}
	}
	if HasHead(dir) {
		out, err := run(dir, "diff", "--name-only", "HEAD")
		if err != nil {
			return nil, err
		}
		add(out)
	} else {
		out, err := run(dir, "ls-files")
		if err != nil {
			return nil, err
		}
		add(out)
	}
	out, err := run(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	add(out)
	return names, nil
}

// Numstat returns total lines added and removed vs HEAD.
func Numstat(dir string) (added, removed int, err error) {
	if !HasHead(dir) {
		return 0, 0, nil
	}
	out, err := run(dir, "diff", "--numstat", "HEAD")
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if a, e := strconv.Atoi(fields[0]); e == nil {
			added += a
		}
		if r, e := strconv.Atoi(fields[1]); e == nil {
			removed += r
		}
	}
	return added, removed, nil
}

// ShowAt returns a file's content at a ref ("" if absent at that ref).
func ShowAt(dir, ref, path string) string {
	out, err := run(dir, "show", ref+":"+path)
	if err != nil {
		return ""
	}
	return out
}

// VersionFromHistory returns the last committed .ai/VERSION so the agent
// cannot fake a bump by editing history it does not control.
func VersionFromHistory(dir string) string {
	if !HasHead(dir) {
		return ""
	}
	return strings.TrimSpace(ShowAt(dir, "HEAD", ".ai/VERSION"))
}

// CommitAll stages everything and commits. This is the only place in the
// binary that creates commits; the worker package has no path to it.
func CommitAll(dir, message string) error {
	if _, err := run(dir, "add", "-A"); err != nil {
		return err
	}
	out, err := run(dir, "commit", "-m", message)
	if err != nil {
		if strings.Contains(out, "nothing to commit") {
			return nil
		}
		return err
	}
	return nil
}
