package release

import (
	"errors"
	"testing"
)

func TestAssetNameAndSums(t *testing.T) {
	if AssetName("windows", "arm64") != "whisk_windows_arm64.exe" || AssetName("linux", "amd64") != "whisk_linux_amd64" {
		t.Fatal("asset names")
	}
	sums := ParseSums("ABCD  whisk_linux_amd64\n1234 *whisk_windows_amd64.exe\n\nnot a line\n")
	if sums["whisk_linux_amd64"] != "abcd" || sums["whisk_windows_amd64.exe"] != "1234" || len(sums) != 2 {
		t.Fatalf("sums %v", sums)
	}
	for _, tc := range []struct {
		cur, served string
		want        bool
	}{{"dev", "v1.2.0", true}, {"v1.2.0", "v1.2.0\n", false}, {"v1.2.0", "", false}, {"v1.2.0", "v1.3.0", true}} {
		if Newer(tc.cur, tc.served) != tc.want {
			t.Errorf("Newer(%q,%q) != %v", tc.cur, tc.served, tc.want)
		}
	}
}

func TestSignAndVerify(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("abcd  whisk_linux_amd64\n")
	sig, err := Sign(kp.Private, data)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := Verify(kp.Public, data, sig); !ok || err != nil {
		t.Fatalf("verify: %v %v", ok, err)
	}
	if ok, err := Verify(kp.Public, []byte("tampered"), sig); ok || !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered data verified: %v %v", ok, err)
	}
	if ok, err := Verify(kp.Public, data, "bm90IGEgc2ln"); ok || !errors.Is(err, ErrBadSignature) {
		t.Fatalf("garbage signature verified: %v %v", ok, err)
	}
	if ok, err := Verify("", data, sig); ok || err != nil {
		t.Fatalf("no key should be (false, nil): %v %v", ok, err)
	}
	// A public key of the wrong size or not base64 is the build's fault, never a pass.
	for _, bad := range []string{"bm90IGEga2V5", "not base64!"} {
		if ok, err := Verify(bad, data, sig); ok || err == nil || errors.Is(err, ErrBadSignature) {
			t.Fatalf("Verify with public key %q: %v %v", bad, ok, err)
		}
	}
	if _, err := Sign("nope", data); err == nil {
		t.Fatal("a bad private key should be refused")
	}
}

func TestPublicKeyMatchesTheGeneratedPair(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := PublicKey(kp.Private + "\n")
	if err != nil || pub != kp.Public {
		t.Fatalf("PublicKey = %q, %v; want %q", pub, err, kp.Public)
	}
	if _, err := PublicKey("not a key"); err == nil {
		t.Fatal("PublicKey accepted a value that is not a key")
	}
}

func TestStamp(t *testing.T) {
	sums := "abc  whisk_linux_amd64\n" + StampLine(Stamp{Version: "git-1a2b", Released: 1700000000}) + "\n"
	st, ok := ParseStamp(sums)
	if !ok || st != (Stamp{Version: "git-1a2b", Released: 1700000000}) {
		t.Fatalf("ParseStamp = %+v, %v", st, ok)
	}
	if got := ParseSums(sums); len(got) != 1 || got["whisk_linux_amd64"] != "abc" {
		t.Fatalf("the stamp leaked into ParseSums: %v", got)
	}
	for _, bad := range []string{"", "# whisk v released soon", "# whisk v released 0", "whisk v released 5"} {
		if _, ok := ParseStamp(bad); ok {
			t.Errorf("ParseStamp(%q) found a stamp", bad)
		}
	}
}

func TestCheckStamp(t *testing.T) {
	newer, older := Stamp{"n", 200}, Stamp{"o", 100}
	cases := []struct {
		name            string
		released        int64
		signed, stamped bool
		st              Stamp
		force           bool
		want            Refusal
	}{
		{"newer signed release", 150, true, true, newer, false, ""},
		{"older signed release", 150, true, true, older, false, RefuseOlder},
		{"older with force", 150, true, true, older, true, ""},
		{"signed but unstamped", 150, true, false, Stamp{}, false, RefuseUnstamped},
		{"signed unstamped even with force", 150, true, false, Stamp{}, true, RefuseUnstamped},
		{"unsigned unstamped build without a key", 150, false, false, Stamp{}, false, ""},
		{"development build takes anything stamped", 0, true, true, older, false, ""},
		{"same build time", 100, true, true, older, false, ""},
	}
	for _, tc := range cases {
		if got := CheckStamp(tc.released, tc.signed, tc.st, tc.stamped, tc.force); got != tc.want {
			t.Errorf("%s: CheckStamp = %q, want %q", tc.name, got, tc.want)
		}
	}
}
