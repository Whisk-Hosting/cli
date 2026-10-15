// Package textenc reads text that Windows tools wrote: a UTF-8 byte-order mark (many editors)
// and UTF-16 with a byte-order mark (Windows PowerShell's > and Out-File) become plain UTF-8,
// so every file and stdin input the CLI takes reads the same on every system.
package textenc

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// Decode turns text bytes into UTF-8: a UTF-8 byte-order mark is dropped and UTF-16 with a
// byte-order mark is converted. Anything else is returned as it is. Pure.
func Decode(b []byte) []byte {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return b[3:]
	case len(b) >= 2 && (b[0] == 0xFF && b[1] == 0xFE || b[0] == 0xFE && b[1] == 0xFF):
		order := binary.ByteOrder(binary.LittleEndian)
		if b[0] == 0xFE {
			order = binary.BigEndian
		}
		units := make([]uint16, (len(b)-2)/2)
		for i := range units {
			units[i] = order.Uint16(b[2+2*i:])
		}
		return []byte(string(utf16.Decode(units)))
	}
	return b
}
