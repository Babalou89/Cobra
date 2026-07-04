// Core checks: lock state, config validity, brain.md, version bump.
// brain.md is a file the cage inspects — it is never injected into model
// context; that is why the checks here are existence/length only.
package cage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cobra/internal/config"
	"cobra/internal/gitx"
	"cobra/internal/state"
)

// CoreChecks runs the always-on project health checks and returns one
// failure string per violation.
func CoreChecks(dir string, cfg *config.Config) []string {
	var failures []string

	// Cage lock: three strikes means human reset only.
	st, err := state.Load(state.StatePath(dir))
	if err == nil && st.Locked {
		failures = append(failures, "cage is LOCKED (3 no-progress strikes) — run `cage strikes reset` to unlock")
	}

	// brain.md: exists and is substantial. Inspected, never injected.
	brain := filepath.Join(dir, ".ai", "brain.md")
	if data, err := os.ReadFile(brain); err != nil {
		failures = append(failures, "core: .ai/brain.md is missing")
	} else if countLines(string(data)) < 10 {
		failures = append(failures, "core: .ai/brain.md has fewer than 10 lines")
	}

	// VERSION: exists, and if the tree changed, it must differ from the
	// last committed value — the old value comes from git history, so the
	// agent cannot fake a bump.
	versionPath := filepath.Join(dir, ".ai", "VERSION")
	current, err := os.ReadFile(versionPath)
	if err != nil {
		failures = append(failures, "core: .ai/VERSION is missing")
	} else if gitx.IsRepo(dir) && gitx.HasHead(dir) {
		old := gitx.VersionFromHistory(dir)
		changed, _ := gitx.ChangedFiles(dir)
		if len(meaningful(changed)) > 0 && old != "" && strings.TrimSpace(string(current)) == old {
			failures = append(failures, fmt.Sprintf("core: tree changed but .ai/VERSION still %s — bump it", old))
		}
	}

	return failures
}

// meaningful filters churn that should not force a version bump.
func meaningful(files []string) []string {
	var out []string
	for _, f := range files {
		if strings.HasPrefix(f, ".cage/") || f == ".ai/VERSION" || f == ".ai/brain.md" {
			continue
		}
		out = append(out, f)
	}
	return out
}
