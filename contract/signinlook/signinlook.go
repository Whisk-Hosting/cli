// Package signinlook is the arithmetic of an app's sign-in look (CONTROL-PLANE.md §4.6, "An app's
// own look"): reading its colours, how readable two of them are together, which text sits on a
// button of its colour, and what kind of picture its logo is. The pages, the emails, doctor and
// the push read it from here so they agree. Everything here is pure.
package signinlook

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Whisk's own colours, which stand for any the app leaves unset.
const (
	DefaultColour     = "#14213d"
	DefaultBackground = "#faf8f3"
	DefaultText       = "#14213d"
)

// The texts a button of the app's colour carries: white, or near-black on a light colour.
const (
	Light = "#ffffff"
	Dark  = "#111111"
)

// MinContrast is the WCAG AA contrast for body text (W131).
const MinContrast = 4.5

// MaxLogo is the largest logo the push keeps (W130).
const MaxLogo = 100 << 10

// The logo types a page and an email can both show.
const (
	PNG  = "image/png"
	JPEG = "image/jpeg"
)

// RGB is a colour's red, green and blue, 0 to 255.
type RGB struct{ R, G, B uint8 }

// Parse reads #rgb or #rrggbb, case aside.
func Parse(hex string) (RGB, bool) {
	hex = strings.TrimSpace(hex)
	if !strings.HasPrefix(hex, "#") {
		return RGB{}, false
	}
	h := hex[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return RGB{}, false
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}, true
}

// Hex is the colour as #rrggbb, lower case.
func (c RGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// Normal is a colour setting as #rrggbb, or fallback when it is unset or unreadable.
func Normal(hex, fallback string) string {
	if c, ok := Parse(hex); ok {
		return c.Hex()
	}
	return fallback
}

// luminance is WCAG's relative luminance.
func (c RGB) luminance() float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// Contrast is the WCAG contrast ratio of two colours, from 1 to 21; 0 when either does not read.
func Contrast(a, b string) float64 {
	x, ok1 := Parse(a)
	y, ok2 := Parse(b)
	if !ok1 || !ok2 {
		return 0
	}
	l1, l2 := x.luminance(), y.luminance()
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// OnColour is the text a button of colour carries: Light or Dark, whichever contrasts more.
func OnColour(colour string) string {
	if Contrast(colour, Dark) > Contrast(colour, Light) {
		return Dark
	}
	return Light
}

// Mix is a share (0 to 1) of a over b, as #rrggbb; b when either does not read.
func Mix(a, b string, share float64) string {
	x, ok1 := Parse(a)
	y, ok2 := Parse(b)
	if !ok1 || !ok2 {
		return Normal(b, b)
	}
	m := func(p, q uint8) uint8 { return uint8(math.Round(float64(p)*share + float64(q)*(1-share))) }
	return RGB{m(x.R, y.R), m(x.G, y.G), m(x.B, y.B)}.Hex()
}

// LogoType is a logo's media type judged by its first bytes, never its name: PNG, JPEG or "".
func LogoType(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return PNG
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return JPEG
	}
	return ""
}

// Extension is the file extension a logo of type is served with.
func Extension(typ string) string {
	if typ == JPEG {
		return "jpg"
	}
	return "png"
}

// LogoProblem is what is wrong with a logo's bytes for W130, or "" when it may be kept.
func LogoProblem(name string, b []byte) string {
	switch {
	case len(b) > MaxLogo:
		return fmt.Sprintf("%s is %d KB; a sign-in logo is at most %d KB", name, (len(b)+1023)/1024, MaxLogo>>10)
	case LogoType(b) == "":
		return name + " is not a PNG or JPEG"
	}
	return ""
}
