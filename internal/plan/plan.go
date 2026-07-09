package plan

// Step is a single planned action with a deterministic criterion.
type Step struct {
	ID        int    `json:"id"`
	Action    string `json:"action"`
	Criterion string `json:"criterion"`
}

// Plan is an ordered list of steps.
type Plan struct {
	Steps []Step `json:"steps"`
}

// CoverageGaps returns criteria that are not referenced by any step's Criterion.
func CoverageGaps(criteria []string, plan Plan) []string {
	covered := make(map[string]bool, len(plan.Steps))
	for _, s := range plan.Steps {
		covered[s.Criterion] = true
	}
	var gaps []string
	for _, c := range criteria {
		if !covered[c] {
			gaps = append(gaps, c)
		}
	}
	return gaps
}
