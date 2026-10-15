package doctor

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16LE is text as Windows PowerShell's > writes it: a byte-order mark, then UTF-16LE.
func utf16LE(s string) string {
	b := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return string(b)
}

// Files Windows tools wrote read as the text they are: a byte-order mark is no finding, a
// UTF-16 source is checked like any other, and a UTF-16 whisk.yaml is named as such (the
// platform reads UTF-8), never as missing.
func TestWindowsEncodings(t *testing.T) {
	bom := runDoctor(t, with(passing, map[string]string{"whisk.yaml": "\xEF\xBB\xBF" + passing["whisk.yaml"]}), false)
	if len(bom.Findings) != 0 {
		t.Errorf("a UTF-8 byte-order mark: %+v", bom.Findings)
	}
	manifest := runDoctor(t, with(passing, map[string]string{"whisk.yaml": utf16LE(passing["whisk.yaml"])}), false)
	if !has(manifest, "W001") || strings.Contains(manifest.Findings[0].Message, "missing") || !strings.Contains(manifest.Findings[0].Message, "UTF-16") {
		t.Errorf("a UTF-16 whisk.yaml: %+v", manifest.Findings)
	}
	source := runDoctor(t, with(passing, map[string]string{"src/index.ts": utf16LE(strings.ReplaceAll(passing["src/index.ts"], "process.env.PORT ?? 8080", "8080"))}), false)
	if !has(source, "W020") {
		t.Errorf("a UTF-16 source was not read: %+v", source.Findings)
	}
	garbage := runDoctor(t, with(passing, map[string]string{"whisk.yaml": "\x00\x01\x02whisk"}), false)
	if !has(garbage, "W001") || strings.Contains(garbage.Findings[0].Message, "missing") {
		t.Errorf("a whisk.yaml that is not text: %+v", garbage.Findings)
	}
}
