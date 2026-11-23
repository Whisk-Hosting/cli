package webpush

import (
	"encoding/json"
	"strings"

	"github.com/whisk-run/contract/signinlook"
)

// InstallPath is where Whisk serves an app's web manifest, so any app can be added to a phone's
// Home Screen as an app of its own: on an iPhone or iPad that is what lets it show notifications.
// A page links it with <link rel="manifest" href="/.whisk/manifest.webmanifest">.
const InstallPath = "/.whisk/manifest.webmanifest"

// Install is what an app's web manifest is made from: its name, its sign-in colours and its logo.
type Install struct {
	Name       string `json:"name"`
	Colour     string `json:"colour,omitempty"`
	Background string `json:"background,omitempty"`
	// Icon is the logo's full address, "" for none.
	Icon     string `json:"icon,omitempty"`
	IconType string `json:"icon_type,omitempty"`
}

// Manifest is the web manifest for in: opened standalone at the app's front page, in the app's
// colours (Whisk's where it set none), with its logo as the icon. Pure.
func Manifest(in Install) []byte {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "App"
	}
	short := name
	if r := []rune(short); len(r) > 12 {
		short = strings.TrimSpace(string(r[:12]))
	}
	theme := colourOr(in.Colour, signinlook.DefaultColour)
	background := colourOr(in.Background, signinlook.DefaultBackground)
	m := map[string]any{
		"name": name, "short_name": short, "start_url": "/", "scope": "/", "display": "standalone",
		"theme_color": theme, "background_color": background,
	}
	if in.Icon != "" {
		m["icons"] = []map[string]string{{"src": in.Icon, "sizes": "any", "type": in.IconType, "purpose": "any"}}
	}
	b, _ := json.Marshal(m)
	return b
}

// colourOr is a #rrggbb colour, or or when v is not one. Pure.
func colourOr(v, or string) string {
	c, ok := signinlook.Parse(v)
	if !ok {
		return or
	}
	return c.Hex()
}
