package relayclient

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"chatgo/internal/e2e"
)

func e2eRequest(t *testing.T, key []byte, device string, ts int64, body []byte) *http.Request {
	t.Helper()
	nonce, ciphertext, err := e2e.Seal(key, body, e2e.AAD("POST", "/api/sessions", device, ts))
	if err != nil {
		t.Fatal(err)
	}
	return e2eRequestWith(t, nonce, ciphertext, ts)
}

// e2eRequestWith builds a request around an already sealed envelope, so a
// test can send the very same bytes twice.
func e2eRequestWith(t *testing.T, nonce, ciphertext []byte, ts int64) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(ciphertext))
	req.Header.Set(e2e.HeaderVersion, e2e.Version)
	req.Header.Set(e2e.HeaderNonce, base64.StdEncoding.EncodeToString(nonce))
	req.Header.Set(e2e.HeaderTime, strconv.FormatInt(ts, 10))
	return req
}

func TestDecryptRequestRejectsReplay(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	device := "dev_1"
	c := New(Config{})
	ts := e2e.Timestamp()
	body := []byte(`{"title":"hi"}`)
	nonce, ciphertext, err := e2e.Seal(key, body, e2e.AAD("POST", "/api/sessions", device, ts))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.decryptRequest(e2eRequestWith(t, nonce, ciphertext, ts), key, device); err != nil {
		t.Fatalf("first request: %v", err)
	}
	_, err = c.decryptRequest(e2eRequestWith(t, nonce, ciphertext, ts), key, device)
	if err == nil || !strings.Contains(err.Error(), "replayed") {
		t.Fatalf("replayed request error = %v", err)
	}
}

func TestDecryptRequestRejectsStaleTimestamp(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	device := "dev_1"
	c := New(Config{})
	old := e2e.Timestamp() - int64(10*e2e.MaxSkew.Milliseconds())
	_, err := c.decryptRequest(e2eRequest(t, key, device, old, []byte(`{}`)), key, device)
	if err == nil || !strings.Contains(err.Error(), "window") {
		t.Fatalf("stale request error = %v", err)
	}
}

func TestDecryptRequestNeedsTimestamp(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	device := "dev_1"
	c := New(Config{})
	req := e2eRequest(t, key, device, e2e.Timestamp(), []byte(`{}`))
	req.Header.Del(e2e.HeaderTime)
	if _, err := c.decryptRequest(req, key, device); err == nil {
		t.Fatal("a request without a timestamp must be refused")
	}
}
