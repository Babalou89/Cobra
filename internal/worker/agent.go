package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"cobra/internal/backend"
	"cobra/internal/ctxdiet"
)

// maxRepeat is the in-attempt loop detector's trigger: this many identical
// consecutive actions or protocol errors abort the attempt. The strike
// system watches between attempts; this watches within one.
const maxRepeat = 3

// slidingWindowSize is how many recent actions the loop detector examines.
const slidingWindowSize = 6

// fileReadThreshold is the max number of file_read calls allowed in the
// sliding window before we declare the model stuck.
const fileReadThreshold = 4

// ErrTurnsExhausted is returned by Run when the turn budget ran out without
// the model signalling done. The work on disk may still be correct, so the
// caller verifies once instead of discarding the attempt.
var ErrTurnsExhausted = errors.New("turns exhausted")

// Agent drives one backend through the tool registry until the model
// signals done, the dead-man's switch fires, or the loop detector trips.
type Agent struct {
	Backend        backend.Backend
	Tools          *Registry
	Budget         ctxdiet.Budget
	MaxTurns       int
	AttemptTimeout time.Duration
	MaxGenTokens   int
	Temperature    float64

	Ralph     bool
	Instruct  bool // instruct mode: stripped-down prompt, no notes, no brain
	NotesPath string
	PlanPath  string
	BrainPath string // .ai/brain.md — project memory injected every turn

	Log     func(format string, args ...any)
	Trace   func(turn int, tool string, ok bool, summary string)
	Observe func(kind string, fields map[string]any)
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
	var system string
	if a.Instruct {
		system = InstructPrompt(a.Tools.Describe())
	} else {
		system = SystemPrompt(a.Tools.Describe(), a.Ralph)
	}

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

	recentTools := make([]string, 0, slidingWindowSize)
	fileReadCount := 0

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

		recentTools = append(recentTools, act.Tool)
		if len(recentTools) > slidingWindowSize {
			recentTools = recentTools[1:]
		}
		readCount := 0
		for _, t := range recentTools {
			if t == "file_read" {
				readCount++
			}
		}
		if readCount >= fileReadThreshold {
			return fmt.Errorf("loop detected: %d file_read calls in last %d actions — model is stuck reading instead of writing; aborting attempt", readCount, slidingWindowSize)
		}

		if act.Tool == "file_read" {
			fileReadCount++
			readCap := a.MaxTurns / 3
			if readCap < 3 {
				readCap = 3
			}
			if fileReadCount > readCap {
				return fmt.Errorf("file_read cap exceeded: %d reads used (cap %d of %d max turns) — stop reading, use file_write", fileReadCount, readCap, a.MaxTurns)
			}
		}

		if a.Ralph && act.Tool == "file_edit" {
			observe("result", map[string]any{"turn": turn, "tool": "file_edit", "ok": false, "text": "file_edit disabled in ralph mode — use file_write instead"})
			steps = append(steps,
				ctxdiet.Step{Role: "assistant", Content: a.Budget.ClampMessage(reply)},
				ctxdiet.Step{Role: "tool", Content: "[file_edit BLOCKED] file_edit is disabled in ralph mode. Use file_write to write the entire file. Do not try to match exact strings — just write the complete file with your changes."})
			steps = a.trim(steps)
			continue
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
	return fmt.Errorf("%w: worker used all %d turns without signalling done", ErrTurnsExhausted, a.MaxTurns)
}

// buildUser assembles the per-turn user message. In ralph mode this is the
// whole context reconstruction: everything the model needs, every turn.
// Injection order: task → failures → verify report → brain.md → NOTES.md → PLAN.md
func (a *Agent) buildUser(task, fixTarget, verifyReport string) string {
	var sb strings.Builder
	sb.WriteString("Task:\n" + task + "\n")
	if fixTarget != "" {
		sb.WriteString("\nFix these failures:\n" + fixTarget + "\n")
	}
	if verifyReport != "" && verifyReport != fixTarget {
		sb.WriteString("\nVerify report:\n" + verifyReport + "\n")
	}

	// Project memory (brain.md) — the model's persistent world model.
	// Tech stack, version history, known issues, session log, rules.
	// Injected every turn so the model never loses sight of what the
	// project IS, even after conversation reset.
	// Skipped in instruct mode — the task is self-contained.
	if a.BrainPath != "" && !a.Instruct {
		brain := "(no project memory found)"
		if data, err := os.ReadFile(a.BrainPath); err == nil && len(data) > 0 {
			brain = ctxdiet.ClampTo(string(data), 2000)
		}
		sb.WriteString("\nPROJECT MEMORY (brain.md):\n" + brain + "\n")
	}

	// In instruct mode, no notes — task is self-contained.
	if a.Ralph && !a.Instruct {
		notes := "(empty - you have recorded nothing yet)"
		if a.NotesPath != "" {
			if data, err := os.ReadFile(a.NotesPath); err == nil && len(data) > 0 {
				notes = ctxdiet.ClampTo(string(data), 1500)
			}
		}
		sb.WriteString("\nNOTES.md - your only durable memory (conversation resets every turn):\n" + notes + "\n")
	}
	if a.PlanPath != "" {
		plan := "(no plan - planner not run yet)"
		if data, err := os.ReadFile(a.PlanPath); err == nil && len(data) > 0 {
			plan = ctxdiet.ClampTo(string(data), 1500)
		}
		sb.WriteString("\nPlanner diagnosis - follow this plan:\n" + plan + "\n")
	}
	result := sb.String()
	lines := strings.Split(result, "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
		result = strings.Join(lines, "\n")
	}
	return result
}

// trim bounds the carried history: ralph mode keeps the last 4 exchange
// pairs (enough to see read patterns), conversational mode keeps a
// compressed window.
func (a *Agent) trim(steps []ctxdiet.Step) []ctxdiet.Step {
	if a.Ralph {
		if len(steps) > 8 {
			return steps[len(steps)-8:]
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

func repairJSON(raw []byte) []byte {
	var out []byte
	inString := false
	escaped := false
	for _, c := range raw {
		if inString {
			switch {
			case escaped:
				escaped = false
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

var xmlToolCallRe = regexp.MustCompile(`(?s)<function=([^>]+)>\s*(.*?)\s*</function>`)
var xmlParamRe = regexp.MustCompile(`(?s)<parameter=([^>]+)>(.*?)</parameter>`)

// parseXMLAction extracts a tool call from Qwen XML format.
func parseXMLAction(reply string) (*action, error) {
	match := xmlToolCallRe.FindStringSubmatch(reply)
	if match == nil {
		return nil, fmt.Errorf("no XML tool call found")
	}
	toolName := strings.TrimSpace(match[1])
	paramsBlock := match[2]
	args := make(map[string]any)
	paramMatches := xmlParamRe.FindAllStringSubmatch(paramsBlock, -1)
	for _, pm := range paramMatches {
		if len(pm) >= 3 {
			args[strings.TrimSpace(pm[1])] = strings.TrimSpace(pm[2])
		}
	}
	return &action{Tool: toolName, Args: args}, nil
}

func parseAction(reply string) (*action, error) {
	// Try JSON first
	start := strings.IndexByte(reply, '{')
	if start >= 0 {
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
	}
	// Fallback: try XML tool-call format (Qwen native)
	if act, err := parseXMLAction(reply); err == nil {
		return act, nil
	}
	return nil, fmt.Errorf("no JSON or XML tool call found")
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
		// Qwen often emits {"tool_name": {...}} or {"tool_name": "value"} instead
		// of {"tool": "...", "args": {...}}. Detect both patterns.
		var flat map[string]any
		if json.Unmarshal(raw, &flat) == nil {
			// Sorted keys: Go map order is random, and a reply with two
			// candidate keys must decode the same way every time.
			keys := make([]string, 0, len(flat))
			for k := range flat {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := flat[k]
				switch k {
				case "done", "summary", "tool", "args":
					continue
				}
				switch val := v.(type) {
				case map[string]any:
					// {"file_write": {"path": "...", "content": "..."}}
					act.Tool = k
					act.Args = val
					return &act, nil
				case string:
					// {"note": "some text"} or {"done": true, "summary": "..."}
					act.Tool = k
					act.Args = map[string]any{"text": val}
					return &act, nil
				}
			}
		}
		return nil, fmt.Errorf(`action must set "tool" or "done"`)
	}
	// Local models often flatten args to the top level — accept both shapes.
	// Precedence: "tool"+"args" > "done" > tool-name-keyed.
	if len(act.Args) > 0 && act.Tool != "" {
		act.Done = false
		return &act, nil
	}
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
