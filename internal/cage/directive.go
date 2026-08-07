package cage

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Directive is what the cage tells Ralph to tell the model.
type Directive struct {
	Task      string // what to build — always from DOD.Task
	FixTarget string // verify failures + relevant code + DOD criteria — empty on turn 1
	Turn      int
}

// fileRe matches file paths in verify failure strings.
var fileRe = regexp.MustCompile(`([a-zA-Z0-9_./-]+\.(py|go|js|ts|sh|yaml|yml|json|md))`)

// Direct builds a directive. Turn 1: task + DOD criteria. Turn 2+: failures + relevant source code + DOD status.
func (d *DOD) Direct(attempt int, verifyReport string) Directive {
	if attempt <= 0 {
		attempt = 1
	}
	dir := Directive{
		Task: d.Task,
		Turn: attempt,
	}

	// Always include the DOD criteria checklist so the model knows what to build.
	dodChecklist := d.CriteriaChecklist(verifyReport)

	if verifyReport != "" {
		dir.FixTarget = buildFixTarget(verifyReport, dodChecklist)
	} else {
		// First attempt — include criteria so the model knows the target.
		dir.FixTarget = dodChecklist
	}
	return dir
}

// CriteriaChecklist renders a human-readable checklist of all DOD criteria
// with pass/fail status based on the verify report. This gives the model
// a clear target to aim for instead of just failure feedback.
func (d *DOD) CriteriaChecklist(verifyReport string) string {
	var sb strings.Builder
	sb.WriteString("DOD criteria:\n")
	for i, crit := range d.Criteria {
		status := "?"
		if verifyReport != "" {
			// Check if this criterion appears in the failures.
			if strings.Contains(verifyReport, fmt.Sprintf("DOD %q:", crit.Name)) {
				status = "FAIL"
			} else {
				status = "PASS"
			}
		}
		sb.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, status, crit.Name))
	}
	return sb.String()
}

// InjectPlanner reads PLAN.md if it exists and returns a summary section
// for injection into the directive. Returns empty string if no plan exists.
func InjectPlanner(jailRoot string) string {
	planPath := jailRoot + "/PLAN.md"
	data, err := os.ReadFile(planPath)
	if err != nil {
		return ""
	}
	content := string(data)
	// Clamp to 1500 chars — model context is precious.
	if len(content) > 1500 {
		content = content[:1500] + fmt.Sprintf("\n... [%d bytes truncated]", len(data))
	}
	return "\nDiagnosis from planner:\n" + content + "\n"
}

// buildFixTarget reads files mentioned in failures and includes their contents.
// The cage does the reading so the model doesn't have to.
func buildFixTarget(report string, dodChecklist string) string {
	var sb strings.Builder

	// Start with the DOD checklist — model always knows what's passing.
	if dodChecklist != "" {
		sb.WriteString(dodChecklist)
		sb.WriteString("\n")
	}

	sb.WriteString("Fix these failures:\n")
	sb.WriteString(report)
	sb.WriteString("\n")

	// Extract unique file paths from the report
	seen := make(map[string]bool)
	matches := fileRe.FindAllString(report, -1)
	for _, m := range matches {
		// Strip trailing line numbers
		if idx := strings.LastIndex(m, ":"); idx > 0 {
			after := m[idx+1:]
			if isNumeric(after) {
				m = m[:idx]
			}
		}
		if seen[m] {
			continue
		}
		seen[m] = true

		// Read the file and include contents
		data, err := os.ReadFile(m)
		if err != nil {
			continue // file doesn't exist yet — that's fine, model needs to create it
		}
		content := string(data)
		if len(content) > 4000 {
			content = content[:4000] + fmt.Sprintf("\n... [%d bytes truncated]", len(data))
		}
		sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n--- end %s ---\n", m, content, m))
	}

	return sb.String()
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ClampContext keeps only the last maxLines of content.
func ClampContext(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	lines = lines[len(lines)-maxLines:]
	return strings.Join(lines, "\n")
}
