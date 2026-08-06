package cage

import "strings"

// Directive is what the cage tells Ralph to tell the model.
// Task is always present (from DOD). FixTarget is populated after
// turn 1 when the cage has verify failures to report.
type Directive struct {
	Task      string // what to build — always from DOD.Task
	FixTarget string // verify failures to fix — empty on turn 1
	Turn      int
}

// Direct breaks the DOD into a single directive for Ralph to inject.
// Turn 1: cage gives the model the task (no failures yet).
// Turn 2+: cage tells Ralph exactly what failed — Ralph steers the model.
func (d *DOD) Direct(attempt int, verifyReport string) Directive {
	if attempt <= 0 {
		attempt = 1
	}
	dir := Directive{
		Task: d.Task,
		Turn: attempt,
	}
	if verifyReport != "" {
		dir.FixTarget = verifyReport
	}
	return dir
}

// ClampContext keeps only the last maxLines of content.
// This is the rolling window: oldest lines drop, newest stay.
func ClampContext(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	lines = lines[len(lines)-maxLines:]
	return strings.Join(lines, "\n")
}
