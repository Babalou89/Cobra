package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cobra/internal/backend"
	"cobra/internal/ctxdiet"
)

// maxRepeat is the in-attempt loop detector's trigger: this many identical
// consecutive actions or protocol errors abort the attempt. The strike
// system watches between attempts; this watches within one.
const maxRepeat = 3

// Agent drives one backend through the tool registry until the model
// signals done, the dead-man's switch fires, or the loop detector trips.
// It cannot verify and it cannot touch the repository — both live on the
// cage side of the binary.
type Agent struct {
	Backend        backend.Backend
	Tools          *Registry
	Budget         ctxdiet.Budget
	MaxTurns       int           // per attempt; the outer loop re-runs until verify passes or strikes lock
	AttemptTimeout time.Duration // dead-man's switch: wall-clock cap per attempt
	MaxGenTokens   int           // generation cap per reply
	Temperature    float64

	// Ralph mode: context is reconstructed every turn — task + DOD +
	// NOTES.md + last verify report + the last couple of actions. Nothing
	// accumulates, so nothing overflows. NotesPath is the model's only
	// durable memory, a plain file inside the jail.
	Ralph     bool
	NotesPath string
	PlanPath  string // path to PLAN.md (written by planner between attempts)

	Log     func(format string, args ...any)
	Trace   func(turn int, tool string, ok bool, summary string)
	Observe func(kind string, fields map[string]any) // live feed for cage watch
}

type action struct {
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Done    bool           `json:"done"`
	Summary string         `json:"summary"`
}

// Run executes one attempt.
func (a *Agent) Run(task, fixTarget, verifyReport string) error {
	if a.MaxTurns <= 0 {
		a.MaxTurns = 60
	}
	if a.AttemptTimeout <= 0 {
		a.AttemptTimeout = 15 * time.Minute
	}
	if a.MaxGenTokens <= 0 {
		a.MaxGenTokens = 4096
	}
	if a.Temperature == 0 {
		a.Temperature = 0.6
	}
	logf := a.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	observe := a.Observe
	if observe == nil {
		observe = func(string, map[string]any) {}
	}

	deadline := time.Now().Add(a.AttemptTimeout)
	system := SystemPrompt(a.Tools.Describe(), a.Ralph)

	var steps []ctxdiet.Step
	lastSig := ""
	repeatCount := 0
	repeated := func(sig string) bool {
		if sig == lastSig {
			repeatCount++
		} else {
			lastSig = sig
			repeatCount = 1
		}
		return repeatCount >= maxRepeat
	}

	for turn := 1; turn <= a.MaxTurns; turn++ {
		if time.Now().After(deadline) {
			return fmt.Errorf("dead-man's switch: attempt exceeded %s at turn %d", a.AttemptTimeout, turn)
		}

		user := a.buildUser(task, fixTarget, verifyReport)
		msgs := a.buildMessages(system, user, steps)
		for a.overBudget(msgs) && len(steps) > 2 {
			steps = steps[2:]
			msgs = a.buildMessages(system, user, steps)
		}
		if a.overBudget(msgs) {
			return fmt.Errorf("context diet: task + system prompt alone exceed the budget")
		}
		ctxTokens := a.payloadTokens(msgs)

		reply, err := a.Backend.Chat(msgs, a.Temperature, a.MaxGenTokens)
		if err != nil {
			// A context overflow the estimator missed is recoverable:
			// shed half the history and retry the turn.
			if strings.Contains(err.Error(), "context size") && len(steps) > 2 {
				logf("context overflow from backend — shedding %d history steps", len(steps)/2)
				steps = steps[len(steps)/2:]
				continue
			}
			return fmt.Errorf("backend %s: %w", a.Backend.Name(), err)
		}
		observe("reply", map[string]any{
			"turn": turn, "text": firstChars(reply, 400),
			"ctx_tokens": ctxTokens, "ctx_budget": a.Budget.MaxTokens,
		})

		act, err := parseAction(reply)
		if err != nil {
			sig := "protocol:" + firstChars(err.Error(), 60)
			observe("result", map[string]any{"turn": turn, "tool": "protocol", "ok": false, "text": err.Error()})
			if repeated(sig) {
				return fmt.Errorf("loop detected: %d consecutive protocol errors (%s) — aborting attempt", repeatCount, err)
			}
			steps = append(steps,
				ctxdiet.Step{Role: "assistant", Content: a.Budget.ClampMessage(reply)},
				ctxdiet.Step{Role: "tool", Content: "protocol error: " + err.Error() + " — reply with exactly one JSON object; use file_write with \"append\": true to write large files in chunks"})
			steps = a.trim(steps)
			continue
		}
		if act.Done {
			logf("worker signalled done after %d turn(s): %s", turn, act.Summary)
			if a.Trace != nil {
				a.Trace(turn, "done", true, act.Summary)
			}
			observe("action", map[string]any{"turn": turn, "tool": "done", "text": act.Summary})
			return nil
		}

		sig := act.Tool + "|" + argsHash(act.Args)
		if repeated(sig) {
			return fmt.Errorf("loop detected: %s called %d times in a row with identical arguments — aborting attempt", act.Tool, repeatCount)
		}

		observe("action", map[string]any{"turn": turn, "tool": act.Tool, "text": firstChars(compactArgs(act.Args), 200)})
		result := a.Tools.Dispatch(act.Tool, act.Args)
		status := "ok"
		if !result.OK {
			status = "error"
		}
		logf("turn %d: %s → %s", turn, act.Tool, status)
		if a.Trace != nil {
			a.Trace(turn, act.Tool, result.OK, firstLine(result.Output))
		}
		observe("result", map[string]any{"turn": turn, "tool": act.Tool, "ok": result.OK, "text": firstChars(result.Output, 300)})

		steps = append(steps,
			ctxdiet.Step{Role: "assistant", Content: a.Budget.ClampMessage(reply)},
			ctxdiet.Step{Role: "tool", Content: fmt.Sprintf("[%s %s] %s", act.Tool, status, ctxdiet.ClampTo(result.Output, a.toolCeiling()))})
		steps = a.trim(steps)
	}
	return fmt.Errorf("worker used all %d turns without signalling done", a.MaxTurns)
}

// buildUser assembles the per-turn user message. In ralph mode this is the
// whole context reconstruction: everything the model needs, every turn.
func (a *Agent) buildUser(task, fixTarget, verifyReport string) string {
	var sb strings.Builder
	sb.WriteString("Task:\n" + task + "\n")
	if fixTarget != "" {
		sb.WriteString("\nFix these failures:\n" + fixTarget + "\n")
	}
	if verifyReport != "" && verifyReport != fixTarget {
		sb.WriteString("\nVerify report:\n" + verifyReport + "\n")
	}
	result := sb.String()
	lines := strings.Split(result, "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
		result = strings.Join(lines, "\n")
	}
	return result
}



// trim bounds the carried history: ralph mode keeps only the last
// exchange pair, conversational mode keeps a compressed window.
func (a *Agent) trim(steps []ctxdiet.Step) []ctxdiet.Step {
	if a.Ralph {
		if len(steps) > 2 {
			return steps[len(steps)-2:]
		}
		return steps
	}
	return ctxdiet.Compress(steps, 6)
}

// toolCeiling caps a single tool result well below the prompt ceiling —
// tool output is the main context flooder.
func (a *Agent) toolCeiling() int {
	if a.Budget.PromptCeiling > 0 {
		return a.Budget.PromptCeiling / 4
	}
	return 1500
}

func (a *Agent) overBudget(msgs []backend.Msg) bool {
	return a.payloadTokens(msgs) > a.Budget.MaxTokens && a.Budget.MaxTokens > 0
}

func (a *Agent) payloadTokens(msgs []backend.Msg) int {
	total := 0
	for _, m := range msgs {
		total += ctxdiet.Estimate(m.Content)
	}
	return total
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

func firstChars(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func compactArgs(args map[string]any) string {
	data, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(data)
}

func argsHash(args map[string]any) string {
	data, _ := json.Marshal(args)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// repairJSON escapes raw control characters found inside JSON string
// literals — the dominant local-model failure when a file body rides
// inside a JSON action ("content":"line1<newline>line2").
func repairJSON(raw []byte) []byte {
	var out []byte
	inString := false
	escaped := false
	for _, c := range raw {
		if inString {
			switch {
			case escaped:
				escaped = false
				// \' is invalid JSON (but valid in Python) — models emit it when
				// embedding code with single quotes. Drop the backslash we already
				// wrote; the model meant a bare '. Deterministic repair of a
				// single-answer syntax error — never an interpretation of intent.
				if c == '\'' && len(out) > 0 && out[len(out)-1] == '\\' {
					out = out[:len(out)-1]
				}
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			case c == '\n':
				out = append(out, '\\', 'n')
				continue
			case c == '\r':
				out = append(out, '\\', 'r')
				continue
			case c == '\t':
				out = append(out, '\\', 't')
				continue
			case c < 0x20:
				out = append(out, []byte(fmt.Sprintf("\\u%04x", c))...)
				continue
			}
		} else if c == '"' {
			inString = true
		}
		out = append(out, c)
	}
	return out
}

// parseAction extracts the first JSON object from a model reply,
// tolerating prose or code fences around it, flattened args, and raw
// control characters inside string values.
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
				return decodeAction([]byte(reply[start : i+1]))
			}
		}
	}
	return nil, fmt.Errorf("unterminated JSON object — if writing a large file, use file_write with \"append\": true and smaller chunks")
}

func decodeAction(raw []byte) (*action, error) {
	var act action
	err := json.Unmarshal(raw, &act)
	if err != nil {
		// Second chance: deterministically repair control chars in strings.
		raw = repairJSON(raw)
		if err2 := json.Unmarshal(raw, &act); err2 != nil {
			return nil, fmt.Errorf("invalid action JSON: %v", err)
		}
	}
	if !act.Done && act.Tool == "" {
		return nil, fmt.Errorf(`action must set "tool" or "done"`)
	}
	// Local models often flatten args to the top level — accept both shapes.
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
