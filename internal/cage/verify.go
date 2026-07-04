// The verdict. Order: core checks → DOD criteria → quality officer →
// pluggable tools. Any failure → no commit; the caller runs the
// no-progress strike logic and prints the full report — every failure,
// every file, at once. All pass → the binary (and only the binary) may
// commit. Never partial. No model is consulted anywhere in this path.
package cage

import (
	"fmt"
	"strings"

	"cobra/internal/config"
)

// Options selects what one verify run covers.
type Options struct {
	Dir     string // project root (default ".")
	DODPath string // explicit DOD file; overrides config
	DODOnly bool   // evaluate only the DOD (used by `cage verify --dod`)
	Gate    bool   // running as a push gate (pre-receive)
}

// Result is the full verdict of one run.
type Result struct {
	Passed   bool
	Failures []string
	Quality  *QualityResult
}

// Verify runs the deterministic gate and returns the verdict. It never
// commits; commit authority stays with the caller in cmd/.
func Verify(opts Options) (*Result, error) {
	dir := opts.Dir
	if dir == "" {
		dir = "."
	}
	res := &Result{}

	var cfg *config.Config
	var err error
	if opts.DODOnly {
		cfg = config.Defaults()
	} else {
		cfg, err = config.Load(dir)
		if err != nil {
			res.Failures = append(res.Failures, "core: config invalid: "+err.Error())
		} else {
			res.Failures = append(res.Failures, CoreChecks(dir, cfg)...)
		}
	}

	dodPath := opts.DODPath
	if dodPath == "" && cfg != nil {
		dodPath = cfg.DOD
	}
	var dod *DOD
	if dodPath != "" {
		dod, err = LoadDOD(join(dir, dodPath))
		if err != nil {
			// Invalid or empty DOD is a hard fail, not a skip.
			res.Failures = append(res.Failures, "DOD: "+err.Error())
		} else {
			res.Failures = append(res.Failures, dod.Evaluate(dir)...)
		}
	}

	if !opts.DODOnly && cfg != nil {
		var thresholds *Thresholds
		if dod != nil {
			thresholds = dod.Quality
		}
		res.Quality = QualityOfficer(dir, cfg.DisabledChecks(), thresholds)
		res.Failures = append(res.Failures, res.Quality.Failures...)
		if dod != nil {
			res.Failures = append(res.Failures, RunPluggable(dir, dod.Tools)...)
		}
	}

	res.Passed = len(res.Failures) == 0
	return res, nil
}

// Report renders the verdict for humans and for the worker feedback loop:
// the whole picture in one shot.
func (r *Result) Report() string {
	var sb strings.Builder
	if r.Passed {
		sb.WriteString("VERDICT: PASS — every check holds\n")
		return sb.String()
	}
	sb.WriteString(fmt.Sprintf("VERDICT: FAIL — %d failure(s)\n", len(r.Failures)))
	for _, f := range r.Failures {
		sb.WriteString("  ✗ " + f + "\n")
	}
	return sb.String()
}
