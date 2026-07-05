// Package skills is the on-disk skill library and its deterministic
// adoption gate. A skill is executable code plus a test that proves it.
// Skills enter the library only through the gate — no model, no judgment,
// just an exit code. This package imports no model layer, ever.
//
// Layout on disk:
//
//	.cage/skills/<name>/skill.yaml   metadata
//	.cage/skills/<name>/run.sh       entrypoint (bash)
//	.cage/skills/<name>/test.sh      deterministic proof; exit 0 = valid
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Skill is one installed, gate-approved capability.
type Skill struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Usage       string    `yaml:"usage"`
	CreatedBy   string    `yaml:"created_by"` // backend name that proposed it
	CreatedAt   time.Time `yaml:"created_at"`
	Dir         string    `yaml:"-"`
}

// Proposal is a candidate skill from the proposer, not yet trusted.
type Proposal struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Usage       string `json:"usage"`
	RunSh       string `json:"run_sh"`
	TestSh      string `json:"test_sh"`
}

// Root returns the skill library location for a project dir.
func Root(projectDir string) string {
	return filepath.Join(projectDir, ".cage", "skills")
}

// StagingRoot returns where candidates are staged before the gate.
func StagingRoot(projectDir string) string {
	return filepath.Join(projectDir, ".cage", "staging")
}

// List reads every installed skill. A missing library is an empty one.
func List(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, "skill.yaml"))
		if err != nil {
			continue // not a skill dir
		}
		var s Skill
		if yaml.Unmarshal(data, &s) != nil || s.Name == "" {
			continue
		}
		s.Dir = dir
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Describe renders the library for a tool description: name + usage only,
// context-diet friendly.
func Describe(list []Skill) string {
	if len(list) == 0 {
		return "(no skills installed yet)"
	}
	var sb strings.Builder
	for _, s := range list {
		sb.WriteString(fmt.Sprintf("  %s — %s (usage: %s)\n", s.Name, s.Description, s.Usage))
	}
	return sb.String()
}

var namePat = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,47}$`)

const maxScriptBytes = 16 * 1024

// Validate rejects malformed proposals before anything touches disk.
func Validate(p Proposal) error {
	if !namePat.MatchString(p.Name) {
		return fmt.Errorf("skill name %q invalid (want ^[a-z][a-z0-9_-]{1,47}$)", p.Name)
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("skill %s: empty description", p.Name)
	}
	if strings.TrimSpace(p.RunSh) == "" || strings.TrimSpace(p.TestSh) == "" {
		return fmt.Errorf("skill %s: run_sh and test_sh must both be non-empty — a skill without a test cannot be gated", p.Name)
	}
	if len(p.RunSh) > maxScriptBytes || len(p.TestSh) > maxScriptBytes {
		return fmt.Errorf("skill %s: script exceeds %d bytes", p.Name, maxScriptBytes)
	}
	for _, banned := range []string{"sudo", "rm -rf /", "mkfs", "shutdown", "reboot"} {
		if strings.Contains(p.RunSh, banned) || strings.Contains(p.TestSh, banned) {
			return fmt.Errorf("skill %s: contains banned pattern %q", p.Name, banned)
		}
	}
	return nil
}

// Stage writes a candidate into its own staging directory for the gate.
func Stage(stagingRoot string, p Proposal, createdBy string) (string, error) {
	if err := Validate(p); err != nil {
		return "", err
	}
	dir := filepath.Join(stagingRoot, p.Name)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	meta := Skill{
		Name:        p.Name,
		Description: p.Description,
		Usage:       p.Usage,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}
	metaData, err := yaml.Marshal(meta)
	if err != nil {
		return "", err
	}
	writes := map[string][]byte{
		"skill.yaml": metaData,
		"run.sh":     []byte(p.RunSh),
		"test.sh":    []byte(p.TestSh),
	}
	for name, data := range writes {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, mode); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// Install moves a gate-approved staged skill into the library. Existing
// skills are never overwritten — a replacement must be a new name or a
// human decision.
func Install(root, stagedDir string) error {
	name := filepath.Base(stagedDir)
	dst := filepath.Join(root, name)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("skill %q already installed — refusing to overwrite", name)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return os.Rename(stagedDir, dst)
}
