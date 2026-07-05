package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cobra/internal/backend"
	"cobra/internal/config"
	"cobra/internal/evolve"
	"cobra/internal/gitx"
	"cobra/internal/state"
)

var evolveMax int

var evolveCmd = &cobra.Command{
	Use:   "evolve",
	Short: "continual harness: propose skills from trajectories, adopt only what passes the gate",
	Long: `evolve runs one offline harness-refinement pass. The proposer (the
configured backend) reads the trajectory log, the last verify report, and
the strike state, then proposes skills as code plus a test. Each proposal
is staged and its test executed; only a passing test installs the skill.
The proposer's opinion of its own work counts for nothing — the gate
decides, and the binary commits what survives. Verify itself never
involves a model; evolution rides on top of it, not inside it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		be, err := backend.New(cfg.Backend.Type, cfg.Backend.Params)
		if err != nil {
			return err
		}
		if !be.Health() {
			fmt.Printf("warning: backend %s failed its health check — continuing\n", be.Name())
		}

		fmt.Printf("cage evolve — proposer=%s, gate=deterministic\n", be.Name())
		out, err := evolve.Run(dir, be, evolveMax)
		if err != nil {
			return err
		}

		for _, name := range out.Adopted {
			fmt.Printf("  ✓ adopted  %s (test passed)\n", name)
		}
		for _, rej := range out.Rejected {
			fmt.Printf("  ✗ rejected %s\n", rej)
		}
		if len(out.Adopted) == 0 && len(out.Rejected) == 0 {
			fmt.Println("  proposer offered nothing — an empty proposal is a valid outcome")
		}

		_ = state.Audit(state.AuditPath(dir), "evolve", map[string]any{
			"proposer": be.Name(),
			"adopted":  out.Adopted,
			"rejected": out.Rejected,
		})

		if len(out.Adopted) > 0 && gitx.IsRepo(dir) {
			msg := fmt.Sprintf("cage evolve: adopt %d skill(s) — gate-verified", len(out.Adopted))
			if err := gitx.CommitAll(dir, msg); err != nil {
				return fmt.Errorf("skills installed but commit failed: %w", err)
			}
			fmt.Println("committed adopted skills")
		}
		return nil
	},
}

func init() {
	evolveCmd.Flags().IntVar(&evolveMax, "max", 2, "maximum proposals per pass")
	rootCmd.AddCommand(evolveCmd)
}
