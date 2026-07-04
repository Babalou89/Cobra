package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"cobra/internal/cage"
)

var peekCmd = &cobra.Command{
	Use:   "peek <file>",
	Short: "run the 8 quality checks on one file — preview only, no strike",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		facts := cage.Scrape(path)
		if !facts.Exists {
			return fmt.Errorf("cannot read %s", path)
		}
		findings := cage.RunChecks(facts, nil)

		width := 62
		bar := strings.Repeat("─", width)
		fmt.Printf("┌%s\n", bar)
		fmt.Printf("│ cage peek: %s  (%s, %d lines, %d code)\n", path, langLabel(facts.Lang), facts.Lines, facts.CodeLines)
		fmt.Printf("├%s\n", bar)
		failed := 0
		for _, f := range findings {
			status := "PASS"
			if !f.Passed {
				status = "FAIL"
				failed++
			}
			loc := ""
			if f.Line > 0 {
				loc = fmt.Sprintf("line %d  ", f.Line)
			}
			fmt.Printf("│ %-4s  %s  %s%s\n", f.Code, status, loc, f.Detail)
		}
		fmt.Printf("├%s\n", bar)
		fmt.Printf("│ %d/%d checks failed — preview only, nothing recorded\n", failed, len(findings))
		fmt.Printf("└%s\n", bar)
		return nil
	},
}

func langLabel(lang string) string {
	if lang == "" {
		return "unknown language — lenient checks"
	}
	return lang
}

func init() {
	rootCmd.AddCommand(peekCmd)
}
