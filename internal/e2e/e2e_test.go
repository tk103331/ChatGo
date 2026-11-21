package e2e

import (
	"testing"
	"time"
)

func TestAADBindsTimestamp(t *testing.T) {
	a := AAD("POST", "/api/sessions", "dev_1", 1000)
	b := AAD("POST", "/api/sessions", "dev_1", 2000)
	if string(a) == string(b) {
		t.Fatal("aad must differ for a different timestamp")
	}
	if string(AAD("POST", "/api/sessions", "dev_1", 1000)) != string(a) {
		t.Fatal("aad must be stable for the same inputs")
	}
}

func TestWithinWindow(t *testing.T) {
	if !WithinWindow(Timestamp()) {
		t.Fatal("now must be inside the window")
	}
	if WithinWindow(Timestamp() - int64(3*MaxSkew/time.Millisecond)) {
		t.Fatal("an old timestamp must be outside the window")
	}
	if WithinWindow(Timestamp() + int64(3*MaxSkew/time.Millisecond)) {
		t.Fatal("a future timestamp must be outside the window")
	}
	if WithinWindow(0) {
		t.Fatal("a missing timestamp must be outside the window")
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	ts := Timestamp()
	nonce, ciphertext, err := Seal(key, []byte("hello"), AAD("GET", "/api/meta", "dev_1", ts))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Open(key, nonce, ciphertext, AAD("GET", "/api/meta", "dev_1", ts))
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "hello" {
		t.Fatalf("plain = %q", plain)
	}
	if _, err := Open(key, nonce, ciphertext, AAD("GET", "/api/meta", "dev_1", ts+1)); err == nil {
		t.Fatal("a replayed request with a fresh timestamp must not decrypt")
	}
}
