// Package cmd is the `cage` command surface.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cage",
	Short: "deterministic enforcement cage — the agent works, the cage decides",
	Long: `cage is a single-binary enforcement cage for AI coding agents.

The worker (an external model over HTTP) executes tools inside a jailed
workspace. The cage reads a DOD, scrapes quality off every touched file,
and decides done-or-not with zero LLM in the decision. Only the cage
commits, and only after verify passes.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI and maps errors to exit code 1.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "cage:", err)
		os.Exit(1)
	}
}

// projectDir resolves the working directory flag shared by subcommands.
func projectDir(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}
