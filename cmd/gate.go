package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const preReceiveHook = `#!/bin/sh
# installed by cage — defense in depth; do not edit
exec cage verify --gate
`

var gateCmd = &cobra.Command{
	Use:   "gate install|uninstall <bare-repo>",
	Short: "install or remove the pre-receive hook in a bare repository",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		action, repo := args[0], args[1]
		hooksDir := filepath.Join(repo, "hooks")
		hookPath := filepath.Join(hooksDir, "pre-receive")
		switch action {
		case "install":
			if _, err := os.Stat(repo); err != nil {
				return fmt.Errorf("repository %s not found", repo)
			}
			if err := os.MkdirAll(hooksDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(hookPath, []byte(preReceiveHook), 0o755); err != nil {
				return err
			}
			fmt.Println("installed pre-receive gate:", hookPath)
		case "uninstall":
			if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			fmt.Println("removed pre-receive gate:", hookPath)
		default:
			return fmt.Errorf("usage: cage gate install|uninstall <bare-repo>")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gateCmd)
}
