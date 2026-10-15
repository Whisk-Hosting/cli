package wakewindow

import (
	"testing"
	"time"
)

// The spans must nest with margin: Confirm inside Grace, and Grace at least five minutes inside
// SubnetQuarantine, or a stale address could reach a reused subnet.
func TestSpansNest(t *testing.T) {
	if Confirm >= Grace {
		t.Errorf("Confirm %s must be shorter than Grace %s", Confirm, Grace)
	}
	if margin := SubnetQuarantine - Grace; margin < 5*time.Minute {
		t.Errorf("SubnetQuarantine %s leaves only %s beyond Grace %s; keep at least 5m", SubnetQuarantine, margin, Grace)
	}
}
