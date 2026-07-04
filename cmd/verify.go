package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"cobra/internal/cage"
	"cobra/internal/state"
)

var (
	verifyDOD      string
	verifyGate     bool
	verifyWorktree string
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "run the deterministic gate: core checks → DOD → quality officer",
	Long: `verify decides done-or-not with zero LLM involvement. Exit 0 means every
check holds; exit 1 means at least one failure — the full report prints
every failure at once. With --dod FILE only that DOD is evaluated. With
--gate it runs as a push gate. Failures feed the no-progress strike logic.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := projectDir(verifyWorktree)
		opts := cage.Options{
			Dir:     dir,
			DODPath: verifyDOD,
			DODOnly: verifyDOD != "",
			Gate:    verifyGate,
		}
		res, err := cage.Verify(opts)
		if err != nil {
			return err
		}
		fmt.Print(res.Report())

		// Strikes and reports only apply to full project verifies —
		// evaluating an explicit DOD file is a preview, not a run.
		if !opts.DODOnly {
			st, lerr := state.Load(state.StatePath(dir))
			if lerr == nil {
				struck := st.RecordRun(res.Failures)
				if err := st.Save(); err == nil && struck {
					fmt.Printf("STRIKE %d/%d — identical failures to last run (no progress)\n", st.Strikes, state.MaxStrikes)
					if st.Locked {
						fmt.Println("cage is now LOCKED — `cage strikes reset` (human only)")
					}
				}
			}
			_ = state.WriteReport(dir, state.QualityReport{Time: time.Now(), Passed: res.Passed, Failures: res.Failures})
			_ = state.Audit(state.AuditPath(dir), "verify", map[string]any{"passed": res.Passed, "failures": len(res.Failures), "gate": verifyGate})
		}
		if !res.Passed {
			return fmt.Errorf("%d failure(s)", len(res.Failures))
		}
		return nil
	},
}

func init() {
	verifyCmd.Flags().StringVar(&verifyDOD, "dod", "", "evaluate only this DOD file")
	verifyCmd.Flags().BoolVar(&verifyGate, "gate", false, "run as a push gate (pre-receive)")
	verifyCmd.Flags().StringVar(&verifyWorktree, "worktree", "", "project directory to verify (default .)")
	rootCmd.AddCommand(verifyCmd)
}
