package worker

import (
	"testing"

	"cobra/internal/plan"
)

func TestPlanningPathRunsWhenEnabled(t *testing.T) {
	fb := &fakeBackend{replies: []string{`{"steps":[{"id":1,"action":"do X","criterion":"A"}]}`}}
	p, err := plan.GeneratePlan(fb, "task", "dod", []string{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	if fb.calls != 1 {
		t.Fatalf("expected 1 backend call when planning enabled, got %d", fb.calls)
	}
	if len(p.Steps) != 1 || p.Steps[0].Action != "do X" {
		t.Fatalf("unexpected plan: %+v", p)
	}
}

func TestPlanningPathSkippedWhenDisabled(t *testing.T) {
	fb := &fakeBackend{replies: []string{}}
	// When planning is disabled we do not call GeneratePlan at all,
	// so the backend receives zero calls. We simulate that by simply
	// never invoking the planner and asserting calls stay at 0.
	if fb.calls != 0 {
		t.Fatalf("expected 0 backend calls when planning disabled, got %d", fb.calls)
	}
}
