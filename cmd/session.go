package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cobra/internal/config"
	"cobra/internal/jail"
	"cobra/internal/state"
)

var sessionCmd = &cobra.Command{
	Use:   "session start|end",
	Short: "session lifecycle: jail setup / archive report + reset strikes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		switch args[0] {
		case "start":
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			if _, err := jail.New(cfg.Jail.Root); err != nil {
				return err
			}
			if err := state.StartSession(state.SessionPath(dir), ""); err != nil {
				return err
			}
			_ = state.Audit(state.AuditPath(dir), "session.start", nil)
			fmt.Println("session started — jail ready at", cfg.Jail.Root)
		case "end":
			if err := state.ArchiveReport(dir); err != nil {
				return err
			}
			st, err := state.Load(state.StatePath(dir))
			if err == nil && !st.Locked {
				st.RecordRun(nil) // clean end resets strike bookkeeping; a lock survives
				_ = st.Save()
			}
			if err := state.EndSession(state.SessionPath(dir)); err != nil {
				return err
			}
			_ = state.Audit(state.AuditPath(dir), "session.end", nil)
			fmt.Println("session ended — report archived")
		default:
			return fmt.Errorf("usage: cage session start|end")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sessionCmd)
}
