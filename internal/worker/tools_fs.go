package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cobra/internal/jail"
)

// protectedWrite blocks writes into the cage's own records even when the
// jail encloses them (jail.root: "."). The skill library, state, and
// audit trail are the binary's — the worker earns nothing except through
// the gate.
func protectedWrite(abs string) error {
	sep := string(filepath.Separator)
	for _, part := range strings.Split(abs, sep) {
		if part == ".cage" || part == ".git" {
			return fmt.Errorf("path %s is protected — the worker cannot write into %s", abs, part)
		}
	}
	return nil
}

// registerFS wires file tools. Every path is forced through the jail —
// nothing the model names can land outside it.
func registerFS(r *Registry, w *jail.Workspace) {
	r.Register(&Tool{
		Name:  "file_read",
		Usage: `{"path": "relative/path"}`,
		Desc:  "read a file inside the workspace",
		Fn: func(args map[string]any) ToolResult {
			path := argString(args, "path", "")
			if path == "" {
				return ToolResult{OK: false, Output: `missing "path" argument`}
			}
			data, err := w.ReadFile(path)
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			return ToolResult{OK: true, Output: clip(string(data), 8000)}
		},
	})
	r.Register(&Tool{
		Name:  "file_write",
		Usage: `{"path": "relative/path", "content": "...", "append": false}`,
		Desc:  "write a file inside the workspace; set append:true to add a chunk to an existing file (use for large files)",
		Fn: func(args map[string]any) ToolResult {
			path := argString(args, "path", "")
			content := argString(args, "content", "")
			if path == "" {
				return ToolResult{OK: false, Output: `missing "path" argument`}
			}
			abs, err := w.Resolve(path)
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			if err := protectedWrite(abs); err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			if argBool(args, "append", false) {
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					return ToolResult{OK: false, Output: err.Error()}
				}
				f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return ToolResult{OK: false, Output: err.Error()}
				}
				defer f.Close()
				if _, err := f.WriteString(content); err != nil {
					return ToolResult{OK: false, Output: err.Error()}
				}
				return ToolResult{OK: true, Output: fmt.Sprintf("appended %d bytes to %s", len(content), abs)}
			}
			if abs, err = w.WriteFile(path, []byte(content)); err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			return ToolResult{OK: true, Output: fmt.Sprintf("wrote %d bytes to %s", len(content), abs)}
		},
	})
	r.Register(&Tool{
		Name:  "file_edit",
		Usage: `{"path": "relative/path", "old": "...", "new": "..."}`,
		Desc:  "replace an exact string once in a file inside the workspace",
		Fn: func(args map[string]any) ToolResult {
			path := argString(args, "path", "")
			oldStr := argString(args, "old", "")
			newStr := argString(args, "new", "")
			if oldStr == "" {
				return ToolResult{OK: false, Output: "old string must not be empty"}
			}
			if abs, rerr := w.Resolve(path); rerr == nil {
				if perr := protectedWrite(abs); perr != nil {
					return ToolResult{OK: false, Output: perr.Error()}
				}
			}
			data, err := w.ReadFile(path)
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			text := string(data)
			count := strings.Count(text, oldStr)
			if count == 0 {
				return ToolResult{OK: false, Output: "old string not found in " + path}
			}
			if count > 1 {
				return ToolResult{OK: false, Output: fmt.Sprintf("old string occurs %d times in %s — make it unique", count, path)}
			}
			if _, err := w.WriteFile(path, []byte(strings.Replace(text, oldStr, newStr, 1))); err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			return ToolResult{OK: true, Output: "edited " + path}
		},
	})
	r.Register(&Tool{
		Name:  "list_dir",
		Usage: `{"path": "."}`,
		Desc:  "list a directory inside the workspace",
		Fn: func(args map[string]any) ToolResult {
			path := argString(args, "path", ".")
			abs, err := w.Resolve(path)
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			entries, err := os.ReadDir(abs)
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			var sb strings.Builder
			for _, e := range entries {
				if e.IsDir() {
					sb.WriteString(e.Name() + "/\n")
				} else {
					sb.WriteString(e.Name() + "\n")
				}
			}
			return ToolResult{OK: true, Output: clip(sb.String(), 4000)}
		},
	})
}
