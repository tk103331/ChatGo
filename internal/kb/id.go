package kb

import (
	"crypto/rand"
	"encoding/hex"
)

// newID returns a random 128-bit identifier in hex.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString(b[:])
	}
	return hex.EncodeToString(b[:])
}
