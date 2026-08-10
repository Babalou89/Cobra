package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cobra/assets"
	"cobra/internal/backend"
	"cobra/internal/cage"
	"cobra/internal/config"
	"cobra/internal/critic"
	"cobra/internal/ctxdiet"
	"cobra/internal/gitx"
	"cobra/internal/jail"
	"cobra/internal/plan"
	"cobra/internal/planner"
	"cobra/internal/skills"
	"cobra/internal/state"
	"cobra/internal/worker"
)

var runDOD string
var runWatch bool
var runLog string

// startLogCapture redirects os.Stdout to both the terminal and a log file.
// Returns a cleanup function that must be deferred.
func startLogCapture(logPath string) func() {
	if logPath == "" {
		return func() {}
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: could not create %s: %v\n", logPath, err)
		return func() {}
	}

	// Save the real stdout fd so we can write to terminal + file.
	origStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		logFile.Close()
		fmt.Fprintf(os.Stderr, "log: pipe failed: %v\n", err)
		return func() {}
	}
	os.Stdout = w

	// Background goroutine: read from pipe, write to terminal + log file.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				_, _ = origStdout.Write(buf[:n])
				_, _ = logFile.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	return func() {
		w.Close()   // signals the goroutine to exit
		r.Close()   // cleanup read end
		logFile.Close()
		os.Stdout = origStdout
	}
}

var runCmd = &cobra.Command{
	Use:   `run "<task>"`,
	Short: "drive the worker through the jail until verify passes or strikes lock",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := args[0]
		dir := "."

		// Start session log capture — everything from here on goes to file + terminal.
		logPath := runLog
		if logPath == "" {
			logPath = filepath.Join(dir, ".cage", "session.log")
		}
		cleanup := startLogCapture(logPath)
		defer cleanup()

		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		ensureProjectMeta(dir)
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

		if cfg.PlanningStage.Enabled {
			fmt.Println("planning stage enabled — generating plan")
			p, err := plan.GeneratePlan(be, task, string(dodText), dod.CriteriaNames())
			if err != nil {
				return fmt.Errorf("planning stage failed: %w", err)
			}
			planPath := filepath.Join(dir, ".cage", "plan.json")
			data, err := json.MarshalIndent(p, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(planPath, append(data, '\n'), 0o644); err != nil {
				return err
			}
			gaps := plan.CoverageGaps(dod.CriteriaNames(), p)
			if len(gaps) > 0 {
				fmt.Printf("plan written to %s (%d steps, %d coverage gaps)\n", planPath, len(p.Steps), len(gaps))
			} else {
				fmt.Printf("plan written to %s (%d steps, full coverage)\n", planPath, len(p.Steps))
			}
			_ = state.Audit(state.AuditPath(dir), "run.plan", map[string]any{"steps": len(p.Steps), "gaps": len(gaps)})
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

		// Choose tool registry based on mode. Ralph mode gets a minimal
		// 3-tool set (file_read, file_write, code_execute) to prevent
		// the model from exploring instead of coding. Instruct mode gets
		// the tightest set (file_write, code_execute only) for step-following
		// models that get distracted by any extra tool.
		isRalph := cfg.Worker.Mode != "conversational"
		isInstruct := cfg.DodFormat == "instruct"
		var tools *worker.Registry
		switch {
		case isInstruct:
			tools = worker.InstructRegistry(jl)
		case isRalph:
			tools = worker.RalphRegistry(jl)
		default:
			tools = worker.DefaultRegistry(jl, skills.Root(dir))
		}

		currentAttempt := 1
		agent := &worker.Agent{
			Backend:        be,
			Tools:          tools,
			Budget:         ctxdiet.Budget{MaxTokens: cfg.Budget.MaxTokens, PromptCeiling: cfg.Budget.PromptCeiling},
			MaxTurns:       cfg.Worker.MaxTurns,
			AttemptTimeout: time.Duration(cfg.Worker.AttemptSeconds) * time.Second,
			MaxGenTokens:   cfg.Worker.MaxGenTokens,
			Temperature:    cfg.Worker.Temperature,
			Ralph:          isRalph,
			Instruct:       isInstruct,
			NotesPath:      filepath.Join(jl.Root, "NOTES.md"),
			BrainPath:      filepath.Join(dir, ".ai", "brain.md"),
			Log: func(format string, a ...any) {
				fmt.Printf("  "+format+"\n", a...)
			},
			Trace: func(turn int, tool string, ok bool, summary string) {
				_ = state.AppendTrace(state.TracePath(dir), state.TraceStep{
					Task: firstLineOf(task), Turn: turn, Tool: tool, OK: ok, Summary: summary,
				})
			},
			Observe: func(kind string, fields map[string]any) {
				e := state.Event{Attempt: currentAttempt, Kind: kind}
				if v, ok := fields["turn"].(int); ok {
					e.Turn = v
				}
				if v, ok := fields["tool"].(string); ok {
					e.Tool = v
				}
				if v, ok := fields["text"].(string); ok {
					e.Text = v
				}
				if v, ok := fields["ok"].(bool); ok {
					e.OK = &v
				}
				if v, ok := fields["ctx_tokens"].(int); ok {
					e.CtxTokens = v
				}
				if v, ok := fields["ctx_budget"].(int); ok {
					e.CtxBudget = v
				}
				_ = state.AppendEvent(state.EventsPath(dir), e)
			},
		}

		fmt.Printf("cage run — backend=%s dod=%s (%d criteria) jail=%s\n", be.Name(), dodPath, len(dod.Criteria), jl.Root)
		fmt.Printf("session log: %s\n", logPath)
		_ = state.Audit(state.AuditPath(dir), "run.start", map[string]any{"task": task, "backend": be.Name()})

		// Launch the live dashboard when --watch is set (default true).
		if runWatch {
			dashURL, shutdownDash := startWatchServer(dir, watchPort, true)
			defer shutdownDash()
			fmt.Printf("dashboard: %s\n", dashURL)
		}

		report := ""
		for attempt := 1; ; attempt++ {
			currentAttempt = attempt
			fmt.Printf("attempt %d\n", attempt)
			_ = state.AppendEvent(state.EventsPath(dir), state.Event{Attempt: attempt, Kind: "status", Text: "attempt started: " + firstLineOf(task)})
			_ = state.TouchCooldown(state.CooldownPath(dir))
			directive := dod.Direct(attempt, report)

			// Inject planner summary from PLAN.md if it exists.
			if planSummary := cage.InjectPlanner(jl.Root); planSummary != "" {
				directive.FixTarget += planSummary
			}

			if err := agent.Run(directive.Task, directive.FixTarget, report); err != nil {
				fmt.Fprintf(os.Stderr, "worker error: %v\n", err)
				_ = state.AppendEvent(state.EventsPath(dir), state.Event{Attempt: attempt, Kind: "status", Text: "worker error: " + err.Error()})
			}

			res, err := cage.Verify(cage.Options{Dir: dir, DODPath: dodPath, DODOnly: false})
			if err != nil {
				return err
			}
			fmt.Print(res.Report())
			passed := res.Passed
			_ = state.AppendEvent(state.EventsPath(dir), state.Event{Attempt: attempt, Kind: "verify", OK: &passed, Text: firstLineOf(res.Report()) + fmt.Sprintf(" (%d failures)", len(res.Failures))})
			_ = state.WriteReport(dir, state.QualityReport{Time: time.Now(), Passed: res.Passed, Failures: res.Failures})

			if res.Passed {
				// Post-pass critic: run mypy on touched .py files when enabled.
				if cfg.Critic.Enabled {
					changed, err := gitx.ChangedFiles(dir)
					if err != nil {
						fmt.Fprintf(os.Stderr, "critic: could not list changed files: %v\n", err)
					} else {
						var pyFiles []string
						for _, f := range changed {
							if strings.HasSuffix(f, ".py") {
								pyFiles = append(pyFiles, f)
							}
						}
						if len(pyFiles) > 0 {
							fmt.Printf("critic: running mypy on %d touched .py file(s)\n", len(pyFiles))
							for round := 1; round <= critic.MaxRounds; round++ {
								r, allowlist, err := critic.BoundedRemediate(pyFiles, round)
								if err != nil {
									fmt.Fprintf(os.Stderr, "critic: bounded remediate failed: %v\n", err)
									break
								}
								if len(r.Errors) == 0 {
									fmt.Println("critic: mypy clean")
									break
								}
								fmt.Printf("critic: round %d — %d error(s)\n", r.Number, len(r.Errors))
								_ = allowlist
								if round == critic.MaxRounds {
									fmt.Fprintf(os.Stderr, "critic: max rounds reached, %d residual error(s)\n", len(r.Errors))
								}
							}
						}
					}
				}

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

			// Planner step — diagnose failures for the next attempt.
			if cfg.Planner.Enabled && cfg.Planner.APIKey != "" {
				planResult := planner.Diagnose(cfg, res.Failures, dir)
				if planResult != "" {
					planPath := filepath.Join(jl.Root, "PLAN.md")
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
	},
}

// ensureProjectMeta scaffolds .ai/brain.md and .ai/VERSION when a project
// was never `cage init`-ed. Existing files are never touched.
func ensureProjectMeta(dir string) {
	aiDir := filepath.Join(dir, ".ai")
	_ = os.MkdirAll(aiDir, 0o755)
	brain := filepath.Join(aiDir, "brain.md")
	if _, err := os.Stat(brain); os.IsNotExist(err) {
		_ = os.WriteFile(brain, assets.BrainTemplate, 0o644)
	}
	version := filepath.Join(aiDir, "VERSION")
	if _, err := os.Stat(version); os.IsNotExist(err) {
		_ = os.WriteFile(version, []byte("0.1.0\n"), 0o644)
	}
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
	runCmd.Flags().BoolVar(&runWatch, "watch", true, "auto-launch live dashboard in browser")
	runCmd.Flags().StringVar(&runLog, "log", "", "session log path (default: .cage/session.log)")
	rootCmd.AddCommand(runCmd)
}
