package worker

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"cobra/internal/jail"
)

// blockedBinaries are refused only in command position (start of the
// command or after ; & | $( or backtick) — a path or argument merely
// containing one of these words is fine. A directory named "sudo" must be
// inspectable; running sudo must not be. Version control is on the list
// because repository writes belong to the cage, not to the model.
var blockedBinaryPat = regexp.MustCompile("(?i)(^|[;&|]|\\$\\(|`)\\s*(sudo|git|ssh|scp|shutdown|reboot|mkfs[.a-z]*|crontab|chown|dd)\\b")

// blockedSubstrings are dangerous anywhere in a command.
var blockedSubstrings = []string{
	"rm -rf /",
	"rm -rf ~",
	"> /dev/sd",
	"chmod -r 777",
	"| sh",
	"| bash",
}

func guard(command string) error {
	if m := blockedBinaryPat.FindStringSubmatch(command); m != nil {
		return fmt.Errorf("blocked command %q — not allowed inside the jail", m[2])
	}
	lowered := strings.ToLower(command)
	for _, blocked := range blockedSubstrings {
		if strings.Contains(lowered, blocked) {
			return fmt.Errorf("blocked command (matched %q) — not allowed inside the jail", strings.TrimSpace(blocked))
		}
	}
	return nil
}

// registerShell wires shell execution, cwd pinned to the jail root with a
// hard timeout and a blocked-command guard.
func registerShell(r *Registry, w *jail.Workspace) {
	runInJail := func(command string, timeout time.Duration) ToolResult {
		if err := guard(command); err != nil {
			return ToolResult{OK: false, Output: err.Error()}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-c", command)
		cmd.Dir = w.Root
		out, err := cmd.CombinedOutput()
		result := clip(string(out), 4000)
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{OK: false, Output: result + "\n[command timed out]"}
		}
		if err != nil {
			return ToolResult{OK: false, Output: result + "\n[exit error: " + err.Error() + "]"}
		}
		return ToolResult{OK: true, Output: result}
	}

	r.Register(&Tool{
		Name:  "shell",
		Usage: `{"command": "ls -la"}`,
		Desc:  "run a shell command inside the workspace (60s timeout, guarded)",
		Fn: func(args map[string]any) ToolResult {
			command := argString(args, "command", "")
			if strings.TrimSpace(command) == "" {
				return ToolResult{OK: false, Output: "command must not be empty"}
			}
			return runInJail(command, 60*time.Second)
		},
	})
	r.Register(&Tool{
		Name:  "code_execute",
		Usage: `{"language": "python", "code": "print(1)"}`,
		Desc:  "execute a code snippet inside the workspace (python or bash)",
		Fn: func(args map[string]any) ToolResult {
			lang := argString(args, "language", "python")
			code := argString(args, "code", "")
			switch lang {
			case "python", "python3":
				abs, err := w.WriteFile(".cage_snippet.py", []byte(code))
				if err != nil {
					return ToolResult{OK: false, Output: err.Error()}
				}
				return runInJail("python3 "+abs, 60*time.Second)
			case "bash", "sh":
				return runInJail(code, 60*time.Second)
			default:
				return ToolResult{OK: false, Output: "unsupported language " + lang + " (python|bash)"}
			}
		},
	})
}
