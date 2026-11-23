// Package relay runs the self-hosted ChatGo relay: it accepts one outbound
// WebSocket tunnel per desktop, pairs phones with a desktop, and forwards the
// phone's /api requests over that tunnel. The relay never dials a desktop and
// never parses conversation payloads.
package relay

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Protocol version carried by the desktop tunnel and the pair response.
const Protocol = 1

const (
	// HeaderDesktopID names the desktop a tunnel belongs to.
	HeaderDesktopID = "X-ChatGo-Desktop-ID"
	// HeaderDesktopName is the display name of the machine running ChatGo.
	HeaderDesktopName = "X-ChatGo-Desktop-Name"
	// HeaderProtocol is the relay protocol version of a tunnel.
	HeaderProtocol = "X-ChatGo-Protocol"
	// HeaderDeviceID names the paired phone a proxied request came from.
	HeaderDeviceID = "X-ChatGo-Relay-Device"
	// HeaderRelayDesktop is set by the relay and checked by the desktop
	// connector so a request cannot cross desktops.
	HeaderRelayDesktop = "X-ChatGo-Relay-Desktop"
)

// Errors the relay returns to protocol clients.
var (
	ErrDesktopOffline = errors.New("desktop is offline")
	ErrDeviceRevoked  = errors.New("device is revoked")
	ErrDeviceDisabled = errors.New("device is disabled")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrBadPairCode    = errors.New("invalid or expired pair code")
	ErrRateLimited    = errors.New("too many requests")
)

// APIError is a JSON error body.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error APIError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	writeJSON(w, status, errorBody{Error: APIError{Code: code, Message: msg}})
}

// NewID returns a random hex identifier with a short prefix.
func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("chatgo relay: cannot generate id: " + err.Error())
	}
	return prefix + hex.EncodeToString(b[:])
}

// NewToken returns a 256-bit token in URL-safe base32.
func NewToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("chatgo relay: cannot generate token: " + err.Error())
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}

// HashToken is the only form of a device token the relay stores.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if scheme, value, ok := strings.Cut(auth, " "); ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(value)
	}
	return ""
}
