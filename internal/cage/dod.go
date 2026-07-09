// DOD loading, validation, and deterministic evaluation. A DOD with zero
// criteria is a HARD FAIL at load time — an empty contract gates nothing.
package cage

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Check is one deterministic assertion inside a criterion.
type Check struct {
	Type       string `yaml:"type"` // exists | command | grep | lines
	File       string `yaml:"file"`
	Run        string `yaml:"run"`
	ExpectExit *int   `yaml:"expect_exit"`
	Contains   string `yaml:"contains"`
	Regex      string `yaml:"regex"`
	Min        int    `yaml:"min"`
	Max        int    `yaml:"max"`
}

// Criterion is a named group of checks; all must pass.
type Criterion struct {
	Name   string  `yaml:"name"`
	Verify []Check `yaml:"verify"`
}

// PlugTool names an external checker (ruff, pytest, mypy, bandit, …) the
// DOD wants shelled out to. Exit 0 = pass.
type PlugTool struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// Thresholds are project-specific floors/ceilings on the scraped facts.
type Thresholds struct {
	MinCodeLines      int     `yaml:"min_code_lines"`
	MaxFunctionLines  int     `yaml:"max_function_lines"`
	MaxNesting        int     `yaml:"max_nesting"`
	MinCommentRatio   float64 `yaml:"min_comment_ratio"`
	NoEmptyBodies     bool    `yaml:"no_empty_bodies"`
	NoTodos           bool    `yaml:"no_todos"`
	MaxDuplicateFiles int     `yaml:"max_duplicate_files"`
	MinLinesAdded     int     `yaml:"min_lines_added"`
}

// DOD is the whole definition-of-done contract.
type DOD struct {
	Task        string      `yaml:"task"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Criteria    []Criterion `yaml:"criteria"`
	Quality     *Thresholds `yaml:"quality"`
	Tools       []PlugTool  `yaml:"tools"`
}

// LoadDOD reads and validates a DOD file. Zero criteria is a hard fail.
func LoadDOD(path string) (*DOD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read DOD: %w", err)
	}
	var d DOD
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse DOD %s: %w", path, err)
	}
	if len(d.Criteria) == 0 {
		return nil, fmt.Errorf("DOD %s has zero criteria — refusing to gate nothing", path)
	}
	for i, c := range d.Criteria {
		if len(c.Verify) == 0 {
			return nil, fmt.Errorf("DOD %s criterion %d (%q) has no verify checks", path, i+1, c.Name)
		}
	}
	return &d, nil
}

// CriteriaNames returns the names of all criteria.
func (d *DOD) CriteriaNames() []string {
	names := make([]string, len(d.Criteria))
	for i, c := range d.Criteria {
		names[i] = c.Name
	}
	return names
}

// Evaluate runs every criterion in dir and returns one failure string per
// failed check. Empty result = all criteria hold.
func (d *DOD) Evaluate(dir string) []string {
	var failures []string
	for _, crit := range d.Criteria {
		for _, chk := range crit.Verify {
			if msg := evalCheck(dir, chk); msg != "" {
				failures = append(failures, fmt.Sprintf("DOD %q: %s", crit.Name, msg))
			}
		}
	}
	return failures
}

func evalCheck(dir string, chk Check) string {
	switch chk.Type {
	case "exists":
		if _, err := os.Stat(join(dir, chk.File)); err != nil {
			return fmt.Sprintf("file %s does not exist", chk.File)
		}
	case "command":
		want := 0
		if chk.ExpectExit != nil {
			want = *chk.ExpectExit
		}
		cmd := exec.Command("bash", "-c", chk.Run)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		got := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				got = ee.ExitCode()
			} else {
				return fmt.Sprintf("command %q failed to start: %v", chk.Run, err)
			}
		}
		if got != want {
			return fmt.Sprintf("command %q exited %d (want %d): %s", chk.Run, got, want, lastLines(string(out), 3))
		}
	case "grep":
		data, err := os.ReadFile(join(dir, chk.File))
		if err != nil {
			return fmt.Sprintf("grep target %s unreadable: %v", chk.File, err)
		}
		if chk.Contains != "" && !strings.Contains(string(data), chk.Contains) {
			return fmt.Sprintf("%s does not contain %q", chk.File, chk.Contains)
		}
		if chk.Regex != "" {
			re, err := regexp.Compile(chk.Regex)
			if err != nil {
				return fmt.Sprintf("bad regex %q: %v", chk.Regex, err)
			}
			if !re.Match(data) {
				return fmt.Sprintf("%s does not match /%s/", chk.File, chk.Regex)
			}
		}
	case "lines":
		data, err := os.ReadFile(join(dir, chk.File))
		if err != nil {
			return fmt.Sprintf("lines target %s unreadable: %v", chk.File, err)
		}
		n := countLines(string(data))
		if chk.Min > 0 && n < chk.Min {
			return fmt.Sprintf("%s has %d lines (need >= %d)", chk.File, n, chk.Min)
		}
		if chk.Max > 0 && n > chk.Max {
			return fmt.Sprintf("%s has %d lines (need <= %d)", chk.File, n, chk.Max)
		}
	default:
		return fmt.Sprintf("unknown check type %q", chk.Type)
	}
	return ""
}

func join(dir, file string) string {
	if strings.HasPrefix(file, "/") {
		return file
	}
	return dir + "/" + file
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
