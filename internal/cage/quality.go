// The Quality Officer: scrape facts off every touched file (from the
// binary's own git diff), run the 8 checks, apply DOD thresholds, and
// build the change-level facts. Deterministic end to end.
package cage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cobra/internal/gitx"
)

// QualityResult carries everything the officer measured.
type QualityResult struct {
	Findings []Finding
	Facts    []*FileFacts
	Change   ChangeFacts
	Failures []string
}

// QualityOfficer measures the changeset in dir. Touched files come from
// git; thresholds come from the DOD (nil = none).
func QualityOfficer(dir string, disabled map[string]bool, thresholds *Thresholds) *QualityResult {
	res := &QualityResult{}
	if !gitx.IsRepo(dir) {
		return res // nothing to diff against — quality has no jurisdiction
	}
	changed, err := gitx.ChangedFiles(dir)
	if err != nil {
		res.Failures = append(res.Failures, "quality: cannot list changed files: "+err.Error())
		return res
	}

	bySHA := map[string][]string{}
	for _, rel := range changed {
		// The cage's own files are not the deliverable — never grade them:
		// the code-execution scratch file, the worker's memory scratchpad,
		// and everything under .cage/.
		if rel == ".cage_snippet.py" || rel == "NOTES.md" ||
			rel == ".cage" || strings.HasPrefix(rel, ".cage/") {
			continue
		}
		abs := filepath.Join(dir, rel)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			res.Change.FilesDeleted++
			continue
		}
		facts := Scrape(abs)
		facts.Path = rel
		res.Facts = append(res.Facts, facts)
		res.Change.FilesTouched++
		if facts.SHA256 != "" && facts.SizeBytes > 0 {
			bySHA[facts.SHA256] = append(bySHA[facts.SHA256], rel)
		}

		for _, finding := range RunChecks(facts, disabled) {
			res.Findings = append(res.Findings, finding)
			if !finding.Passed {
				res.Failures = append(res.Failures, fmt.Sprintf("quality: %s %s:%d — %s", finding.Code, rel, finding.Line, finding.Detail))
			}
		}
		res.Failures = append(res.Failures, applyThresholds(facts, thresholds)...)
	}

	// dupl — identical-hash files across the changeset.
	for sha, files := range bySHA {
		if len(files) > 1 {
			res.Change.DuplicateFiles = append(res.Change.DuplicateFiles, files)
			if !disabled["dupl"] {
				res.Findings = append(res.Findings, Finding{Code: "dupl", Passed: false, File: files[0], Detail: fmt.Sprintf("identical content (sha %s…): %v", sha[:8], files)})
				res.Failures = append(res.Failures, fmt.Sprintf("quality: dupl — identical files: %v", files))
			}
		}
	}

	res.Change.LinesAdded, res.Change.LinesRemoved, _ = gitx.Numstat(dir)
	res.Change.VersionOld = gitx.VersionFromHistory(dir)
	if v, err := os.ReadFile(filepath.Join(dir, ".ai", "VERSION")); err == nil {
		res.Change.VersionNew = string(v)
	}
	res.Failures = append(res.Failures, applyChangeThresholds(&res.Change, thresholds)...)
	return res
}

func applyThresholds(f *FileFacts, t *Thresholds) []string {
	if t == nil || !isCodeLang(f.Lang) {
		return nil
	}
	var out []string
	fail := func(format string, args ...any) {
		out = append(out, "threshold: "+f.Path+" — "+fmt.Sprintf(format, args...))
	}
	if t.MinCodeLines > 0 && f.CodeLines < t.MinCodeLines {
		fail("%d code lines (need >= %d)", f.CodeLines, t.MinCodeLines)
	}
	if t.MaxFunctionLines > 0 && f.LongestFunc > t.MaxFunctionLines {
		fail("longest function %d lines (max %d)", f.LongestFunc, t.MaxFunctionLines)
	}
	if t.MaxNesting > 0 && f.MaxNesting > t.MaxNesting {
		fail("nesting depth %d (max %d)", f.MaxNesting, t.MaxNesting)
	}
	if t.MinCommentRatio > 0 && f.CommentRatio < t.MinCommentRatio {
		fail("comment ratio %.2f (need >= %.2f)", f.CommentRatio, t.MinCommentRatio)
	}
	if t.NoEmptyBodies && f.EmptyBodies > 0 {
		fail("%d empty bodies (none allowed)", f.EmptyBodies)
	}
	if t.NoTodos && f.TodoCount > 0 {
		fail("%d unfinished-work markers (none allowed)", f.TodoCount)
	}
	return out
}

func applyChangeThresholds(c *ChangeFacts, t *Thresholds) []string {
	if t == nil {
		return nil
	}
	var out []string
	if t.MaxDuplicateFiles > 0 && len(c.DuplicateFiles) > t.MaxDuplicateFiles {
		out = append(out, fmt.Sprintf("threshold: %d duplicate-file groups (max %d)", len(c.DuplicateFiles), t.MaxDuplicateFiles))
	}
	if t.MinLinesAdded > 0 && c.LinesAdded < t.MinLinesAdded {
		out = append(out, fmt.Sprintf("threshold: %d lines added (need >= %d)", c.LinesAdded, t.MinLinesAdded))
	}
	return out
}
