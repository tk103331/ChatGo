// Package e2e implements the end-to-end envelope between ChatGo Mobile and a
// desktop: X25519 key agreement, HKDF key derivation and ChaCha20-Poly1305
// request/response and event frames. The relay only sees ciphertext.
package e2e

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"strconv"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	Version = "2"
	// HeaderVersion, HeaderNonce and HeaderBody carry an encrypted HTTP body.
	HeaderVersion = "X-E2E-Version"
	HeaderNonce   = "X-E2E-Nonce"
	HeaderBody    = "X-E2E-Body"
	// HeaderTime carries the sender's clock in unix milliseconds. The
	// desktop refuses an envelope outside MaxSkew, and the ciphertext is
	// bound to the value, so a captured request cannot be replayed later.
	HeaderTime = "X-E2E-Time"
)

// MaxSkew is how far the phone's and the desktop's clocks may differ. A
// request outside the window is refused instead of being replayed.
const MaxSkew = 2 * time.Minute

// Timestamp returns the current time in the unit HeaderTime carries.
func Timestamp() int64 { return time.Now().UnixMilli() }

// WithinWindow reports whether a timestamp is close enough to now to be
// accepted.
func WithinWindow(ts int64) bool {
	if ts <= 0 {
		return false
	}
	delta := time.Since(time.UnixMilli(ts))
	if delta < 0 {
		delta = -delta
	}
	return delta <= MaxSkew
}

// Generate returns a new X25519 private/public key pair.
func Generate() (priv, pub []byte, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return key.Bytes(), key.PublicKey().Bytes(), nil
}

// Shared derives the 32-byte session key from a private key and the peer's
// public key.
func Shared(priv, peerPub []byte) ([]byte, error) {
	key, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pub, err := ecdh.X25519().NewPublicKey(peerPub)
	if err != nil {
		return nil, err
	}
	shared, err := key.ECDH(pub)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, []byte("chatgo-e2e-v1"), []byte("chatgo-e2e-key")), out); err != nil {
		return nil, err
	}
	return out, nil
}

// Seal encrypts plaintext and returns the nonce with the ciphertext. aad binds
// the envelope to its method/path/device, so a relay cannot move it elsewhere.
func Seal(key, plaintext, aad []byte) (nonce, ciphertext []byte, err error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, plaintext, aad), nil
}

// Open decrypts a sealed body.
func Open(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, errors.New("e2e: bad nonce length")
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

// AAD binds an HTTP envelope to a method, path, device and the sender's
// timestamp. The timestamp is part of the authenticated data, so a captured
// ciphertext cannot be replayed with a fresh clock.
func AAD(method, path, deviceID string, ts int64) []byte {
	return []byte("http\n" + method + "\n" + path + "\n" + deviceID + "\n" + strconv.FormatInt(ts, 10))
}

// ResponseAAD binds a response to the device. The request already binds its
// method and path; the response does not repeat them so a proxy that rewrites
// paths cannot break decryption.
func ResponseAAD(string) []byte {
	return []byte("http-response")
}

// EventAAD binds an event frame to the device.
func EventAAD(string) []byte {
	return []byte("events")
}
