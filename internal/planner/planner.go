package planner

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"cobra/internal/config"
)

// filePat matches file paths in failure strings like "server/skill.py:70"
// or "backends/vision.py" or "scripts/gpu-power-cap.sh".
var filePat = regexp.MustCompile(`([a-zA-Z0-9_./-]+\.(py|go|js|ts|sh|yaml|yml|json|md|html|css|h|c))`)

// Diagnose calls an external LLM to analyze verify failures and produce
// an actionable plan. Returns empty string if the planner is disabled
// or the API call fails (degrades gracefully — cage runs without it).
//
// The planner reads:
//   - The verify failure list (from cage.Verify)
//   - The DOD criteria (from the DOD file)
//   - Relevant source files (mentioned in failures)
//
// The planner writes:
//   - A structured plan with diagnosis per failure, exact file changes,
//     and priority ordering
func Diagnose(cfg *config.Config, failures []string, dir string) string {
	if !cfg.Planner.Enabled {
		return ""
	}
	if len(failures) == 0 {
		return ""
	}

	// Extract file paths from failure strings and read them.
	paths := ExtractFileNames(failures)
	codeFiles := make(map[string]string)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			// Try relative to dir.
			data, err = os.ReadFile(dir + "/" + p)
		}
		if err == nil {
			// Clamp each file to 4000 chars to stay within context budget.
			content := string(data)
			if len(content) > 4000 {
				content = content[:4000] + fmt.Sprintf("\n... [%d bytes truncated]", len(data)-4000)
			}
			codeFiles[p] = content
		}
	}

	// Read the DOD text if configured.
	dodText := ""
	if cfg.DOD != "" {
		if data, err := os.ReadFile(cfg.DOD); err == nil {
			dodText = string(data)
			// Clamp DOD to 3000 chars.
			if len(dodText) > 3000 {
				dodText = dodText[:3000] + "\n... [truncated]"
			}
		}
	}

	// Build prompt.
	system, user := BuildPrompt(failures, dodText, codeFiles)

	// Call the planner API.
	client := NewClient(cfg.Planner.BaseURL, cfg.Planner.APIKey, cfg.Planner.Model)
	plan, err := client.Chat(system, user, cfg.Planner.MaxTokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "planner: %v\n", err)
		return ""
	}

	return strings.TrimSpace(plan)
}

// ExtractFileNames parses failure strings for file paths.
// Matches patterns like "server/skill.py", "backends/vision.py", etc.
func ExtractFileNames(failures []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, f := range failures {
		matches := filePat.FindAllString(f, -1)
		for _, m := range matches {
			// Strip trailing line numbers like ":70"
			if idx := strings.LastIndex(m, ":"); idx > 0 {
				// Only strip if what follows is purely numeric.
				after := m[idx+1:]
				if isNumeric(after) {
					m = m[:idx]
				}
			}
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
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
