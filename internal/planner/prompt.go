package planner

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are a code diagnostician for a cage enforcement system. A coding agent
failed verify. Read the failures, the relevant code, and the DOD criteria.
Diagnose each failure and write an actionable plan.

Rules:
- One numbered item per failure
- For each: exact file path, exact change needed, expected result
- Priority order: fix blocking failures first (DOD checks) then quality
- Be specific: show the actual code to write, not vague descriptions
- If a failure is ambiguous, state your interpretation
- Keep total response under 2000 tokens`

// BuildPrompt assembles the planner's context: failures, DOD, source code.
// Returns system and user prompt strings ready for the API call.
func BuildPrompt(failures []string, dodText string, codeFiles map[string]string) (string, string) {
	var sb strings.Builder

	// Failure list
	sb.WriteString(fmt.Sprintf("## Verify Failures (%d)\n\n", len(failures)))
	for i, f := range failures {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
	}

	// DOD text (if available)
	if dodText != "" {
		sb.WriteString("\n## DOD Criteria\n\n")
		sb.WriteString(dodText)
		sb.WriteString("\n")
	}

	// Relevant source code
	if len(codeFiles) > 0 {
		sb.WriteString("\n## Relevant Source Code\n\n")
		for name, content := range codeFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n```\n", name))
			sb.WriteString(content)
			sb.WriteString("\n```\n\n")
		}
	}

	return systemPrompt, sb.String()
}
