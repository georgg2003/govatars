package imagevalidate

import (
	"bytes"
)

// MatchesDeclaredMIME checks that the first bytes match the declared image/* MIME (sniff vs magic).
func MatchesDeclaredMIME(mime string, head []byte) bool {
	switch mime {
	case "image/jpeg":
		return len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF
	case "image/png":
		return len(head) >= 8 && bytes.HasPrefix(head, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	case "image/webp":
		return len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP"
	default:
		return false
	}
}
