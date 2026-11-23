package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// Config configures the relay server.
type Config struct {
	// Addr is the public listen address used by ListenAndServe.
	Addr string
	// DataPath is the SQLite file. Empty keeps the store in memory.
	DataPath string
	// PublicURL is the https/wss origin phones and desktops use, e.g.
	// https://relay.example.com.
	PublicURL string
	// PairTTL is how long a desktop's pair code stays valid. Defaults to 2m.
	PairTTL time.Duration
	// MaxBodyBytes bounds a proxied request body. Defaults to 12 MiB, the
	// browser attachment limit.
	MaxBodyBytes int64
	// Pusher delivers notifications. Nil logs them.
	Pusher Pusher
	// Logger receives relay diagnostics. Nil uses log.Default.
	Logger *log.Logger
}

// Server is the relay's HTTP and tunnel service.
type Server struct {
	cfg    Config
	store  *Store
	owns   bool
	logger *log.Logger

	upgrader websocket.Upgrader
	pairs    *rateLimiter

	mu      sync.Mutex
	tunnels map[string]*Tunnel
}

// New opens the store and returns a relay server.
func New(cfg Config) (*Server, error) {
	store, err := OpenStore(cfg.DataPath)
	if err != nil {
		return nil, err
	}
	s := NewWithStore(cfg, store)
	s.owns = true
	return s, nil
}

// NewWithStore builds a server around an existing store, which tests share.
func NewWithStore(cfg Config, store *Store) *Server {
	if cfg.PairTTL <= 0 {
		cfg.PairTTL = 2 * time.Minute
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 12 << 20
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Server{
		cfg:     cfg,
		store:   store,
		logger:  cfg.Logger,
		tunnels: map[string]*Tunnel{},
		pairs:   newRateLimiter(30, time.Minute),
		upgrader: websocket.Upgrader{
			HandshakeTimeout: 15 * time.Second,
			ReadBufferSize:   32 << 10,
			WriteBufferSize:  32 << 10,
			CheckOrigin:      func(*http.Request) bool { return true },
		},
	}
}

// Store exposes the relay's store for CLI and tests.
func (s *Server) Store() *Store { return s.store }

// Handler returns the relay's public HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/tunnel", s.handleTunnel)
	mux.HandleFunc("POST /v1/pair", s.handlePair)
	mux.HandleFunc("POST /v1/push", s.handlePush)
	mux.HandleFunc("/d/", s.handleProxy)
	return mux
}

// ListenAndServe serves the relay on cfg.Addr until Close.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve serves the relay on ln.
func (s *Server) Serve(ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ErrorLog: s.logger}
	return srv.Serve(ln)
}

// Close closes every tunnel and, when New opened it, the store.
func (s *Server) Close() error {
	s.mu.Lock()
	tunnels := make([]*Tunnel, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		tunnels = append(tunnels, t)
	}
	s.tunnels = map[string]*Tunnel{}
	s.mu.Unlock()
	for _, t := range tunnels {
		t.Close()
	}
	if s.owns {
		return s.store.Close()
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "protocol": Protocol})
}

func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	desktopID := strings.TrimSpace(r.Header.Get(HeaderDesktopID))
	secret := bearerToken(r)
	if desktopID == "" || secret == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", ErrUnauthorized)
		return
	}
	desktop, err := s.store.RegisterDesktop(Desktop{
		ID:      desktopID,
		Name:    strings.TrimSpace(r.Header.Get(HeaderDesktopName)),
		Version: "",
	}, HashToken(secret))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(4 << 20)
	session, err := yamux.Server(NewWSConn(conn), yamuxConfig())
	if err != nil {
		_ = conn.Close()
		return
	}
	tunnel := newTunnel(desktop.ID, session, s.logger)
	s.registerTunnel(tunnel)
	s.store.TouchDesktop(desktop.ID)
	s.logger.Printf("relay: desktop %s connected", desktop.ID)

	go s.serveControl(tunnel)

	<-session.CloseChan()
	s.removeTunnel(tunnel)
	tunnel.Close()
	s.logger.Printf("relay: desktop %s disconnected", desktop.ID)
}

func (s *Server) registerTunnel(t *Tunnel) {
	s.mu.Lock()
	old := s.tunnels[t.id]
	s.tunnels[t.id] = t
	s.mu.Unlock()
	if old != nil && old != t {
		old.Close()
	}
}

func (s *Server) removeTunnel(t *Tunnel) {
	s.mu.Lock()
	if s.tunnels[t.id] == t {
		delete(s.tunnels, t.id)
	}
	s.mu.Unlock()
}

func (s *Server) tunnel(id string) (*Tunnel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tunnels[id]
	return t, ok
}

func (s *Server) serveControl(t *Tunnel) {
	srv := &http.Server{Handler: s.controlHandler(t), ErrorLog: s.logger}
	err := srv.Serve(NewYamuxListener(t.session))
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, yamux.ErrSessionShutdown) {
		s.logger.Printf("relay: control %s: %v", t.id, err)
	}
}

func (s *Server) controlHandler(t *Tunnel) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /__relay/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderDesktopID) != t.id {
			writeError(w, http.StatusUnauthorized, "unauthorized", ErrUnauthorized)
			return
		}
		var in struct {
			DeviceID string `json:"device_id"`
		}
		if err := decodeLimited(r, &in, 64<<10); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err)
			return
		}
		if err := s.store.RevokeDevice(t.id, in.DeviceID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /__relay/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderDesktopID) != t.id {
			writeError(w, http.StatusUnauthorized, "unauthorized", ErrUnauthorized)
			return
		}
		var in struct {
			DeviceID string `json:"device_id"`
			Disabled bool   `json:"disabled"`
		}
		if err := decodeLimited(r, &in, 64<<10); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err)
			return
		}
		if strings.TrimSpace(in.DeviceID) == "" {
			writeError(w, http.StatusBadRequest, "bad_request", errors.New("device_id is required"))
			return
		}
		if err := s.store.SetDeviceDisabled(t.id, in.DeviceID, in.Disabled); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "disabled": in.Disabled})
	})
	mux.HandleFunc("GET /__relay/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderDesktopID) != t.id {
			writeError(w, http.StatusUnauthorized, "unauthorized", ErrUnauthorized)
			return
		}
		devices, err := s.store.ListDevices(t.id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
	})
	mux.HandleFunc("POST /__relay/notify", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderDesktopID) != t.id {
			writeError(w, http.StatusUnauthorized, "unauthorized", ErrUnauthorized)
			return
		}
		var n Notification
		if err := decodeLimited(r, &n, 64<<10); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err)
			return
		}
		targets, err := s.store.PushTargets(t.id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err)
			return
		}
		if len(n.DeviceIDs) > 0 {
			want := map[string]bool{}
			for _, id := range n.DeviceIDs {
				want[id] = true
			}
			filtered := targets[:0]
			for _, target := range targets {
				if want[target.DeviceID] {
					filtered = append(filtered, target)
				}
			}
			targets = filtered
		}
		if s.cfg.Pusher != nil {
			if err := s.cfg.Pusher.Push(r.Context(), targets, n); err != nil {
				writeError(w, http.StatusBadGateway, "push_failed", err)
				return
			}
		} else {
			LogPusher{Logger: s.logger}.Push(r.Context(), targets, n)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "targets": len(targets)})
	})
	return mux
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if !s.pairs.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", ErrRateLimited)
		return
	}
	var in struct {
		DesktopID    string `json:"desktop_id"`
		Code         string `json:"code"`
		DeviceName   string `json:"device_name"`
		Platform     string `json:"platform"`
		DevicePubKey string `json:"device_pubkey,omitempty"`
	}
	if err := decodeLimited(r, &in, 64<<10); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err)
		return
	}
	in.DesktopID = strings.TrimSpace(in.DesktopID)
	in.Code = strings.TrimSpace(in.Code)
	if in.DesktopID == "" || in.Code == "" {
		writeError(w, http.StatusBadRequest, "bad_request", errors.New("desktop_id and code are required"))
		return
	}
	tunnel, ok := s.tunnel(in.DesktopID)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "desktop_offline", ErrDesktopOffline)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var out pairResponse
	err := tunnel.controlJSON(ctx, http.MethodPost, "/__relay/pair", in, &out)
	if err != nil {
		status := http.StatusForbidden
		code := "pair_rejected"
		var re *remoteError
		if errors.As(err, &re) {
			code = re.Code
			if re.Status >= 400 {
				status = re.Status
			}
		}
		writeError(w, status, code, err)
		return
	}
	if out.DeviceID == "" || out.Token == "" {
		writeError(w, http.StatusBadGateway, "pair_failed", errors.New("desktop returned an empty device"))
		return
	}
	now := time.Now()
	if err := s.store.AddDevice(Device{
		ID:        out.DeviceID,
		DesktopID: in.DesktopID,
		Name:      firstNonEmpty(out.Name, in.DeviceName),
		Platform:  in.Platform,
		TokenHash: HashToken(out.Token),
		CreatedAt: now,
		LastSeen:  now,
	}); err != nil {
		writeError(w, http.StatusConflict, "device_exists", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"desktop_id":     in.DesktopID,
		"device_id":      out.DeviceID,
		"token":          out.Token,
		"name":           firstNonEmpty(out.Name, in.DeviceName),
		"capabilities":   out.Capabilities,
		"protocol":       Protocol,
		"desktop_pubkey": out.DesktopPubKey,
		"e2e":            out.E2E,
		"desktop_name":   out.DesktopName,
		"platform":       out.Platform,
		"mac":            out.MAC,
		"ip":             out.IP,
	})
}

type pairResponse struct {
	DeviceID      string          `json:"device_id"`
	Token         string          `json:"token"`
	Name          string          `json:"name"`
	Capabilities  map[string]bool `json:"capabilities"`
	DesktopPubKey string          `json:"desktop_pubkey,omitempty"`
	E2E           bool            `json:"e2e,omitempty"`
	DesktopName   string          `json:"desktop_name,omitempty"`
	Platform      string          `json:"platform,omitempty"`
	MAC           string          `json:"mac,omitempty"`
	IP            string          `json:"ip,omitempty"`
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	desktopID := strings.TrimSpace(r.Header.Get(HeaderDesktopID))
	device, err := s.deviceFor(r, desktopID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	var in struct {
		Provider string `json:"provider"`
		Token    string `json:"token"`
		Env      string `json:"env"`
	}
	if err := decodeLimited(r, &in, 64<<10); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err)
		return
	}
	switch in.Provider {
	case "apns", "fcm":
	default:
		writeError(w, http.StatusBadRequest, "bad_request", errors.New("provider must be apns or fcm"))
		return
	}
	if err := s.store.SetPushToken(device.ID, device.DesktopID, in.Provider, strings.TrimSpace(in.Token), in.Env); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/d/")
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		writeError(w, http.StatusNotFound, "not_found", errors.New("unknown relay path"))
		return
	}
	desktopID := rest[:slash]
	path := rest[slash:]
	if path != "/api" && !strings.HasPrefix(path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", errors.New("only /api is exposed"))
		return
	}
	device, err := s.deviceFor(r, desktopID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	tunnel, ok := s.tunnel(desktopID)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "desktop_offline", ErrDesktopOffline)
		return
	}
	r = withDevice(r, device.ID)
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	}
	if websocket.IsWebSocketUpgrade(r) {
		tunnel.proxy.ServeHTTP(w, r)
		return
	}
	timeout := 60 * time.Second
	if strings.Contains(path, "/attachments") {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	tunnel.proxy.ServeHTTP(w, r.WithContext(ctx))
}

func (s *Server) deviceFor(r *http.Request, desktopID string) (*Device, error) {
	if desktopID == "" {
		return nil, ErrUnauthorized
	}
	token := bearerToken(r)
	if token == "" {
		return nil, ErrUnauthorized
	}
	return s.store.DeviceByToken(desktopID, HashToken(token))
}

// writeDeviceError tells a phone apart from a desktop that paused it: a
// disabled device gets a code it can name in its settings.
func writeDeviceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrDeviceDisabled) {
		writeError(w, http.StatusForbidden, "device_disabled", err)
		return
	}
	writeError(w, http.StatusUnauthorized, "unauthorized", err)
}

func decodeLimited(r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, limit))
	return dec.Decode(v)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 30 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}

// Tunnel is one desktop's yamux session and the HTTP proxy bound to it.
type Tunnel struct {
	id      string
	session *yamux.Session
	proxy   *httputil.ReverseProxy
	control *http.Client
	logger  *log.Logger
}

func newTunnel(id string, session *yamux.Session, logger *log.Logger) *Tunnel {
	t := &Tunnel{id: id, session: session, logger: logger}
	transport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return session.OpenStream()
		},
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     false,
	}
	t.proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// The relay's own /d/<id> prefix is removed here; the desktop
			// sees the same /api path a local browser would.
			req.URL.Scheme = "http"
			req.URL.Host = "desktop"
			req.Host = "desktop"
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/d/"+id)
			req.URL.RawPath = ""
			req.Header.Del("Authorization")
			for key := range req.Header {
				if strings.HasPrefix(strings.ToLower(key), "x-chatgo-") {
					delete(req.Header, key)
				}
			}
			if device := req.Context().Value(deviceContextKey{}); device != nil {
				if id, ok := device.(string); ok {
					req.Header.Set(HeaderDeviceID, id)
				}
			}
			req.Header.Set(HeaderRelayDesktop, id)
		},
		Transport:     transport,
		FlushInterval: 50 * time.Millisecond,
		ModifyResponse: func(resp *http.Response) error {
			cookies := resp.Cookies()
			resp.Header.Del("Set-Cookie")
			for _, cookie := range cookies {
				if cookie.Name != "chatgo_web_token" {
					resp.Header.Add("Set-Cookie", cookie.String())
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				writeError(w, 499, "client_closed", err)
				return
			}
			writeError(w, http.StatusBadGateway, "desktop_unavailable", err)
		},
	}
	t.control = &http.Client{
		Transport: &http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				return session.OpenStream()
			},
			ResponseHeaderTimeout: 30 * time.Second,
			ForceAttemptHTTP2:     false,
		},
		Timeout: 30 * time.Second,
	}
	return t
}

func (t *Tunnel) Close() {
	t.session.Close()
}

// deviceContextKey carries the authenticated device id into the proxy
// director without mutating the caller's request headers.
type deviceContextKey struct{}

func (t *Tunnel) controlJSON(ctx context.Context, method, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://relay"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderDesktopID, t.id)
	resp, err := t.control.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e errorBody
		_ = json.Unmarshal(data, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = string(data)
		}
		return &remoteError{Status: resp.StatusCode, Code: firstNonEmpty(e.Error.Code, "remote_error"), Message: msg}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

type remoteError struct {
	Status  int
	Code    string
	Message string
}

func (e *remoteError) Error() string { return e.Message }

// SetDevice stores the authenticated device on the request context so the
// proxy director can attach it to the outbound request.
func withDevice(r *http.Request, deviceID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), deviceContextKey{}, deviceID))
}
