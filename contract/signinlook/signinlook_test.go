package signinlook

import (
	"math"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"#0b6e4f", "#0b6e4f", true},
		{"#0B6E4F", "#0b6e4f", true},
		{"#abc", "#aabbcc", true},
		{"0b6e4f", "", false},
		{"#0b6e4", "", false},
		{"#ggg", "", false},
		{"", "", false},
		{"#", "", false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if ok != c.ok || (ok && got.Hex() != c.want) {
			t.Errorf("Parse(%q) = %v %v, want %q %v", c.in, got.Hex(), ok, c.want, c.ok)
		}
	}
}

func TestContrast(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"#000000", "#ffffff", 21},
		{"#ffffff", "#000000", 21},
		{"#777777", "#777777", 1},
		{"#1a1a1a", "#ffffff", 17.4},
		{"#cccccc", "#ffffff", 1.6},
		{"nope", "#ffffff", 0},
	}
	for _, c := range cases {
		if got := Contrast(c.a, c.b); math.Abs(got-c.want) > 0.05 {
			t.Errorf("Contrast(%s, %s) = %.2f, want %.1f", c.a, c.b, got, c.want)
		}
	}
}

func TestOnColour(t *testing.T) {
	cases := map[string]string{
		"#0b6e4f": Light, "#14213d": Light, "#e3a92b": Dark, "#ffff00": Dark, "#ffffff": Dark, "#000000": Light,
	}
	for colour, want := range cases {
		if got := OnColour(colour); got != want {
			t.Errorf("OnColour(%s) = %s, want %s", colour, got, want)
		}
	}
}

func TestMix(t *testing.T) {
	if got := Mix("#000000", "#ffffff", 0.5); got != "#808080" {
		t.Errorf("Mix half = %s", got)
	}
	if got := Mix("#ff0000", "#0000ff", 1); got != "#ff0000" {
		t.Errorf("Mix all = %s", got)
	}
	if got := Mix("bad", "#0000ff", 1); got != "#0000ff" {
		t.Errorf("Mix bad = %s", got)
	}
}

func TestLogo(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nrest")
	jpg := []byte{0xff, 0xd8, 0xff, 0xe0}
	svg := []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")
	big := append(append([]byte{}, png...), make([]byte, MaxLogo)...)
	cases := []struct {
		name string
		b    []byte
		typ  string
		bad  bool
	}{
		{"png", png, PNG, false},
		{"jpg", jpg, JPEG, false},
		{"svg", svg, "", true},
		{"empty", nil, "", true},
		{"big", big, PNG, true},
	}
	for _, c := range cases {
		if got := LogoType(c.b); got != c.typ {
			t.Errorf("%s: LogoType = %q, want %q", c.name, got, c.typ)
		}
		if got := LogoProblem("logo", c.b); (got != "") != c.bad {
			t.Errorf("%s: LogoProblem = %q", c.name, got)
		}
	}
	if Extension(JPEG) != "jpg" || Extension(PNG) != "png" {
		t.Error("Extension")
	}
}
