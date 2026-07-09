package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"cobra/internal/cage"
	"cobra/internal/config"
	"cobra/internal/plan"
)

var planDOD string

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "generate a step plan from the DOD and write .cage/plan.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."

		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		if !cfg.PlanningStage.Enabled {
			return fmt.Errorf("planning stage is disabled — set planning_stage.enabled in .cage/config.yaml")
		}

		dodPath := planDOD
		if dodPath == "" {
			dodPath = cfg.DOD
		}
		if dodPath == "" {
			return fmt.Errorf("no DOD configured — set dod: in .cage/config.yaml or pass --dod")
		}
		dod, err := cage.LoadDOD(dodPath)
		if err != nil {
			return err
		}

		p := plan.Plan{
			Steps: []plan.Step{
				{ID: 1, Action: "plan generated", Criterion: "planning stage active"},
			},
		}
		var critNames []string
		for i, c := range dod.Criteria {
			critNames = append(critNames, c.Name)
			p.Steps = append(p.Steps, plan.Step{
				ID:        i + 2,
				Action:    "satisfy criterion",
				Criterion: c.Name,
			})
		}

		planPath := filepath.Join(dir, ".cage", "plan.json")
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(planPath, append(data, '\n'), 0o644); err != nil {
			return err
		}

		gaps := plan.CoverageGaps(critNames, p)
		if len(gaps) > 0 {
			fmt.Printf("plan written to %s (%d steps, %d coverage gaps)\n", planPath, len(p.Steps), len(gaps))
		} else {
			fmt.Printf("plan written to %s (%d steps, full coverage)\n", planPath, len(p.Steps))
		}
		return nil
	},
}

func init() {
	planCmd.Flags().StringVar(&planDOD, "dod", "", "DOD file for this plan (default from config)")
	rootCmd.AddCommand(planCmd)
}
