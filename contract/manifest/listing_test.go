package manifest

import "testing"

// The listing (CONTRACT.md §3.4) is optional, trimmed, and refuses a control character.
func TestListing(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: job-tracker\ntitle: \" Job board \"\ndescription: Every open job.\ncategory: Jobs\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Job board" || m.Description != "Every open job." || m.Category != "Jobs" {
		t.Errorf("listing = %q %q %q", m.Title, m.Description, m.Category)
	}
	if m, err := Parse([]byte("whisk: 1\nname: job-tracker\n")); err != nil || m.Title != "" || m.Category != "" {
		t.Errorf("left out: %+v %v", m, err)
	}
	if _, err := Parse([]byte("whisk: 1\nname: job-tracker\ncategory: \"Sales\\tTeam\"\n")); err == nil {
		t.Error("a tab in the category was accepted")
	}
}
