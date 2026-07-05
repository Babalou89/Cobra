package worker

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cobra/internal/jail"
	"cobra/internal/skills"
)

// registerSkills exposes the gate-approved skill library as one tool. The
// worker can RUN skills but cannot create, edit, or remove them — skills
// enter the library only through `cage evolve` and its deterministic gate.
func registerSkills(r *Registry, w *jail.Workspace, skillsRoot string) {
	list, err := skills.List(skillsRoot)
	if err != nil || len(list) == 0 {
		return // no library, no tool — keeps the surface minimal
	}
	byName := map[string]skills.Skill{}
	for _, s := range list {
		byName[s.Name] = s
	}

	r.Register(&Tool{
		Name:  "skill_run",
		Usage: `{"skill": "<name>", "args": "optional arguments", "stdin": "optional input"}`,
		Desc:  "run an installed, verified skill. Installed skills:\n" + skills.Describe(list),
		Fn: func(args map[string]any) ToolResult {
			name := argString(args, "skill", "")
			s, ok := byName[name]
			if !ok {
				return ToolResult{OK: false, Output: fmt.Sprintf("unknown skill %q — installed:\n%s", name, skills.Describe(list))}
			}
			extra := argString(args, "args", "")
			stdin := argString(args, "stdin", "")

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", filepath.Join(s.Dir, "run.sh"))
			if extra != "" {
				cmd.Args = append(cmd.Args, strings.Fields(extra)...)
			}
			if stdin != "" {
				cmd.Stdin = strings.NewReader(stdin)
			}
			cmd.Dir = w.Root // skills act on the workspace, from inside it
			out, err := cmd.CombinedOutput()
			result := clip(string(out), 4000)
			if ctx.Err() == context.DeadlineExceeded {
				return ToolResult{OK: false, Output: result + "\n[skill timed out]"}
			}
			if err != nil {
				return ToolResult{OK: false, Output: result + "\n[skill exit error: " + err.Error() + "]"}
			}
			return ToolResult{OK: true, Output: result}
		},
	})
}
