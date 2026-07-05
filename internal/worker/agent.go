package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"cobra/internal/backend"
	"cobra/internal/ctxdiet"
)

// Agent drives one backend through the tool registry until the model
// signals done or turns run out. It cannot verify and it cannot touch the
// repository — both live on the cage side of the binary.
type Agent struct {
	Backend     backend.Backend
	Tools       *Registry
	Budget      ctxdiet.Budget
	MaxTurns    int // safety valve per attempt; the outer loop re-runs until verify passes or strikes lock
	Temperature float64
	Log         func(format string, args ...any)
	// Trace, when set, receives every dispatched action with its measured
	// outcome. The binary wires this to the trajectory log — the evolver's
	// input comes from dispatch facts, not model self-report.
	Trace func(turn int, tool string, ok bool, summary string)
}

type action struct {
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Done    bool           `json:"done"`
	Summary string         `json:"summary"`
}

// Run executes one attempt: task + DOD text (+ previous verify report) in,
// tool calls until the model signals done. The context diet is enforced
// every turn: trajectory compression, per-message clamps, hard budget.
func (a *Agent) Run(task, dodText, verifyReport string) error {
	if a.MaxTurns <= 0 {
		a.MaxTurns = 60
	}
	if a.Temperature == 0 {
		a.Temperature = 0.2
	}
	logf := a.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}

	system := SystemPrompt(a.Tools.Describe())
	var user strings.Builder
	user.WriteString("Task:\n" + task + "\n")
	if dodText != "" {
		user.WriteString("\nDefinition of done (the external verifier checks exactly this):\n" + dodText + "\n")
	}
	if verifyReport != "" {
		user.WriteString("\nPrevious verify report — fix every failure listed:\n" + verifyReport + "\n")
	}

	var steps []ctxdiet.Step
	for turn := 1; turn <= a.MaxTurns; turn++ {
		msgs := a.buildMessages(system, user.String(), steps)
		var contents []string
		for _, m := range msgs {
			contents = append(contents, m.Content)
		}
		if err := a.Budget.CheckTotal(contents); err != nil {
			// Over budget even after compression: drop the oldest steps.
			if len(steps) > 4 {
				steps = steps[len(steps)-4:]
				msgs = a.buildMessages(system, user.String(), steps)
			} else {
				return fmt.Errorf("context diet: %w", err)
			}
		}

		reply, err := a.Backend.Chat(msgs, a.Temperature, 2048)
		if err != nil {
			return fmt.Errorf("backend %s: %w", a.Backend.Name(), err)
		}
		act, err := parseAction(reply)
		if err != nil {
			steps = append(steps,
				ctxdiet.Step{Role: "assistant", Content: a.Budget.ClampMessage(reply)},
				ctxdiet.Step{Role: "tool", Content: "protocol error: " + err.Error() + " — reply with exactly one JSON object"})
			continue
		}
		if act.Done {
			logf("worker signalled done after %d turn(s): %s", turn, act.Summary)
			if a.Trace != nil {
				a.Trace(turn, "done", true, act.Summary)
			}
			return nil
		}
		result := a.Tools.Dispatch(act.Tool, act.Args)
		status := "ok"
		if !result.OK {
			status = "error"
		}
		logf("turn %d: %s → %s", turn, act.Tool, status)
		if a.Trace != nil {
			a.Trace(turn, act.Tool, result.OK, firstLine(result.Output))
		}
		steps = append(steps,
			ctxdiet.Step{Role: "assistant", Content: a.Budget.ClampMessage(reply)},
			ctxdiet.Step{Role: "tool", Content: fmt.Sprintf("[%s %s] %s", act.Tool, status, a.Budget.ClampMessage(result.Output))})
		steps = ctxdiet.Compress(steps, 8)
	}
	return fmt.Errorf("worker used all %d turns without signalling done", a.MaxTurns)
}

func (a *Agent) buildMessages(system, user string, steps []ctxdiet.Step) []backend.Msg {
	msgs := []backend.Msg{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	for _, s := range steps {
		role := "assistant"
		if s.Role == "tool" {
			role = "tool"
		}
		msgs = append(msgs, backend.Msg{Role: role, Content: s.Content})
	}
	return msgs
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// parseAction extracts the first JSON object from a model reply, tolerating
// prose or code fences around it.
func parseAction(reply string) (*action, error) {
	start := strings.IndexByte(reply, '{')
	if start < 0 {
		return nil, fmt.Errorf("no JSON object found")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(reply); i++ {
		c := reply[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				raw := []byte(reply[start : i+1])
				var act action
				if err := json.Unmarshal(raw, &act); err != nil {
					return nil, fmt.Errorf("invalid action JSON: %w", err)
				}
				if !act.Done && act.Tool == "" {
					return nil, fmt.Errorf(`action must set "tool" or "done"`)
				}
				// Local models often flatten args to the top level
				// ({"tool":"x","path":...} instead of nesting under
				// "args") — accept both shapes.
				if len(act.Args) == 0 {
					var flat map[string]any
					if json.Unmarshal(raw, &flat) == nil {
						act.Args = map[string]any{}
						for k, v := range flat {
							switch k {
							case "tool", "done", "summary", "args":
							default:
								act.Args[k] = v
							}
						}
					}
				}
				return &act, nil
			}
		}
	}
	return nil, fmt.Errorf("unterminated JSON object")
}
