// Package critic provides deterministic, toggleable post-run critics.
// The mypy critic runs mypy on given files, parses errors, generates a
// remediation DOD, and can drive a bounded remediation loop.
package critic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// MypyError is one structured error line from mypy output.
type MypyError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// MypyResult holds parsed output and the raw stdout/stderr.
type MypyResult struct {
	Errors []MypyError
	Raw    string
}

// RunMypy executes mypy on the supplied files and returns parsed errors.
// It returns an error only if mypy itself cannot be started.
func RunMypy(files []string) (*MypyResult, error) {
	if len(files) == 0 {
		return &MypyResult{}, nil
	}
	args := append([]string{"--show-column-numbers", "--no-error-summary"}, files...)
	cmd := exec.Command("mypy", args...)
	out, _ := cmd.CombinedOutput()
	return ParseErrors(string(out)), nil
}

// ParseErrors converts mypy output into structured errors.
// It recognises lines of the form:  file:line:col: error: message
func ParseErrors(output string) *MypyResult {
	var errors []MypyError
	re := regexp.MustCompile(`^(.+):(\d+):\d+:\s*(error|warning):\s*(.+)$`)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var ln int
		fmt.Sscanf(m[2], "%d", &ln)
		errors = append(errors, MypyError{
			File:    m[1],
			Line:    ln,
			Message: m[4],
		})
	}
	return &MypyResult{Errors: errors, Raw: output}
}

// Remediate generates a scoped DOD that, when satisfied, means the listed
// mypy errors are fixed.  It also returns the allowlist of files that may be
// touched and a residual report string.
func Remediate(result *MypyResult, originalDODPath string) (*DODSpec, []string, string) {
	allowlist := map[string]bool{}
	for _, e := range result.Errors {
		allowlist[e.File] = true
	}
	files := make([]string, 0, len(allowlist))
	for f := range allowlist {
		files = append(files, f)
	}

	var criteria []CriterionSpec
	for _, e := range result.Errors {
		criteria = append(criteria, CriterionSpec{
			Name: fmt.Sprintf("mypy clean %s:%d", e.File, e.Line),
			Verify: []CheckSpec{
				{Type: "command", Run: fmt.Sprintf("mypy %s", e.File), ExpectExit: ptrInt(0)},
				{Type: "grep", File: e.File, Regex: "# type: ignore"},
			},
		})
	}

	// If there are no errors, generate a single pass-through criterion.
	if len(criteria) == 0 {
		criteria = append(criteria, CriterionSpec{
			Name: "mypy clean",
			Verify: []CheckSpec{
				{Type: "command", Run: "mypy --version", ExpectExit: ptrInt(0)},
			},
		})
	}

	dod := &DODSpec{
		Task:        "mypy-fix: resolve all mypy errors in flagged files",
		Name:        "mypy-fix",
		Description: "Auto-generated remediation DOD from mypy critic. Writes limited to flagged files; no new '# type: ignore'.",
		Criteria:    criteria,
	}

	report := result.Raw
	if report == "" {
		report = "mypy: no errors found"
	}
	return dod, files, report
}

// GenerateDOD serialises a remediation DOD to a YAML file.
func GenerateDOD(dod *DODSpec, path string) error {
	data, err := yaml.Marshal(dod)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// DODSpec is a lightweight YAML-serialisable DOD used for remediation.
type DODSpec struct {
	Task        string          `yaml:"task"`
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Criteria    []CriterionSpec `yaml:"criteria"`
}

// CriterionSpec mirrors cage.Criterion for YAML generation.
type CriterionSpec struct {
	Name   string      `yaml:"name"`
	Verify []CheckSpec `yaml:"verify"`
}

// CheckSpec mirrors cage.Check for YAML generation.
type CheckSpec struct {
	Type       string `yaml:"type"`
	File       string `yaml:"file,omitempty"`
	Run        string `yaml:"run,omitempty"`
	ExpectExit *int   `yaml:"expect_exit,omitempty"`
	Regex      string `yaml:"regex,omitempty"`
}

func ptrInt(n int) *int { return &n }

// MaxRounds is the upper bound for remediation rounds so the critic cannot
// loop forever.
const MaxRounds = 5

// Round tracks the current remediation round and residual state.
type Round struct {
	Number int
	Errors []MypyError
	Report string
}

// BoundedRemediate runs mypy, generates a DOD, and returns a bounded loop
// descriptor together with the residual report.  It does not itself execute
// the loop — that is the caller's responsibility — but it validates the
// allowlist and forbids new "# type: ignore" comments.
func BoundedRemediate(files []string, round int) (*Round, []string, error) {
	if round > MaxRounds {
		return nil, nil, fmt.Errorf("max rounds (%d) exceeded", MaxRounds)
	}
	res, err := RunMypy(files)
	if err != nil {
		return nil, nil, err
	}
	_, allowlist, report := Remediate(res, "")
	return &Round{Number: round, Errors: res.Errors, Report: report}, allowlist, nil
}

// CheckAllowlist returns an error if any path is outside the allowed set.
func CheckAllowlist(paths, allowlist []string) error {
	allowed := make(map[string]bool, len(allowlist))
	for _, p := range allowlist {
		ap, _ := filepath.Abs(p)
		allowed[ap] = true
	}
	for _, p := range paths {
		ap, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if !allowed[ap] {
			return fmt.Errorf("file %s is not in the allowlist", p)
		}
	}
	return nil
}

// HasNewTypeIgnore scans content for "# type: ignore" comments.
func HasNewTypeIgnore(content string) bool {
	return strings.Contains(content, "# type: ignore")
}
