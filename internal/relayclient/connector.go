// Package relayclient connects the desktop app to a self-hosted relay. It
// dials out, accepts the relay's proxied /api requests, and sends pairing,
// revocation and notification control requests back over the same tunnel.
package relayclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"

	"chatgo/internal/e2e"
	"chatgo/internal/relay"
)

// State is the connector's current link state.
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
)

// Device is one paired phone as the desktop lists it.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// E2EKey is the per-device ChaCha20-Poly1305. It never leaves the
	// desktop except in the pairing response's public keys.
	E2EKey []byte `json:"-"`
}

// Config describes one relay link.
type Config struct {
	RelayURL     string
	DesktopID    string
	DesktopName  string
	Secret       string
	Version      string
	LocalURL     string
	LocalToken   string
	AllowPairing bool
	Capabilities map[string]bool
	OnState      func(State)
	OnDevices    func([]Device)
}

// Connector keeps one desktop connected to one relay.
type Connector struct {
	cfg Config

	mu      sync.Mutex
	sess    *yamux.Session
	state   State
	devices map[string]Device

	pairCode    string
	pairExpires time.Time
	pairTries   int

	// replay remembers the nonces of recent E2E requests per device, so a
	// captured request cannot be sent twice inside the clock window.
	replay map[string]map[string]time.Time
}

// New returns a connector that has not dialed yet.
func New(cfg Config) *Connector {
	if cfg.Capabilities == nil {
		cfg.Capabilities = map[string]bool{"chat": true, "rooms": true, "tasks": true}
	}
	return &Connector{
		cfg:     cfg,
		state:   StateDisconnected,
		devices: map[string]Device{},
		replay:  map[string]map[string]time.Time{},
	}
}

// seenNonce reports whether a device already used a nonce, and records it
// when it did not. The cache only keeps the window a request may live in.
func (c *Connector) seenNonce(deviceID, nonce string) bool {
	if nonce == "" {
		return false
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := c.replay[deviceID]
	if seen == nil {
		seen = map[string]time.Time{}
		c.replay[deviceID] = seen
	}
	for n, at := range seen {
		if now.Sub(at) > 2*e2e.MaxSkew {
			delete(seen, n)
		}
	}
	if _, ok := seen[nonce]; ok {
		return true
	}
	seen[nonce] = now
	return false
}

// State returns the current link state.
func (c *Connector) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// SetDevices seeds the list from persisted config.
func (c *Connector) SetDevices(devices []Device) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range devices {
		c.devices[d.ID] = d
	}
}

// Devices returns a copy of the paired phones.
func (c *Connector) Devices() []Device {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Device, 0, len(c.devices))
	for _, d := range c.devices {
		out = append(out, d)
	}
	return out
}

// Run keeps the tunnel connected until ctx is canceled.
func (c *Connector) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			c.setState(StateDisconnected)
			return
		}
		c.setState(StateConnecting)
		sess, err := c.dial(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.setState(StateDisconnected)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		c.mu.Lock()
		c.sess = sess
		c.mu.Unlock()
		c.setState(StateConnected)

		serveDone := make(chan struct{})
		go func() {
			defer close(serveDone)
			srv := &http.Server{Handler: c.handler()}
			_ = srv.Serve(relay.NewYamuxListener(sess))
		}()

		select {
		case <-ctx.Done():
			_ = sess.Close()
			<-serveDone
			c.setState(StateDisconnected)
			return
		case <-sess.CloseChan():
		}
		_ = sess.Close()
		<-serveDone
		c.mu.Lock()
		if c.sess == sess {
			c.sess = nil
		}
		c.mu.Unlock()
		c.setState(StateDisconnected)
	}
}

func (c *Connector) dial(ctx context.Context) (*yamux.Session, error) {
	u := strings.TrimRight(c.cfg.RelayURL, "/") + "/v1/tunnel"
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+c.cfg.Secret)
	headers.Set(relay.HeaderDesktopID, c.cfg.DesktopID)
	headers.Set(relay.HeaderDesktopName, c.cfg.DesktopName)
	headers.Set(relay.HeaderProtocol, fmt.Sprint(relay.Protocol))
	conn, resp, err := dialer.DialContext(ctx, u, headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("tunnel: %s: %w", resp.Status, err)
		}
		return nil, err
	}
	conn.SetReadLimit(4 << 20)
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 30 * time.Second
	cfg.LogOutput = io.Discard
	sess, err := yamux.Client(relay.NewWSConn(conn), cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return sess, nil
}

func (c *Connector) setState(s State) {
	c.mu.Lock()
	changed := c.state != s
	c.state = s
	c.mu.Unlock()
	if changed && c.cfg.OnState != nil {
		c.cfg.OnState(s)
	}
}

// handler serves the streams the relay opens: pairing control and /api
// forwarding.
func (c *Connector) handler() http.Handler {
	local, _ := url.Parse(c.cfg.LocalURL)
	proxy := httputil.NewSingleHostReverseProxy(local)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = local.Scheme
		req.URL.Host = local.Host
		req.Host = local.Host
		req.Header.Del("Authorization")
		for key := range req.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-chatgo-") {
				delete(req.Header, key)
			}
		}
		if c.cfg.LocalToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.cfg.LocalToken)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		cookies := resp.Cookies()
		resp.Header.Del("Set-Cookie")
		for _, cookie := range cookies {
			if cookie.Name != "chatgo_web_token" {
				resp.Header.Add("Set-Cookie", cookie.String())
			}
		}
		key, _ := resp.Request.Context().Value(e2eKeyContext{}).([]byte)
		if len(key) == 0 {
			return nil
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return err
		}
		resp.Body.Close()
		nonce, ciphertext, err := e2e.Seal(key, body, e2e.ResponseAAD(deviceIDFrom(resp.Request)))
		if err != nil {
			return err
		}
		resp.Body = io.NopCloser(bytes.NewReader(ciphertext))
		resp.ContentLength = int64(len(ciphertext))
		resp.Header.Set("Content-Length", strconv.Itoa(len(ciphertext)))
		resp.Header.Set("Content-Type", "application/octet-stream")
		resp.Header.Set(e2e.HeaderVersion, e2e.Version)
		resp.Header.Set(e2e.HeaderNonce, base64.StdEncoding.EncodeToString(nonce))
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "local web service unavailable: "+err.Error(), http.StatusBadGateway)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /__relay/pair", func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get(relay.HeaderDesktopID); id != "" && id != c.cfg.DesktopID {
			http.Error(w, "wrong desktop", http.StatusForbidden)
			return
		}
		var in pairRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "bad_request", "message": err.Error()}})
			return
		}
		out, err := c.pair(in)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]string{"code": "pair_rejected", "message": err.Error()}})
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("/__relay/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get(relay.HeaderRelayDesktop); id != "" && id != c.cfg.DesktopID {
			http.Error(w, "wrong desktop", http.StatusForbidden)
			return
		}
		if r.Header.Get(e2e.HeaderVersion) == e2e.Version {
			c.proxyEvents(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get(relay.HeaderRelayDesktop); id != "" && id != c.cfg.DesktopID {
			http.Error(w, "wrong desktop", http.StatusForbidden)
			return
		}
		if r.Header.Get(e2e.HeaderVersion) == e2e.Version {
			deviceID := r.Header.Get(relay.HeaderDeviceID)
			key := c.deviceKey(deviceID)
			if len(key) == 0 {
				http.Error(w, "e2e key not found", http.StatusUnauthorized)
				return
			}
			var err error
			r, err = c.decryptRequest(r, key, deviceID)
			if err != nil {
				code := "e2e_rejected"
				if strings.Contains(err.Error(), "window") {
					code = "e2e_clock_skew"
				}
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": map[string]string{"code": code, "message": err.Error()},
				})
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), e2eKeyContext{}, key))
			r = r.WithContext(context.WithValue(r.Context(), e2eDeviceContext{}, deviceID))
			r.Header.Set("X-E2E-Device", deviceID)
		}
		proxy.ServeHTTP(w, r)
	}))
	return mux
}

type pairRequest struct {
	DesktopID    string `json:"desktop_id"`
	Code         string `json:"code"`
	DeviceName   string `json:"device_name"`
	Platform     string `json:"platform"`
	DevicePubKey string `json:"device_pubkey,omitempty"`
}

type pairResult struct {
	DeviceID      string          `json:"device_id"`
	Token         string          `json:"token"`
	Name          string          `json:"name"`
	Capabilities  map[string]bool `json:"capabilities"`
	DesktopPubKey string          `json:"desktop_pubkey,omitempty"`
	E2E           bool            `json:"e2e,omitempty"`
}

// NewPairCode creates a fresh 8-character code valid for ttl.
func (c *Connector) NewPairCode(ttl time.Duration) (string, time.Time) {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("chatgo relay client: cannot generate pair code: " + err.Error())
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	expires := time.Now().Add(ttl)
	c.mu.Lock()
	c.pairCode = code
	c.pairExpires = expires
	c.pairTries = 0
	c.mu.Unlock()
	return code, expires
}

// PairCode returns the active code and its expiry.
func (c *Connector) PairCode() (string, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pairCode, c.pairExpires
}

func (c *Connector) pair(in pairRequest) (pairResult, error) {
	if !c.cfg.AllowPairing {
		return pairResult{}, errors.New("pairing is disabled on this desktop")
	}
	var desktopPub string
	e2eOn := false
	c.mu.Lock()
	code := c.pairCode
	expires := c.pairExpires
	tries := c.pairTries
	if code == "" || time.Now().After(expires) {
		c.mu.Unlock()
		return pairResult{}, errors.New("pair code expired")
	}
	if tries >= 5 {
		c.mu.Unlock()
		return pairResult{}, errors.New("too many attempts; generate a new code")
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(strings.TrimSpace(in.Code))) != 1 {
		c.pairTries++
		c.mu.Unlock()
		return pairResult{}, errors.New("invalid pair code")
	}
	device := Device{
		ID:        relay.NewID("dev_"),
		Name:      strings.TrimSpace(in.DeviceName),
		Platform:  strings.TrimSpace(in.Platform),
		CreatedAt: time.Now(),
	}
	if in.DevicePubKey != "" {
		peer, err := base64.StdEncoding.DecodeString(in.DevicePubKey)
		if err != nil {
			c.mu.Unlock()
			return pairResult{}, errors.New("invalid device public key")
		}
		priv, pub, err := e2e.Generate()
		if err != nil {
			c.mu.Unlock()
			return pairResult{}, err
		}
		key, err := e2e.Shared(priv, peer)
		if err != nil {
			c.mu.Unlock()
			return pairResult{}, errors.New("invalid device public key")
		}
		device.E2EKey = key
		desktopPub = base64.StdEncoding.EncodeToString(pub)
		e2eOn = true
	}
	token := relay.NewToken()
	c.devices[device.ID] = device
	c.pairCode = ""
	c.pairTries = 0
	devices := make([]Device, 0, len(c.devices))
	for _, d := range c.devices {
		devices = append(devices, d)
	}
	c.mu.Unlock()
	if c.cfg.OnDevices != nil {
		c.cfg.OnDevices(devices)
	}
	return pairResult{
		DeviceID:      device.ID,
		Token:         token,
		Name:          firstNonEmpty(device.Name, "ChatGo Mobile"),
		Capabilities:  c.cfg.Capabilities,
		DesktopPubKey: desktopPub,
		E2E:           e2eOn,
	}, nil
}

// Revoke tells the relay first, then removes the device locally. A relay
// that is offline refuses the revocation rather than leaving a live token in
// a list the user believes is gone.
func (c *Connector) Revoke(ctx context.Context, deviceID string) error {
	if err := c.control(ctx, "/__relay/revoke", map[string]string{"device_id": deviceID}); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.devices, deviceID)
	devices := make([]Device, 0, len(c.devices))
	for _, d := range c.devices {
		devices = append(devices, d)
	}
	c.mu.Unlock()
	if c.cfg.OnDevices != nil {
		c.cfg.OnDevices(devices)
	}
	return nil
}

// Notify asks the relay to push a metadata-only notification.
func (c *Connector) Notify(ctx context.Context, n relay.Notification) error {
	return c.control(ctx, "/__relay/notify", n)
}

func (c *Connector) control(ctx context.Context, path string, in any) error {
	c.mu.Lock()
	sess := c.sess
	c.mu.Unlock()
	if sess == nil {
		return errors.New("relay is not connected")
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://relay"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(relay.HeaderDesktopID, c.cfg.DesktopID)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				return sess.OpenStream()
			},
			ResponseHeaderTimeout: 10 * time.Second,
			ForceAttemptHTTP2:     false,
		},
		Timeout: 15 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type e2eKeyContext struct{}
type e2eDeviceContext struct{}

func (c *Connector) deviceKey(id string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.devices[id].E2EKey
}

func deviceIDFrom(r *http.Request) string {
	if id, _ := r.Context().Value(e2eDeviceContext{}).(string); id != "" {
		return id
	}
	if id := r.Header.Get("X-E2E-Device"); id != "" {
		return id
	}
	return r.Header.Get(relay.HeaderDeviceID)
}

func (c *Connector) decryptRequest(r *http.Request, key []byte, deviceID string) (*http.Request, error) {
	nonceB64 := r.Header.Get(e2e.HeaderNonce)
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil && nonceB64 != "" {
		return nil, err
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get(e2e.HeaderTime)), 10, 64)
	if err != nil {
		return nil, errors.New("e2e envelope has no timestamp")
	}
	if !e2e.WithinWindow(ts) {
		return nil, errors.New("e2e timestamp outside the accepted window")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	r.Body.Close()
	var plaintext []byte
	if len(body) > 0 {
		plaintext, err = e2e.Open(key, nonce, body, e2e.AAD(r.Method, r.URL.Path, deviceID, ts))
		if err != nil {
			return nil, err
		}
		if c.seenNonce(deviceID, nonceB64) {
			return nil, errors.New("e2e request replayed")
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(plaintext))
	r.ContentLength = int64(len(plaintext))
	if len(plaintext) > 0 {
		r.Header.Set("Content-Type", "application/json")
	}
	for key := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "x-e2e-") {
			delete(r.Header, key)
		}
	}
	return r, nil
}

var eventUpgrader = websocket.Upgrader{
	HandshakeTimeout: 15 * time.Second,
	CheckOrigin:      func(*http.Request) bool { return true },
}

func (c *Connector) proxyEvents(w http.ResponseWriter, r *http.Request) {
	deviceID := r.Header.Get(relay.HeaderDeviceID)
	key := c.deviceKey(deviceID)
	if len(key) == 0 {
		http.Error(w, "e2e key not found", http.StatusUnauthorized)
		return
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get(e2e.HeaderTime)), 10, 64)
	if err != nil || !e2e.WithinWindow(ts) {
		http.Error(w, "e2e timestamp outside the accepted window", http.StatusUnauthorized)
		return
	}
	conn, err := eventUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	local := strings.TrimRight(c.cfg.LocalURL, "/")
	local = strings.Replace(local, "https://", "wss://", 1)
	local = strings.Replace(local, "http://", "ws://", 1)
	local += "/api/events"
	if r.URL.RawQuery != "" {
		local += "?" + r.URL.RawQuery
	}
	headers := http.Header{}
	if c.cfg.LocalToken != "" {
		headers.Set("Authorization", "Bearer "+c.cfg.LocalToken)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	upstream, _, err := dialer.Dial(local, headers)
	if err != nil {
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
				continue
			}
			nonce, ciphertext, err := e2e.Seal(key, msg, e2e.EventAAD(deviceID))
			if err != nil {
				return
			}
			frame := make([]byte, 0, 1+len(nonce)+len(ciphertext))
			frame = append(frame, 1)
			frame = append(frame, nonce...)
			frame = append(frame, ciphertext...)
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.BinaryMessage && len(msg) > 13 && msg[0] == 1 {
				plain, err := e2e.Open(key, msg[1:13], msg[13:], e2e.EventAAD(deviceID))
				if err != nil {
					return
				}
				msg = plain
			}
			if err := upstream.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()
	<-done
}
