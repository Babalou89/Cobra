package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cobra/internal/cage"
	"cobra/internal/config"
	"cobra/internal/critic"
	"cobra/internal/gitx"
	"cobra/internal/planner"
	"cobra/internal/state"
	"cobra/internal/worker"
)

// loopParams is everything the attempt loop needs, so it can be driven from
// tests with a scripted backend instead of only from the cobra command.
type loopParams struct {
	Dir       string
	DODPath   string
	DOD       *cage.DOD
	Agent     *worker.Agent
	Task      string
	Cfg       *config.Config
	State     *state.State
	JailRoot  string
	OnAttempt func(n int) // optional: told the attempt number before each attempt
}

// runAttempts drives worker attempts, each followed by a fresh verify, until
// verify passes (commit) or the run ends in a terminal state (lock).
func runAttempts(p loopParams) error {
	dir, agent, cfg, st := p.Dir, p.Agent, p.Cfg, p.State
	report := ""
	for attempt := 1; ; attempt++ {
		if p.OnAttempt != nil {
			p.OnAttempt(attempt)
		}
		fmt.Printf("attempt %d\n", attempt)
		_ = state.AppendEvent(state.EventsPath(dir), state.Event{Attempt: attempt, Kind: "status", Text: "attempt started: " + firstLineOf(p.Task)})
		_ = state.TouchCooldown(state.CooldownPath(dir))
		directive := p.DOD.Direct(attempt, report)

		// Inject planner summary from PLAN.md if it exists.
		if planSummary := cage.InjectPlanner(p.JailRoot); planSummary != "" {
			directive.FixTarget += planSummary
		}

		if err := agent.Run(directive.Task, directive.FixTarget, report); err != nil {
			fmt.Fprintf(os.Stderr, "worker error: %v\n", err)
			_ = state.AppendEvent(state.EventsPath(dir), state.Event{Attempt: attempt, Kind: "status", Text: "worker error: " + err.Error()})
		}

		res, err := cage.Verify(cage.Options{Dir: dir, DODPath: p.DODPath, DODOnly: false})
		if err != nil {
			return err
		}
		fmt.Print(res.Report())
		passed := res.Passed
		_ = state.AppendEvent(state.EventsPath(dir), state.Event{Attempt: attempt, Kind: "verify", OK: &passed, Text: firstLineOf(res.Report()) + fmt.Sprintf(" (%d failures)", len(res.Failures))})
		_ = state.WriteReport(dir, state.QualityReport{Time: time.Now(), Passed: res.Passed, Failures: res.Failures})

		if res.Passed {
			if cfg.Critic.Enabled {
				runCritic(dir)
			}
			msg := "cage: " + firstLineOf(p.Task)
			if err := gitx.CommitAll(dir, msg); err != nil {
				return fmt.Errorf("verify passed but commit failed: %w", err)
			}
			st.RecordRun(nil)
			_ = st.Save()
			_ = state.Audit(state.AuditPath(dir), "run.pass", map[string]any{"attempts": attempt})
			fmt.Println("PASS — committed")
			return nil
		}

		// Planner step — diagnose failures for the next attempt.
		if cfg.Planner.Enabled && cfg.Planner.APIKey != "" {
			planResult := planner.Diagnose(cfg, res.Failures, dir)
			if planResult != "" {
				planPath := filepath.Join(p.JailRoot, "PLAN.md")
				_ = os.WriteFile(planPath, []byte(planResult), 0o644)
				agent.PlanPath = planPath
				fmt.Printf("planner: wrote PLAN.md (%d bytes)\n", len(planResult))
			}
		}
		struck := st.RecordRun(res.Failures)
		_ = st.Save()
		if struck {
			fmt.Printf("STRIKE %d/%d — no progress since last attempt\n", st.Strikes, state.MaxStrikes)
		}
		_ = state.Audit(state.AuditPath(dir), "run.fail", map[string]any{"attempt": attempt, "failures": len(res.Failures), "struck": struck})
		if st.Locked {
			return fmt.Errorf("LOCKED after %d no-progress attempts — human reset required", state.MaxStrikes)
		}
		report = res.Report()
	}
}

// runCritic runs the post-pass mypy critic on touched .py files.
func runCritic(dir string) {
	changed, err := gitx.ChangedFiles(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "critic: could not list changed files: %v\n", err)
		return
	}
	var pyFiles []string
	for _, f := range changed {
		if strings.HasSuffix(f, ".py") {
			pyFiles = append(pyFiles, f)
		}
	}
	if len(pyFiles) == 0 {
		return
	}
	fmt.Printf("critic: running mypy on %d touched .py file(s)\n", len(pyFiles))
	for round := 1; round <= critic.MaxRounds; round++ {
		r, _, err := critic.BoundedRemediate(pyFiles, round)
		if err != nil {
			fmt.Fprintf(os.Stderr, "critic: bounded remediate failed: %v\n", err)
			break
		}
		if len(r.Errors) == 0 {
			fmt.Println("critic: mypy clean")
			break
		}
		fmt.Printf("critic: round %d — %d error(s)\n", r.Number, len(r.Errors))
		if round == critic.MaxRounds {
			fmt.Fprintf(os.Stderr, "critic: max rounds reached, %d residual error(s)\n", len(r.Errors))
		}
	}
}
