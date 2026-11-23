// Package agui is the client side of the AG-UI protocol. It sends a user
// message to an agent's run endpoint and turns the SSE events it streams
// into the text a ChatGo conversation displays.
package agui

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/netx"
)

// Client is one configured remote AG-UI service.
type Client struct {
	conn config.AGUIConnection
	http *http.Client
}

// ChatRequest is one user turn. ThreadID continues an AG-UI thread; empty
// starts one.
type ChatRequest struct {
	ThreadID string
	Text     string
}

// Result is the text a turn produced and the thread it belongs to.
type Result struct {
	Text     string
	ThreadID string
	RunID    string
}

// New returns a client for conn. The HTTP client has no total timeout so a
// long-running agent is not cut off; the caller's context controls it.
func New(conn config.AGUIConnection) (*Client, error) {
	if strings.TrimSpace(conn.URL) == "" {
		return nil, fmt.Errorf("AG-UI 连接 %q 没有 URL", conn.Name)
	}
	return &Client{conn: conn, http: netx.Client(0)}, nil
}

// Ping checks that the endpoint answers HTTP at all. An AG-UI service has no
// discovery document, so this only separates a wrong address from a slow or
// protected one; the first turn is the real test.
func (c *Client) Ping(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, c.conn.URL, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("服务返回 HTTP %d", resp.StatusCode)
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode), nil
}

// Chat sends one user message and streams the agent's text into onChunk.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onChunk func(string)) (Result, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return Result{}, errors.New("消息为空")
	}
	threadID := strings.TrimSpace(req.ThreadID)
	if threadID == "" {
		threadID = "thread-" + randomID()
	}
	runID := "run-" + randomID()
	body, err := json.Marshal(map[string]any{
		"threadId": threadID,
		"runId":    runID,
		"messages": []map[string]any{{
			"id": "msg-" + randomID(), "role": "user", "content": text,
		}},
		"tools":   []any{},
		"context": []any{},
	})
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.conn.URL, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return Result{}, fmt.Errorf("AG-UI 服务返回 HTTP %d：%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return Result{}, fmt.Errorf("AG-UI 服务没有返回事件流：%s", strings.TrimSpace(string(data)))
	}

	res := Result{ThreadID: threadID, RunID: runID}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Delta   string `json:"delta"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "TEXT_MESSAGE_CONTENT":
			if ev.Delta != "" {
				res.Text += ev.Delta
				if onChunk != nil {
					onChunk(ev.Delta)
				}
			}
		case "RUN_ERROR":
			msg := strings.TrimSpace(ev.Message)
			if msg == "" {
				msg = strings.TrimSpace(ev.Error)
			}
			if msg == "" {
				msg = "AG-UI 运行失败"
			}
			return res, errors.New(msg)
		case "RUN_FINISHED":
			return res, nil
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return res, err
	}
	return res, nil
}

func (c *Client) setHeaders(req *http.Request) {
	for k, v := range c.conn.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	if key := strings.TrimSpace(c.conn.APIKey); key != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-ChatGo-Protocol", "agui")
}

func randomID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
