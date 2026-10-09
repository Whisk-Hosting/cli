package status

import (
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite deploy.json")

// deploy.json is the deploy statuses for code that cannot import Go: the dashboard's tests read
// it, so its own list cannot drift from this one.
func TestDeployJSONIsCurrent(t *testing.T) {
	want, err := json.MarshalIndent(map[string]any{
		"statuses":     Deploys(),
		"finished":     Filter(Deploy.Finished),
		"supersedable": Filter(Deploy.Supersedable),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *update {
		if err := os.WriteFile("deploy.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile("deploy.json")
	if err != nil || string(got) != string(want) {
		t.Fatalf("deploy.json is stale: run go test ./status -update")
	}
}

func TestEveryDeployEitherFinishesOrMovesOn(t *testing.T) {
	for _, d := range Deploys() {
		if !d.Valid() {
			t.Errorf("%s is listed but not valid", d)
		}
		if d.Finished() && d.Supersedable() {
			t.Errorf("%s is both finished and supersedable", d)
		}
	}
	if Deploy("nonsense").Valid() || Deploy("nonsense").Finished() {
		t.Error("an unknown status is neither valid nor finished")
	}
}
