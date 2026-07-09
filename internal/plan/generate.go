package plan

import (
	"encoding/json"
	"fmt"
	"strings"

	"cobra/internal/backend"
)

// GeneratePlan calls the backend once to generate a plan for the task.
// It falls back to a deterministic plan if the backend returns unparseable JSON.
func GeneratePlan(be backend.Backend, task, dodText string, criteria []string) (Plan, error) {
	prompt := fmt.Sprintf("Generate a plan for the task: %s\n\nDOD:\n%s\n\nRespond with JSON only: {\"steps\":[{\"id\":1,\"action\":\"...\",\"criterion\":\"...\"},...]}", task, dodText)
	reply, err := be.Chat([]backend.Msg{{Role: "user", Content: prompt}}, 0.6, 4096)
	if err != nil {
		return Plan{}, err
	}

	// Try to extract JSON from the reply.
	reply = strings.TrimSpace(reply)
	if idx := strings.Index(reply, "{"); idx >= 0 {
		if last := strings.LastIndex(reply, "}"); last >= idx {
			reply = reply[idx : last+1]
		}
	}

	var p Plan
	if err := json.Unmarshal([]byte(reply), &p); err != nil {
		// Fallback to deterministic plan.
		p = Plan{Steps: []Step{{ID: 1, Action: "plan generated", Criterion: "planning stage active"}}}
		for i, c := range criteria {
			p.Steps = append(p.Steps, Step{
				ID:        i + 2,
				Action:    "satisfy criterion",
				Criterion: c,
			})
		}
	}
	return p, nil
}
