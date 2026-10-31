package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"chatgo/pkg/models"
)

type modelChoice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type metaInfo struct {
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	Ready    bool          `json:"ready"`
	Error    string        `json:"error,omitempty"`
	Note     string        `json:"note,omitempty"`
	Models   []modelChoice `json:"models,omitempty"`
}

type sessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Workspace string    `json:"workspace,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  int       `json:"messages"`
	Running   bool      `json:"running"`
}

type sessionsResponse struct {
	Sessions []sessionSummary `json:"sessions"`
}

type sessionResponse struct {
	Session models.Conversation `json:"session"`
	Running bool                `json:"running"`
}

type contextItem struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

type contextResponse struct {
	Workspace string        `json:"workspace,omitempty"`
	Items     []contextItem `json:"items"`
}

var webClient = &http.Client{Timeout: 30 * time.Second}

func apiGet(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return apiDo(req, out)
}

func apiPost(path string, body any, out any) error {
	var buf io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, path, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return apiDo(req, out)
}

func apiDelete(path string, out any) error {
	req, err := http.NewRequest(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return apiDo(req, out)
}

func apiDo(req *http.Request, out any) error {
	resp, err := webClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func sessionPath(id string) string {
	return "/api/sessions/" + url.PathEscape(id)
}
