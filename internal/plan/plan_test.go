package plan

import (
	"reflect"
	"testing"
)

func TestCoverageGaps_missing(t *testing.T) {
	criteria := []string{"A", "B", "C"}
	p := Plan{
		Steps: []Step{
			{ID: 1, Action: "do A", Criterion: "A"},
			{ID: 2, Action: "do B", Criterion: "B"},
		},
	}
	got := CoverageGaps(criteria, p)
	want := []string{"C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CoverageGaps() = %v, want %v", got, want)
	}
}

func TestCoverageGaps_allCovered(t *testing.T) {
	criteria := []string{"A", "B"}
	p := Plan{
		Steps: []Step{
			{ID: 1, Action: "do A", Criterion: "A"},
			{ID: 2, Action: "do B", Criterion: "B"},
		},
	}
	got := CoverageGaps(criteria, p)
	if len(got) != 0 {
		t.Fatalf("CoverageGaps() = %v, want empty", got)
	}
}
