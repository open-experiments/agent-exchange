package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex returns the SHA256 hash of a string as a hex-encoded string
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
