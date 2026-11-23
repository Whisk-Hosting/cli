package webpush

import (
	"encoding/json"
	"testing"
)

func TestManifest(t *testing.T) {
	cases := []struct {
		name string
		in   Install
		want map[string]any
		icon bool
	}{
		{"the app's look", Install{Name: "Team Inbox", Colour: "#0A7", Background: "#ffffff", Icon: "https://auth.whisk.run/assets/brand/a/b.png", IconType: "image/png"},
			map[string]any{"name": "Team Inbox", "short_name": "Team Inbox", "theme_color": "#00aa77", "background_color": "#ffffff", "display": "standalone", "start_url": "/"}, true},
		{"Whisk's colours where it set none", Install{Name: "CRM"},
			map[string]any{"name": "CRM", "theme_color": "#14213d", "background_color": "#faf8f3"}, false},
		{"a colour that is not one", Install{Name: "CRM", Colour: "red;}"},
			map[string]any{"theme_color": "#14213d"}, false},
		{"a long name shortened", Install{Name: "Harbor Freight Dispatch"},
			map[string]any{"short_name": "Harbor Freig"}, false},
		{"no name", Install{}, map[string]any{"name": "App"}, false},
	}
	for _, c := range cases {
		var got map[string]any
		if err := json.Unmarshal(Manifest(c.in), &got); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %s = %v, want %v", c.name, k, got[k], v)
			}
		}
		if _, ok := got["icons"]; ok != c.icon {
			t.Errorf("%s: icons %v", c.name, got["icons"])
		}
	}
}
