package worker

import (
	"fmt"
	"runtime"

	"cobra/internal/jail"
)

// registerSys wires the read-only system probe.
func registerSys(r *Registry, w *jail.Workspace) {
	r.Register(&Tool{
		Name:  "system_info",
		Usage: `{}`,
		Desc:  "report OS, architecture, CPU count, and the workspace root",
		Fn: func(args map[string]any) ToolResult {
			out := fmt.Sprintf("os=%s arch=%s cpus=%d workspace=%s",
				runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), w.Root)
			return ToolResult{OK: true, Output: out}
		},
	})
}
