package naughty

import "testing"

func TestStringsLoads(t *testing.T) {
	s := Strings()
	if len(s) < 500 {
		t.Fatalf("expected the whole list, got %d strings", len(s))
	}
	s[0] = "changed"
	if Strings()[0] == "changed" {
		t.Fatal("Strings must return a copy")
	}
}
