package edgetune

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestKnobsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range Knobs {
		if !strings.HasPrefix(k.Key, "edge.") || seen[k.Key] || k.Label == "" {
			t.Errorf("%s: key or label", k.Key)
		}
		seen[k.Key] = true
		if !(k.Min <= k.Default && k.Default <= k.Max) || k.Min <= 0 {
			t.Errorf("%s: default %d outside %d..%d", k.Key, k.Default, k.Min, k.Max)
		}
	}
}

func TestApply(t *testing.T) {
	t.Cleanup(func() { Apply(nil) })
	if got := DurationOf("edge.pass_lifetime"); got != 24*time.Hour {
		t.Fatalf("default %s", got)
	}
	refused := Apply(map[string]int64{"edge.pass_lifetime": int64(2 * time.Hour), "edge.trap_strikes": 0, "edge.nothing": 1})
	slices.Sort(refused)
	if !slices.Equal(refused, []string{"edge.nothing", "edge.trap_strikes"}) {
		t.Errorf("refused %v", refused)
	}
	if DurationOf("edge.pass_lifetime") != 2*time.Hour || Of("edge.trap_strikes") != 3 {
		t.Errorf("applied: %s %d", DurationOf("edge.pass_lifetime"), Of("edge.trap_strikes"))
	}
	Apply(map[string]int64{})
	if DurationOf("edge.pass_lifetime") != 24*time.Hour {
		t.Error("a value left out did not go back to its default")
	}
}
