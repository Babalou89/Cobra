# PLANNER IMPLEMENTATION — COMBINED DOD APPROACH

**Date:** 2026-07-31
**Status:** Awaiting Billy's review

---

## What We're Building

The Cobra planner — agent-level speculative decoding. Qwen writes code,
Fable 5 diagnoses failures, cage verifies (deterministic, no LLM).

## Phase 1: Planner Go Code (I write, you review)

### Files to Create

**1. `internal/planner/client.go` (~60 lines)**
Anthropic Messages API client for OneProvider.

```go
package planner

type Client struct {
    BaseURL string  // https://api.oneprovider.dev
    APIKey  string  // sk-b11...b147
    Model   string  // claude-fable-5
}

func (c *Client) Chat(system, user string, maxTokens int) (string, error)
// POST /v1/messages
// Headers: x-api-key, anthropic-version: 2023-06-01
// Body: {"model":"...","max_tokens":2000,"system":"...","messages":[...]}
// Parse: content[0].text (type=="text" block)
// Timeout: 60s
// On error: return "", nil (degrade gracefully)
```

**2. `internal/planner/prompt.go` (~50 lines)**
Assembles the planner context from failures + DOD + source files.

```go
func BuildPrompt(failures []string, dodText string, codeFiles map[string]string) (system, user string)
// System: "You are a code diagnostician for a cage enforcement system..."
// User: failures + DOD criteria + relevant source code
```

**3. `internal/planner/planner.go` (~80 lines)**
Orchestration: extract file paths from failures, read files, call client.

```go
func Diagnose(cfg *config.Config, failures []string, dir string) string
// 1. Check cfg.Planner.Enabled && cfg.Planner.APIKey != ""
// 2. Extract file paths from failure strings (regex)
// 3. Read those files from disk
// 4. Build prompt
// 5. Call Fable 5
// 6. Return plan text (or "" on any error)
```

### Files to Modify

**4. `internal/config/config.go`** — add Planner struct to Config
```go
Planner struct {
    Enabled   bool   `yaml:"enabled"`
    BaseURL   string `yaml:"base_url"`
    APIKey    string `yaml:"api_key"`
    Model     string `yaml:"model"`
    MaxTokens int    `yaml:"max_tokens"`
} `yaml:"planner"`
```
Defaults: enabled=false, base_url="https://api.oneprovider.dev", model="claude-fable-5", max_tokens=2000

**5. `internal/worker/agent.go`** — add PlanPath field
```go
type Agent struct {
    // ... existing fields ...
    PlanPath string // path to PLAN.md (written by planner between attempts)
}
```

**6. `internal/worker/prompt.go`** — inject PLAN.md into buildUser()
```go
// After NOTES.md injection:
if a.PlanPath != "" {
    plan := "(no plan)"
    if data, err := os.ReadFile(a.PlanPath); err == nil && len(data) > 0 {
        plan = ctxdiet.ClampTo(string(data), 1500)
    }
    sb.WriteString("\nPlanner diagnosis — follow this plan:\n" + plan + "\n")
}
```

**7. `cmd/run.go`** — wire planner step between verify fail and strike logic
```go
// After res.Failures check, before st.RecordRun():
if cfg.Planner.Enabled && cfg.Planner.APIKey != "" {
    plan := planner.Diagnose(cfg, res.Failures, dir)
    if plan != "" {
        planPath := filepath.Join(jl.Root, "PLAN.md")
        _ = os.WriteFile(planPath, []byte(plan), 0o644)
        agent.PlanPath = planPath
        fmt.Printf("planner: wrote PLAN.md (%d bytes)\n", len(plan))
    }
}
```

### Total: ~230 lines, 7 files (3 new, 4 modified)

---

## Phase 2: Combined DOD (voice pipeline + power management)

### Why Combine

The 3 separate DODs (server.yaml, babaface-voice.yaml, power.yaml) are
all sitting in babaface/. Combining them tests the planner on a more
complex task — multiple failure modes, different code domains (Python
server + shell scripts + systemd services).

### Combined DOD: `.cage/dods/babaface-full.yaml`

Sections:
1. **Server core** (from server.yaml) — config.py, skill.py, server.py compile, skills load
2. **Voice pipeline** (from babaface-voice.yaml) — STT installed, TTS available, LLM reachable, wire protocol match, wake word configured
3. **Power management** (from power.yaml) — GPU cap script+service, idle watchdog script+service, WOL wake script, WOL NIC enable
4. **Systemd services** — babaface.service exists, power services exist

~30-35 total criteria. More complex than any single DOD we've run.

### Combined PLAN.md

Full context for Qwen: hardware specs, code examples for each script,
exact file paths, the routing architecture, everything it needs to
implement all sections in one run.

---

## Phase 3: Build & Test Sequence

```
1. Billy reviews this plan
2. I write the 3 new Go files + modify 4 existing files
3. I write the combined DOD + PLAN.md
4. Billy reviews the code
5. Build Cobra: cd /home/billy/cobra && go build -o cage ./cmd/
6. Configure babaface/.cage/config.yaml with planner section
7. Reset strikes
8. Billy launches: cage run "implement full babaface per babaface-full.yaml"
9. If Qwen fails → Fable kicks in → writes PLAN.md → Qwen follows
10. Evaluate results together
```

---

## Planner Config in babaface/.cage/config.yaml

```yaml
planner:
  enabled: true
  base_url: "https://api.oneprovider.dev"
  api_key: "sk-b1146d73e32ddec4611264ab618c8b21597a28fd457f1073d78c3ecc2dc3b147"
  model: "claude-fable-5"
  max_tokens: 2000
```

---

## What This Does NOT Change

- Cage verify: still 100% deterministic, no LLM
- Strike logic: still fingerprint-based
- Worker tools: unchanged
- Commit authority: still binary-only
- Existing planning stage (plan.GeneratePlan): stays as-is, separate feature

## Cost

$0.007 per failed attempt. 30 criteria, even 10 failures = $0.07 total.
Prompt caching makes repeat calls even cheaper (90% savings on cached content).

---

## Risk: Nothing

Planner degrades gracefully. If API key wrong, timeout, or any error —
cage runs without it. No crash, no hang. Falls through to existing
strike logic.

---

**Awaiting your review. Say the word and I start writing code.**
