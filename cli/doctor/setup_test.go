package doctor

import "testing"

// W176: a shop names its setup list, so whisk deploy and the Whisk bar can say what the business
// still has to give.
func TestW176ShopNamesItsSetupList(t *testing.T) {
	manifest := passing["whisk.yaml"]
	cases := []struct {
		name, yaml string
		want       bool
	}{
		{"a shop without a list", "commerce: medusa\n", true},
		{"a shop with an admin and no list", "commerce: medusa\nadmin:\n  path: /app\n", true},
		{"a shop with its list", "commerce: medusa\nadmin:\n  path: /app\n  setup: /admin/whisk/setup\n", false},
		{"an app that is no shop", "admin:\n  path: /app\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, map[string]string{"whisk.yaml": manifest + c.yaml}), false)
			if got := has(rep, "W176"); got != c.want {
				t.Errorf("W176 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}
