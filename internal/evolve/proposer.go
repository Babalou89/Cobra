// Package evolve is the continual-harness proposer: it reads what actually
// happened (trajectories, verify reports, strikes — all measured by the
// binary, never self-reported) and asks the backend to propose skills as
// code plus a test. Proposals are adopted ONLY if the deterministic gate
// in internal/skills passes their test. The proposer has an opinion; the
// gate has authority. This package is never imported by internal/cage —
// the verify path stays model-free.
package evolve

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cobra/internal/backend"
	"cobra/internal/skills"
	"cobra/internal/state"
)

// Inputs is everything the proposer is allowed to see.
type Inputs struct {
	Trajectory []state.TraceStep
	Report     string
	Strikes    int
	Existing   []skills.Skill
}

// Outcome records what happened to every proposal, adopted or not.
type Outcome struct {
	Adopted  []string
	Rejected []string // "name: reason"
}

// GatherInputs assembles the proposer's view from the binary's own records.
func GatherInputs(dir string, tailSteps int) (Inputs, error) {
	var in Inputs
	var err error
	in.Trajectory, err = state.ReadTraceTail(state.TracePath(dir), tailSteps)
	if err != nil {
		return in, err
	}
	if data, rerr := os.ReadFile(state.ReportPath(dir)); rerr == nil {
		in.Report = string(data)
	}
	if st, serr := state.Load(state.StatePath(dir)); serr == nil {
		in.Strikes = st.Strikes
	}
	in.Existing, err = skills.List(skills.Root(dir))
	return in, err
}

// Propose asks the backend for up to maxProposals candidate skills.
func Propose(be backend.Backend, in Inputs, maxProposals int) ([]skills.Proposal, error) {
	if maxProposals <= 0 {
		maxProposals = 2
	}
	prompt := buildPrompt(in, maxProposals)
	reply, err := be.Chat([]backend.Msg{{Role: "user", Content: prompt}}, 0.3, 3000)
	if err != nil {
		return nil, fmt.Errorf("proposer backend: %w", err)
	}
	proposals, err := parseProposals(reply)
	if err != nil {
		return nil, err
	}
	if len(proposals) > maxProposals {
		proposals = proposals[:maxProposals]
	}
	return proposals, nil
}

// Run is the full evolution pass: gather → propose → stage → gate →
// install. It returns the outcome; committing adopted skills is the
// caller's job (commit authority stays in cmd/, as everywhere else).
func Run(dir string, be backend.Backend, maxProposals int) (*Outcome, error) {
	in, err := GatherInputs(dir, 40)
	if err != nil {
		return nil, err
	}
	if len(in.Trajectory) == 0 {
		return nil, fmt.Errorf("no trajectory data in %s — run `cage run` first; there is nothing to learn from yet", state.TracePath(dir))
	}

	proposals, err := Propose(be, in, maxProposals)
	if err != nil {
		return nil, err
	}

	out := &Outcome{}
	for _, p := range proposals {
		if reason := adopt(dir, p, be.Name()); reason != "" {
			out.Rejected = append(out.Rejected, p.Name+": "+reason)
		} else {
			out.Adopted = append(out.Adopted, p.Name)
		}
	}
	return out, nil
}

// adopt stages one proposal and runs it through the deterministic gate.
// Empty return = installed; otherwise the rejection reason.
func adopt(dir string, p skills.Proposal, createdBy string) string {
	staged, err := skills.Stage(skills.StagingRoot(dir), p, createdBy)
	if err != nil {
		return err.Error()
	}
	defer os.RemoveAll(staged) // gone unless Install renamed it away
	if err := skills.Gate(staged, skills.DefaultGateTimeout); err != nil {
		return err.Error()
	}
	if err := skills.Install(skills.Root(dir), staged); err != nil {
		return err.Error()
	}
	return ""
}

func buildPrompt(in Inputs, maxProposals int) string {
	var sb strings.Builder
	sb.WriteString("You are the harness evolution proposer for a coding agent system. ")
	sb.WriteString("You propose reusable skills; a deterministic gate runs each skill's test and adopts only what passes. Your claims are not trusted — only the test result counts.\n\n")

	sb.WriteString("## Recent trajectory (tool, outcome, summary)\n")
	for _, s := range in.Trajectory {
		status := "ok"
		if !s.OK {
			status = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("- %s %s: %s\n", s.Tool, status, clip(s.Summary, 140)))
	}
	if in.Report != "" {
		sb.WriteString("\n## Last verify report\n" + clip(in.Report, 1200) + "\n")
	}
	if in.Strikes > 0 {
		sb.WriteString(fmt.Sprintf("\n## Strikes\n%d no-progress strike(s) recorded — the agent is repeating failures.\n", in.Strikes))
	}
	sb.WriteString("\n## Installed skills (do NOT re-propose these)\n" + skills.Describe(in.Existing) + "\n")

	sb.WriteString(fmt.Sprintf(`## Task
Propose 0 to %d new skills that would have made the trajectory above shorter or the failures avoidable. Only propose a skill if the pattern is genuinely reusable — an empty list is a good answer when nothing recurs.

Each skill is bash: run_sh (the capability, reads args "$@" or stdin) and test_sh (a self-contained offline proof that runs "bash run.sh" from the same directory and exits 0 only if the behavior is correct). Tests must be fast (<10s), deterministic, need no network and no files outside their own directory.

Respond with ONLY one JSON object, no fences, no prose:
{"skills":[{"name":"kebab-case-name","description":"one line","usage":"how to invoke","run_sh":"#!/bin/bash\n...","test_sh":"#!/bin/bash\nset -e\n..."}]}
`, maxProposals))
	return sb.String()
}

// parseProposals extracts the first JSON object from the reply and reads
// its skills array.
func parseProposals(reply string) ([]skills.Proposal, error) {
	start := strings.IndexByte(reply, '{')
	end := strings.LastIndexByte(reply, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("proposer reply contains no JSON object: %q", clip(reply, 300))
	}
	var out struct {
		Skills []skills.Proposal `json:"skills"`
	}
	if err := json.Unmarshal([]byte(reply[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("proposer reply is not valid JSON: %w", err)
	}
	return out.Skills, nil
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
