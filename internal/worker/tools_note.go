package worker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"cobra/internal/jail"
)

// registerNote wires the model's durable memory: an append-only NOTES.md
// inside the jail. In ralph mode this file is the only state that
// survives between turns.
func registerNote(r *Registry, w *jail.Workspace) {
	r.Register(&Tool{
		Name:  "note",
		Usage: `{"text": "finding or completed step, one line"}`,
		Desc:  "append a line to NOTES.md — your durable memory across turns",
		Fn: func(args map[string]any) ToolResult {
			text := strings.TrimSpace(argString(args, "text", ""))
			if text == "" {
				return ToolResult{OK: false, Output: `missing "text" argument`}
			}
			abs, err := w.Resolve("NOTES.md")
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			defer f.Close()
			line := fmt.Sprintf("- [%s] %s\n", time.Now().Format("15:04"), strings.ReplaceAll(text, "\n", " "))
			if _, err := f.WriteString(line); err != nil {
				return ToolResult{OK: false, Output: err.Error()}
			}
			return ToolResult{OK: true, Output: "noted"}
		},
	})
}
