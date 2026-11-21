package relay

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// PushConfig holds the credentials of the relay's push providers. Empty
// fields disable their provider.
type PushConfig struct {
	APNsKeyPath    string
	APNsKeyID      string
	APNsTeamID     string
	APNsTopic      string
	APNsEnv        string
	FCMAccountPath string
}

// NewPusher builds the pusher the relay uses. It returns nil and no error when
// no provider is configured, so the caller's LogPusher stays in charge.
func NewPusher(cfg PushConfig) (Pusher, error) {
	var pushers []Pusher
	if cfg.APNsKeyPath != "" || cfg.APNsKeyID != "" || cfg.APNsTeamID != "" || cfg.APNsTopic != "" {
		p, err := newAPNsPusher(cfg)
		if err != nil {
			return nil, err
		}
		pushers = append(pushers, p)
	}
	if cfg.FCMAccountPath != "" {
		p, err := newFCMPusher(cfg.FCMAccountPath)
		if err != nil {
			return nil, err
		}
		pushers = append(pushers, p)
	}
	switch len(pushers) {
	case 0:
		return nil, nil
	case 1:
		return pushers[0], nil
	default:
		return multiPusher(pushers), nil
	}
}

type multiPusher []Pusher

func (m multiPusher) Push(ctx context.Context, targets []PushTarget, n Notification) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(m))
	for _, p := range m {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Push(ctx, targets, n); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	var out []string
	for err := range errs {
		out = append(out, err.Error())
	}
	if len(out) > 0 {
		return errors.New(strings.Join(out, "; "))
	}
	return nil
}

func notificationBody(n Notification) string {
	switch n.Kind {
	case "task_done":
		return "自动任务已完成"
	case "attention", "ask":
		return "Agent 需要你的回复"
	case "permission":
		return "Agent 等待权限审批"
	case "room_done":
		return "群聊有新回复"
	default:
		return "ChatGo 有新消息"
	}
}

func base64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// --- APNs ---

type apnsPusher struct {
	key    *ecdsa.PrivateKey
	keyID  string
	teamID string
	topic  string
	host   string
	client *http.Client

	mu     sync.Mutex
	token  string
	issued time.Time
}

func newAPNsPusher(cfg PushConfig) (*apnsPusher, error) {
	if cfg.APNsKeyPath == "" || cfg.APNsKeyID == "" || cfg.APNsTeamID == "" || cfg.APNsTopic == "" {
		return nil, errors.New("push: APNs needs key, key id, team id and topic")
	}
	data, err := os.ReadFile(cfg.APNsKeyPath)
	if err != nil {
		return nil, fmt.Errorf("push: read APNs key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("push: APNs key is not a PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("push: parse APNs key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("push: APNs key is not an ECDSA key")
	}
	host := "https://api.push.apple.com"
	if strings.EqualFold(cfg.APNsEnv, "dev") || strings.EqualFold(cfg.APNsEnv, "development") {
		host = "https://api.sandbox.push.apple.com"
	}
	return &apnsPusher{
		key:    key,
		keyID:  cfg.APNsKeyID,
		teamID: cfg.APNsTeamID,
		topic:  cfg.APNsTopic,
		host:   host,
		client: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (p *apnsPusher) Push(ctx context.Context, targets []PushTarget, n Notification) error {
	var first error
	for _, target := range targets {
		if target.Provider != "apns" {
			continue
		}
		if target.Env == "dev" && p.host == "https://api.push.apple.com" {
			// The target knows its own environment; a token of one environment
			// does not work in the other. Send dev tokens to the sandbox.
			if err := p.pushOne(ctx, "https://api.sandbox.push.apple.com", target.Token, n); err != nil && first == nil {
				first = err
			}
			continue
		}
		if err := p.pushOne(ctx, p.host, target.Token, n); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (p *apnsPusher) pushOne(ctx context.Context, host, token string, n Notification) error {
	jwt, err := p.jwt()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert":     map[string]string{"title": "ChatGo", "body": notificationBody(n)},
			"sound":     "default",
			"thread-id": "chatgo:" + n.Kind + ":" + n.SessionID,
		},
		"v": 1, "kind": n.Kind, "desktop_id": n.DesktopID, "session_id": n.SessionID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+token, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", p.topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("push: APNs %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func (p *apnsPusher) jwt() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Since(p.issued) < 50*time.Minute {
		return p.token, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": p.keyID})
	claims, _ := json.Marshal(map[string]any{"iss": p.teamID, "iat": time.Now().Unix()})
	signing := base64URL(header) + "." + base64URL(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, p.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	p.token = signing + "." + base64URL(sig)
	p.issued = time.Now()
	return p.token, nil
}

// --- FCM ---

type fcmPusher struct {
	project string
	email   string
	key     *rsa.PrivateKey
	client  *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func newFCMPusher(path string) (*fcmPusher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("push: read FCM account: %w", err)
	}
	var account struct {
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &account); err != nil {
		return nil, fmt.Errorf("push: parse FCM account: %w", err)
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return nil, errors.New("push: FCM private key is not a PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("push: parse FCM key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("push: FCM key is not an RSA key")
	}
	return &fcmPusher{
		project: account.ProjectID,
		email:   account.ClientEmail,
		key:     key,
		client:  &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (p *fcmPusher) Push(ctx context.Context, targets []PushTarget, n Notification) error {
	var first error
	for _, target := range targets {
		if target.Provider != "fcm" {
			continue
		}
		if err := p.pushOne(ctx, target.Token, n); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (p *fcmPusher) pushOne(ctx context.Context, token string, n Notification) error {
	access, err := p.accessToken(ctx)
	if err != nil {
		return err
	}
	msg, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": token,
			"notification": map[string]string{
				"title": "ChatGo",
				"body":  notificationBody(n),
			},
			"data": map[string]string{
				"v": "1", "kind": n.Kind, "desktop_id": n.DesktopID, "session_id": n.SessionID,
			},
			"android": map[string]any{
				"priority": "HIGH",
				"notification": map[string]string{
					"channel_id": "chatgo",
				},
			},
		},
	})
	url := "https://fcm.googleapis.com/v1/projects/" + p.project + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("push: FCM %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func (p *fcmPusher) accessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Before(p.expiry.Add(-time.Minute)) {
		return p.token, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{
		"iss":   p.email,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   "https://oauth2.googleapis.com/token",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	signing := base64URL(header) + "." + base64URL(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	assertion := signing + "." + base64URL(sig)
	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + assertion
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("push: FCM token %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("push: FCM returned an empty access token")
	}
	p.token = out.AccessToken
	p.expiry = now.Add(time.Duration(out.ExpiresIn) * time.Second)
	return p.token, nil
}
