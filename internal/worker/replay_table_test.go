package worker

import (
	"fmt"
	"strings"
	"testing"
)

// TestReplayTable prints one line per fixture: outcome class, model calls,
// protocol errors. Used to compare behavior across commits (S4).
func TestReplayTable(t *testing.T) {
	var rows []string
	for _, name := range fixtureNames {
		o := replayFixture(t, name, 24)
		outcome := "done"
		if o.Err != nil {
			outcome = strings.SplitN(o.Err.Error(), " (", 2)[0]
			if len(outcome) > 60 {
				outcome = outcome[:60]
			}
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %d | %d | %d | %d |", name, outcome, o.Turns, o.ProtocolErrors, o.UnknownTools, o.NoteOK))
	}
	t.Log("\n" + strings.Join(rows, "\n"))
}
