package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cobra/internal/state"
)

var strikesCmd = &cobra.Command{
	Use:   "strikes [reset]",
	Short: "show strike count and lock state; reset is human-only",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		st, err := state.Load(state.StatePath(dir))
		if err != nil {
			return err
		}
		if len(args) == 1 {
			if args[0] != "reset" {
				return fmt.Errorf("usage: cage strikes [reset]")
			}
			st.Reset()
			if err := st.Save(); err != nil {
				return err
			}
			_ = state.Audit(state.AuditPath(dir), "strikes.reset", nil)
			fmt.Println("strikes reset — cage unlocked")
			return nil
		}
		lock := "unlocked"
		if st.Locked {
			lock = "LOCKED (human reset required)"
		}
		fmt.Printf("strikes: %d/%d — %s\n", st.Strikes, state.MaxStrikes, lock)
		if st.LastFingerprint != "" {
			fmt.Printf("last failure fingerprint: %s\n", st.LastFingerprint[:16])
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(strikesCmd)
}
