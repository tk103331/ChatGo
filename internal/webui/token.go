package webui

import (
	"crypto/rand"
	"encoding/hex"
)

// NewToken returns a 192-bit random token. The Web server always runs with a
// token: a configured one is long-lived, an empty config gets a fresh token
// on every start.
func NewToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("chatgo: cannot generate web token: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
