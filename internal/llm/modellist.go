package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"chatgo/internal/config"
)

// maxModelPages caps how many pages of a paginated model list are fetched, so
// an upstream that always reports more cannot loop forever.
const maxModelPages = 10

// modelListTimeout bounds the whole listing, pagination included.
const modelListTimeout = 15 * time.Second

// anthropicVersion is the API version the Anthropic endpoints require. It
// matches the value the official SDK sends.
const anthropicVersion = "2023-06-01"

// SupportsListModels reports whether a provider's protocol offers an endpoint
// that enumerates models.
func SupportsListModels(p config.Provider) bool {
	switch p.EffectiveAPIType() {
	case config.ProviderTypeOpenAI, config.ProviderTypeCustom, config.ProviderTypeMagpie,
		config.APITypeOpenAIResponses, config.ProviderTypeDeepSeek, config.ProviderTypeQwen,
		config.ProviderTypeAnthropic, config.ProviderTypeClaude,
		config.ProviderTypeOllama, config.ProviderTypeGemini:
		return true
	}
	return false
}

// ListModels asks the provider which models it serves, following pagination
// where the protocol has it. The names are returned sorted and deduplicated,
// so the caller can show them directly.
func ListModels(ctx context.Context, p config.Provider) ([]string, error) {
	if !SupportsListModels(p) {
		return nil, fmt.Errorf("模型「%s」的类型不支持获取模型列表", p.Name)
	}
	ctx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()

	var names []string
	var err error
	switch p.EffectiveAPIType() {
	case config.ProviderTypeAnthropic, config.ProviderTypeClaude:
		names, err = listAnthropicModels(ctx, p)
	case config.ProviderTypeOllama:
		names, err = listOllamaModels(ctx, p)
	case config.ProviderTypeGemini:
		names, err = listGeminiModels(ctx, p)
	default:
		names, err = listOpenAIModels(ctx, p)
	}
	if err != nil {
		return nil, err
	}
	return cleanModelNames(names), nil
}

// listOpenAIModels reads the OpenAI-compatible GET {base}/models, which every
// OpenAI-style vendor returns in full rather than paginated.
func listOpenAIModels(ctx context.Context, p config.Provider) ([]string, error) {
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	endpoint := joinURL(modelListBaseURL(p), "models")
	if err := getJSON(ctx, p, endpoint, openAIAuth, &body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		names = append(names, m.ID)
	}
	return names, nil
}

// listAnthropicModels reads GET {base}/v1/models, which pages at 20 by
// default: without following has_more the list silently stops short.
func listAnthropicModels(ctx context.Context, p config.Provider) ([]string, error) {
	var names []string
	after := ""
	for page := 0; page < maxModelPages; page++ {
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		endpoint := joinURL(modelListBaseURL(p), "v1/models") + "?limit=1000"
		if after != "" {
			endpoint += "&after_id=" + url.QueryEscape(after)
		}
		if err := getJSON(ctx, p, endpoint, anthropicAuth, &body); err != nil {
			return nil, err
		}
		for _, m := range body.Data {
			names = append(names, m.ID)
		}
		if !body.HasMore || body.LastID == "" {
			break
		}
		after = body.LastID
	}
	return names, nil
}

// listOllamaModels reads GET {base}/api/tags. The entries carry both a name
// and a model field; the name is the tag users pull and run.
func listOllamaModels(ctx context.Context, p config.Provider) ([]string, error) {
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	endpoint := joinURL(modelListBaseURL(p), "api/tags")
	if err := getJSON(ctx, p, endpoint, noAuth, &body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// listGeminiModels reads GET {base}/v1beta/models, following nextPageToken.
// Gemini names every model "models/<id>", which is stripped so the id matches
// what the chat request takes.
func listGeminiModels(ctx context.Context, p config.Provider) ([]string, error) {
	var names []string
	token := ""
	for page := 0; page < maxModelPages; page++ {
		var body struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		endpoint := joinURL(modelListBaseURL(p), "v1beta/models")
		if token != "" {
			endpoint += "?pageToken=" + url.QueryEscape(token)
		}
		if err := getJSON(ctx, p, endpoint, geminiAuth, &body); err != nil {
			return nil, err
		}
		for _, m := range body.Models {
			names = append(names, strings.TrimPrefix(m.Name, "models/"))
		}
		if body.NextPageToken == "" {
			break
		}
		token = body.NextPageToken
	}
	return names, nil
}

// authMode names how a protocol carries its credential.
type authMode int

const (
	noAuth authMode = iota
	openAIAuth
	anthropicAuth
	geminiAuth
)

// getJSON performs the GET and decodes it, turning a refusal into an error
// that keeps the provider's own wording.
func getJSON(ctx context.Context, p config.Provider, endpoint string, mode authMode, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("模型「%s」的地址无效：%s", p.Name, firstLine(err.Error()))
	}
	req.Header.Set("Accept", "application/json")
	key := strings.TrimSpace(p.APIKey)
	switch mode {
	case openAIAuth:
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	case anthropicAuth:
		if key != "" {
			req.Header.Set("X-Api-Key", key)
		}
		req.Header.Set("anthropic-version", anthropicVersion)
	case geminiAuth:
		if key != "" {
			req.Header.Set("x-goog-api-key", key)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 %s：%s", endpoint, firstLine(err.Error()))
	}
	defer resp.Body.Close()
	// The body is read with a cap so a misconfigured endpoint streaming
	// something large cannot exhaust memory.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("读取 %s 的响应失败：%s", p.Name, firstLine(err.Error()))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("模型「%s」获取列表失败（HTTP %d）：%s", p.Name, resp.StatusCode, snippet(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("模型「%s」返回的列表无法解析：%s", p.Name, snippet(data))
	}
	return nil
}

// modelListBaseURL returns the host to ask, falling back to the protocol's
// official address when the provider configured none.
func modelListBaseURL(p config.Provider) string {
	if base := strings.TrimSpace(p.BaseURL); base != "" {
		return base
	}
	switch p.EffectiveAPIType() {
	case config.ProviderTypeAnthropic, config.ProviderTypeClaude:
		return "https://api.anthropic.com"
	case config.ProviderTypeOllama:
		return "http://localhost:11434"
	case config.ProviderTypeGemini:
		return "https://generativelanguage.googleapis.com"
	case config.ProviderTypeMagpie:
		return config.MagpieBaseURL
	case config.ProviderTypeDeepSeek:
		return "https://api.deepseek.com/v1"
	case config.ProviderTypeQwen:
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

// joinURL appends a path to a base URL. A base that already ends in the
// path's first segment is left alone, so a base of ".../v1" does not become
// ".../v1/v1/models".
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	seg := strings.SplitN(path, "/", 2)
	if len(seg) == 2 && strings.HasSuffix(base, "/"+seg[0]) {
		return base + "/" + seg[1]
	}
	return base + "/" + path
}

// cleanModelNames drops blanks and repeats and sorts what is left, so the
// picker shows a stable list.
func cleanModelNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// snippet keeps an upstream payload short enough for a status line while
// still showing what the vendor said.
func snippet(data []byte) string {
	s := firstLine(string(data))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "响应为空"
	}
	return s
}
