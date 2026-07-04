// Pluggable external checkers: when the DOD names tools (ruff, pytest,
// mypy, bandit, …) the cage shells out to them. A named tool that is not
// installed is a failure — the DOD asked for it. Exit 0 = pass.
package cage

import (
	"fmt"
	"os/exec"
	"strings"
)

// RunPluggable executes every tool the DOD names and returns one failure
// string per non-zero exit.
func RunPluggable(dir string, tools []PlugTool) []string {
	var failures []string
	for _, t := range tools {
		if strings.TrimSpace(t.Run) == "" {
			continue
		}
		name := t.Name
		if name == "" {
			name = strings.Fields(t.Run)[0]
		}
		cmd := exec.Command("bash", "-c", t.Run)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			failures = append(failures, fmt.Sprintf("tool %s: %q failed: %s", name, t.Run, lastLines(string(out), 5)))
		}
	}
	return failures
}
