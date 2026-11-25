package native

import (
	"crypto/rand"
	"encoding/hex"
)

// functionCallID names one renderer-initiated function call.
func functionCallID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "rfn-" + hex.EncodeToString(b[:]), nil
}
