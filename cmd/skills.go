package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cobra/internal/skills"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "list the installed, gate-verified skill library",
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := skills.List(skills.Root("."))
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println("no skills installed — run `cage evolve` after some `cage run` history exists")
			return nil
		}
		fmt.Printf("%d skill(s), all gate-verified:\n", len(list))
		for _, s := range list {
			fmt.Printf("  %-24s %s\n", s.Name, s.Description)
			fmt.Printf("  %-24s usage: %s | by %s at %s\n", "", s.Usage, s.CreatedBy, s.CreatedAt.Format("2006-01-02"))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(skillsCmd)
}
