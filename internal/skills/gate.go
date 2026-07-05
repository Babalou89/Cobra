package skills

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DefaultGateTimeout bounds how long a skill test may run.
const DefaultGateTimeout = 30 * time.Second

// Gate is the deterministic adoption decision: run the candidate's test in
// its staged directory; exit 0 means the skill proved itself, anything else
// means it does not exist. No model output is consulted — the proposer's
// claims about its own skill are worthless here by design.
func Gate(stagedDir string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultGateTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// The test runs from inside the staged dir, so the script name is
	// passed bare — a joined path would be resolved against cmd.Dir again.
	cmd := exec.CommandContext(ctx, "bash", "test.sh")
	cmd.Dir = stagedDir
	// Without WaitDelay, a killed test whose children still hold the output
	// pipe (e.g. a stray sleep) blocks CombinedOutput long past the timeout.
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("gate: test timed out after %s", timeout)
	}
	if err != nil {
		return fmt.Errorf("gate: test failed: %s (%v)", tail(string(out), 300), err)
	}
	return nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
