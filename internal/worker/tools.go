package worker

import (
	"fmt"
	"sort"
	"strings"

	"cobra/internal/jail"
)

// ToolResult is what every tool hands back to the loop.
type ToolResult struct {
	OK     bool
	Output string
}

// ToolFunc executes one tool call.
type ToolFunc func(args map[string]any) ToolResult

// Tool is one registered capability.
type Tool struct {
	Name  string
	Desc  string
	Usage string // arg hint shown to the model
	Fn    ToolFunc
}

// Registry holds the dispatch table.
type Registry struct {
	tools map[string]*Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]*Tool{}}
}

// Register adds a tool; duplicate names panic at wiring time.
func (r *Registry) Register(t *Tool) {
	if _, dup := r.tools[t.Name]; dup {
		panic("duplicate tool: " + t.Name)
	}
	r.tools[t.Name] = t
}

// Dispatch runs a named tool; unknown names return a failed result rather
// than an error so the model gets corrective feedback.
func (r *Registry) Dispatch(name string, args map[string]any) ToolResult {
	t, ok := r.tools[name]
	if !ok {
		return ToolResult{OK: false, Output: fmt.Sprintf("unknown tool %q — available: %s", name, strings.Join(r.names(), ", "))}
	}
	return t.Fn(args)
}

// Has returns true if the named tool is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Describe renders the tool list for the system prompt.
func (r *Registry) Describe() string {
	var sb strings.Builder
	for _, name := range r.names() {
		t := r.tools[name]
		sb.WriteString(fmt.Sprintf("- %s %s — %s\n", t.Name, t.Usage, t.Desc))
	}
	return sb.String()
}

func (r *Registry) names() []string {
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// DefaultRegistry wires the full toolset against one jail. skillsRoot may
// be empty (no skill library); installed skills appear as one skill_run
// tool, read-only from the worker's side.
func DefaultRegistry(w *jail.Workspace, skillsRoot string) *Registry {
	r := NewRegistry()
	registerFS(r, w)
	registerShell(r, w)
	registerWeb(r)
	registerSys(r, w)
	registerNote(r, w)
	if skillsRoot != "" {
		registerSkills(r, w, skillsRoot)
	}
	return r
}

// RalphRegistry returns the minimal toolset for ralph mode: file_read,
// file_write, note, code_execute. No exploratory tools (list_dir, web_*,
// system_info, skill_run, file_edit). The cage injects file contents into
// FixTarget so the model doesn't need to explore. file_read stays because
// the model may need files the cage didn't inject. code_execute covers
// both shell and inline code execution.
func RalphRegistry(w *jail.Workspace) *Registry {
	r := NewRegistry()
	registerFSReadWrite(r, w)
	registerShellExecute(r, w)
	registerNote(r, w)
	return r
}

// argBool pulls a boolean argument with a default.
func argBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

// argString pulls a string argument with a default.
func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n[…%d bytes truncated]", len(s)-n)
}
