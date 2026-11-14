package agent

import (
	"fmt"
	"slices"
)

// tadwToolingWindow is worker-local. Only FromEventID is a durable step result.
// Empty means no tooling has been dropped. A nonempty ID is the inclusive first
// retained tool event, not the owning request (which is always pinned).
type tadwToolingWindow struct {
	FromEventID string
	Transcript  tadwTranscript
}

// selectTADWToolingRollover consumes newest-first rendered tool events only.
// The owning request counts toward both the hard budget and the 50% soft target.
// Keep the newest tool event if any; then add whole older events until reaching
// the target. A target-crossing event is included unless it would cross the hard
// budget, in which case stop before it. Never skip that event to fill gaps.
//
// This pure policy is called only after the previous window exceeds its budget.
// Its caller restricts the input to that window, so dropped events cannot return.
// The returned order is owning request first, then the chronological tool suffix.
func selectTADWToolingRollover(root tadwRenderedEvent, next func() (tadwRenderedEvent, bool, error), budget int64) ([]tadwRenderedEvent, error) {
	if root.ID == "" || root.Bytes() == 0 || budget < 0 {
		return nil, fmt.Errorf("invalid TADW tooling rollover input")
	}
	if root.Bytes() > budget {
		return nil, &tadwToolingBudgetError{BudgetBytes: budget, EventID: root.ID, EventBytes: root.Bytes()}
	}
	used, target := root.Bytes(), budget/2
	seen := map[string]bool{root.ID: true}
	var reverse []tadwRenderedEvent
	for len(reverse) == 0 || used < target {
		fragment, ok, err := next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		if fragment.ID == "" || fragment.Bytes() == 0 || seen[fragment.ID] {
			return nil, fmt.Errorf("invalid/duplicate TADW tooling rollover event %q", fragment.ID)
		}
		if fragment.Bytes() > budget-used {
			if len(reverse) == 0 {
				return nil, &tadwToolingBudgetError{BudgetBytes: budget, UsedBytes: used, EventID: fragment.ID, EventBytes: fragment.Bytes()}
			}
			break
		}
		reverse = append(reverse, fragment)
		used += fragment.Bytes()
		seen[fragment.ID] = true
	}
	slices.Reverse(reverse)
	result := make([]tadwRenderedEvent, 1, len(reverse)+1)
	result[0] = root
	return append(result, reverse...), nil
}
