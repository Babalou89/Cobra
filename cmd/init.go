package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"cobra/assets"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "scaffold .cage/ and .ai/ in the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		cageDir := filepath.Join(dir, ".cage")
		if _, err := os.Stat(cageDir); err == nil {
			return fmt.Errorf("already initialized: %s exists", cageDir)
		}
		for _, d := range []string{
			filepath.Join(cageDir, "dods"),
			filepath.Join(cageDir, "reports"),
			filepath.Join(dir, ".ai"),
		} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
		}
		writes := []struct {
			path string
			data []byte
		}{
			{filepath.Join(cageDir, "config.yaml"), assets.ConfigDefault},
			{filepath.Join(cageDir, "dods", "example.yaml"), assets.DODExample},
			{filepath.Join(cageDir, "state.json"), []byte("{}\n")},
		}
		for _, w := range writes {
			if err := os.WriteFile(w.path, w.data, 0o644); err != nil {
				return err
			}
		}
		// .ai/ files are only created when absent — never clobber an
		// existing brain or version.
		brain := filepath.Join(dir, ".ai", "brain.md")
		if _, err := os.Stat(brain); os.IsNotExist(err) {
			if err := os.WriteFile(brain, assets.BrainTemplate, 0o644); err != nil {
				return err
			}
		}
		version := filepath.Join(dir, ".ai", "VERSION")
		if _, err := os.Stat(version); os.IsNotExist(err) {
			if err := os.WriteFile(version, []byte("0.1.0\n"), 0o644); err != nil {
				return err
			}
		}
		fmt.Println("initialized: .cage/ (config, state, dods/, reports/) and .ai/ (brain.md, VERSION)")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
