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
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Ready        bool            `json:"ready"`
	Error        string          `json:"error,omitempty"`
	Note         string          `json:"note,omitempty"`
	Models       []modelChoice   `json:"models,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
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
	Session     models.Conversation `json:"session"`
	Running     bool                `json:"running"`
	Interaction *interactionInfo    `json:"interaction,omitempty"`
}

type agentInfo struct {
	Ref         string   `json:"ref"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	Icon        string   `json:"icon,omitempty"`
	Models      []string `json:"models,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

type toolInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

type skillInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Token       string `json:"token"`
	Description string `json:"description,omitempty"`
}

type askItem struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

type permissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type permissionRequest struct {
	Title   string             `json:"title"`
	Detail  string             `json:"detail,omitempty"`
	Options []permissionOption `json:"options"`
}

type interactionInfo struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Items      []askItem          `json:"items,omitempty"`
	Permission *permissionRequest `json:"permission,omitempty"`
}

type attachmentRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
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

func apiPatch(path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPatch, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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

// messagePath is a message action: "edit" or "regenerate".
func messagePath(id, messageID, action string) string {
	return sessionPath(id) + "/messages/" + url.PathEscape(messageID) + "/" + action
}

type roomMemberInfo struct {
	Agent         string `json:"agent"`
	Name          string `json:"name,omitempty"`
	Muted         bool   `json:"muted,omitempty"`
	Talkativeness int    `json:"talkativeness,omitempty"`
}

type roomInfo struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Workspace          string           `json:"workspace,omitempty"`
	Activation         string           `json:"activation"`
	Master             string           `json:"master,omitempty"`
	AllowSelfResponses bool             `json:"allow_self_responses,omitempty"`
	Members            []roomMemberInfo `json:"members"`
	Sessions           int              `json:"sessions"`
	Updated            time.Time        `json:"updated,omitempty"`
	Running            bool             `json:"running,omitempty"`
}

type roomInput struct {
	Name               string           `json:"name"`
	Workspace          string           `json:"workspace,omitempty"`
	Activation         string           `json:"activation,omitempty"`
	Master             string           `json:"master,omitempty"`
	AllowSelfResponses bool             `json:"allow_self_responses,omitempty"`
	Members            []roomMemberInfo `json:"members"`
}

type roomSessionInfo struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Updated time.Time `json:"updated"`
}

type scheduleInfo struct {
	Kind    string `json:"kind"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Weekday int    `json:"weekday,omitempty"`
	Day     int    `json:"day,omitempty"`
}

type taskInfo struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Agent     string       `json:"agent,omitempty"`
	Prompt    string       `json:"prompt"`
	Workspace string       `json:"workspace,omitempty"`
	Schedule  scheduleInfo `json:"schedule"`
	Reuse     bool         `json:"reuse,omitempty"`
	ConvID    string       `json:"conversation_id,omitempty"`
	Enabled   bool         `json:"enabled"`
	Running   bool         `json:"running,omitempty"`
	LastRun   time.Time    `json:"last_run,omitempty"`
	LastNote  string       `json:"last_note,omitempty"`
	Sessions  int          `json:"sessions"`
}

type taskInput struct {
	Name      string       `json:"name"`
	Agent     string       `json:"agent,omitempty"`
	Prompt    string       `json:"prompt"`
	Workspace string       `json:"workspace,omitempty"`
	Schedule  scheduleInfo `json:"schedule"`
	Reuse     bool         `json:"reuse,omitempty"`
	Enabled   bool         `json:"enabled"`
}
