package custom

import (
	"reflect"
	"testing"
)

// A release meets a change when it changed a file the change names, the file itself or one in a
// folder it names; each file belongs to every change that names it.
func TestMeets(t *testing.T) {
	warehouse := Change{Name: "tell-the-warehouse", Touches: []string{"custom/server/warehouse.ts", "src/deals/won.ts"}}
	region := Change{Name: "company-region", Touches: []string{"src/companies/", "custom/migrations/0001_region.sql"}}
	released := []string{"migrations/0032_deal_stages.sql", "src/companies/list.tsx", "src/companies/filters/region.ts", "src/deals/won.ts"}
	for _, c := range []struct {
		change Change
		want   []string
	}{
		{warehouse, []string{"src/deals/won.ts"}},
		{region, []string{"src/companies/list.tsx", "src/companies/filters/region.ts"}},
		{Change{Name: "own-page", Touches: []string{"custom/web/page.tsx"}}, nil},
	} {
		if got := Meets(c.change, released); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Meets(%s) = %q, want %q", c.change.Name, got, c.want)
		}
	}
	changes := []Change{warehouse, region}
	for p, want := range map[string][]Change{
		"src/deals/won.ts":         {warehouse},
		"src/companies/list.tsx":   {region},
		"migrations/0001_init.sql": {},
	} {
		if got := Naming(changes, p); len(got) != len(want) || len(want) > 0 && !reflect.DeepEqual(got, want) {
			t.Errorf("Naming(%s) = %+v, want %+v", p, got, want)
		}
	}
	if len(changes) != 2 || changes[0].Name != "tell-the-warehouse" {
		t.Errorf("Naming changed what it was given: %+v", changes)
	}
}
