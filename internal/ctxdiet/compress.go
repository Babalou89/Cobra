package ctxdiet

import "strings"

// Step is one entry of the agent trajectory.
type Step struct {
	Role    string // "assistant" | "tool"
	Content string
}

// Compress keeps the last keepLast steps verbatim and reduces everything
// older to a one-line summary (tool names / first line only). History never
// grows without bound; brain.md and full transcripts never enter context.
func Compress(steps []Step, keepLast int) []Step {
	if keepLast < 0 {
		keepLast = 0
	}
	if len(steps) <= keepLast {
		return steps
	}
	cut := len(steps) - keepLast
	out := make([]Step, 0, len(steps))
	for i, s := range steps {
		if i < cut {
			out = append(out, Step{Role: s.Role, Content: summarize(s)})
		} else {
			out = append(out, s)
		}
	}
	return out
}

func summarize(s Step) string {
	first := s.Content
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = first[:idx]
	}
	if len(first) > 120 {
		first = first[:120] + "…"
	}
	if s.Role == "tool" {
		return "[earlier tool result] " + first
	}
	return "[earlier step] " + first
}
