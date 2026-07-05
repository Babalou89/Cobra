package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cobra/internal/backend"
	"cobra/internal/cage"
	"cobra/internal/config"
	"cobra/internal/ctxdiet"
	"cobra/internal/gitx"
	"cobra/internal/jail"
	"cobra/internal/skills"
	"cobra/internal/state"
	"cobra/internal/worker"
)

var runDOD string

var runCmd = &cobra.Command{
	Use:   "run \"<task>\"",
	Short: "drive the worker through the jail until verify passes or strikes lock",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := args[0]
		dir := "."

		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		st, err := state.Load(state.StatePath(dir))
		if err != nil {
			return err
		}
		if st.Locked {
			return fmt.Errorf("cage is LOCKED (3 no-progress strikes) — `cage strikes reset` to unlock")
		}
		if err := state.CheckCooldown(state.CooldownPath(dir), time.Duration(cfg.CooldownSeconds)*time.Second); err != nil {
			return err
		}

		dodPath := runDOD
		if dodPath == "" {
			dodPath = cfg.DOD
		}
		if dodPath == "" {
			return fmt.Errorf("no DOD configured — set `dod:` in .cage/config.yaml or pass --dod")
		}
		dod, err := cage.LoadDOD(dodPath)
		if err != nil {
			return err // zero criteria or unreadable = hard fail before any model call
		}
		dodText, _ := os.ReadFile(dodPath)

		be, err := backend.New(cfg.Backend.Type, cfg.Backend.Params)
		if err != nil {
			return err
		}
		if !be.Health() {
			fmt.Fprintf(os.Stderr, "warning: backend %s failed its health check — continuing\n", be.Name())
		}

		jl, err := jail.New(cfg.Jail.Root)
		if err != nil {
			return err
		}
		// Never let the configured budget exceed what the backend can
		// actually hold: cap at ~55% of the model context, reserving the
		// rest for generation, template overhead, and estimator error.
		budgetCap := be.MaxContext() * 55 / 100
		if cfg.Budget.MaxTokens > budgetCap {
			cfg.Budget.MaxTokens = budgetCap
		}
		agent := &worker.Agent{
			Backend: be,
			Tools:   worker.DefaultRegistry(jl, skills.Root(dir)),
			Budget:  ctxdiet.Budget{MaxTokens: cfg.Budget.MaxTokens, PromptCeiling: cfg.Budget.PromptCeiling},
			Log: func(format string, a ...any) {
				fmt.Printf("  "+format+"\n", a...)
			},
			Trace: func(turn int, tool string, ok bool, summary string) {
				_ = state.AppendTrace(state.TracePath(dir), state.TraceStep{
					Task: firstLineOf(task), Turn: turn, Tool: tool, OK: ok, Summary: summary,
				})
			},
		}

		fmt.Printf("cage run — backend=%s dod=%s (%d criteria) jail=%s\n", be.Name(), dodPath, len(dod.Criteria), jl.Root)
		_ = state.Audit(state.AuditPath(dir), "run.start", map[string]any{"task": task, "backend": be.Name()})

		report := ""
		for attempt := 1; ; attempt++ {
			fmt.Printf("attempt %d\n", attempt)
			_ = state.TouchCooldown(state.CooldownPath(dir))
			if err := agent.Run(task, string(dodText), report); err != nil {
				fmt.Fprintf(os.Stderr, "worker error: %v\n", err)
			}

			res, err := cage.Verify(cage.Options{Dir: dir, DODPath: dodPath, DODOnly: false})
			if err != nil {
				return err
			}
			fmt.Print(res.Report())
			_ = state.WriteReport(dir, state.QualityReport{Time: time.Now(), Passed: res.Passed, Failures: res.Failures})

			if res.Passed {
				// The binary — never the worker — holds commit authority.
				msg := "cage: " + firstLineOf(task)
				if err := gitx.CommitAll(dir, msg); err != nil {
					return fmt.Errorf("verify passed but commit failed: %w", err)
				}
				st.RecordRun(nil)
				_ = st.Save()
				_ = state.Audit(state.AuditPath(dir), "run.pass", map[string]any{"attempts": attempt})
				fmt.Println("PASS — committed")
				return nil
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
	},
}

func firstLineOf(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > 72 {
		s = s[:72]
	}
	return s
}

func init() {
	runCmd.Flags().StringVar(&runDOD, "dod", "", "DOD file for this run (default from config)")
	rootCmd.AddCommand(runCmd)
}
