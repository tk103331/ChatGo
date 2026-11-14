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
	switch {
	case strings.TrimSpace(p.ModelsURL) != "":
		// The provider names its own list endpoint, so the response's shape
		// is the gateway's, not the protocol's; parse what it returned.
		names, err = listModelsFromURL(ctx, p)
	case p.EffectiveAPIType() == config.ProviderTypeAnthropic || p.EffectiveAPIType() == config.ProviderTypeClaude:
		names, err = listAnthropicModels(ctx, p)
	case p.EffectiveAPIType() == config.ProviderTypeOllama:
		names, err = listOllamaModels(ctx, p)
	case p.EffectiveAPIType() == config.ProviderTypeGemini:
		names, err = listGeminiModels(ctx, p)
	default:
		names, err = listOpenAIModels(ctx, p)
	}
	if err != nil {
		return nil, err
	}
	return cleanModelNames(names), nil
}

// listModelsFromURL reads a provider's own model-list endpoint. A gateway
// answers in any of the shapes the ecosystem uses, so the body is probed
// rather than assumed: an array of strings, an array of objects with ids,
// the OpenAI {data:[{id}]}, Anthropic and Gemini shapes among them.
func listModelsFromURL(ctx context.Context, p config.Provider) ([]string, error) {
	endpoint := strings.TrimSpace(p.ModelsURL)
	// A relative URL rides the provider's own base, the way its requests do.
	if !strings.Contains(endpoint, "://") {
		endpoint = joinURL(modelListBaseURL(p), strings.TrimPrefix(endpoint, "/"))
	}
	// The provider's static headers ride along — a gateway's list endpoint
	// often wants the same version or tenant header its chat does — with
	// the credential of the protocol it speaks.
	var body json.RawMessage
	if err := getJSONCustom(ctx, p, endpoint, authModeFor(p.EffectiveAPIType()), &body); err != nil {
		return nil, err
	}
	return modelNamesFromBody(body, p.Name)
}

// authModeFor picks the credential header style of a provider's protocol.
func authModeFor(apiType string) authMode {
	switch apiType {
	case config.ProviderTypeAnthropic, config.ProviderTypeClaude:
		return anthropicAuth
	case config.ProviderTypeGemini:
		return geminiAuth
	case config.ProviderTypeOllama:
		return noAuth
	default:
		return openAIAuth
	}
}

// modelNamesFromBody pulls model names out of a list response in any of the
// shapes seen in the wild, so a plugin's endpoint is not required to mimic a
// particular vendor.
func modelNamesFromBody(data []byte, provider string) ([]string, error) {
	// An array of strings: ["model-a", "model-b"].
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	// An array of objects: [{"id": "…"} or {"name": "…"} or {"model": "…"}].
	var objs []map[string]any
	if err := json.Unmarshal(data, &objs); err == nil {
		return namesFromObjects(objs), nil
	}
	// A field holding either of those: OpenAI's {data: …}, Anthropic's,
	// Ollama's {models: …}, Gemini's {models: [{name: "models/…"}]}, or a
	// custom {models: [{"model": "…"}]}.
	var wrap struct {
		Data   json.RawMessage `json:"data"`
		Models json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("模型「%s」返回的列表无法解析：%s", provider, snippet(data))
	}
	for _, raw := range [][]byte{wrap.Data, wrap.Models} {
		if len(raw) == 0 {
			continue
		}
		if names, err := modelNamesFromBody(raw, provider); err == nil && len(names) > 0 {
			return names, nil
		}
	}
	return nil, fmt.Errorf("模型「%s」返回的列表无法解析：%s", provider, snippet(data))
}

// namesFromObjects reads one name per object, trying each key the ecosystem
// uses, and strips Gemini's "models/" prefix so the id matches what a chat
// request takes.
func namesFromObjects(objs []map[string]any) []string {
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		for _, key := range []string{"id", "name", "model", "slug"} {
			if s, ok := o[key].(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimPrefix(s, "models/"))
				break
			}
		}
	}
	return out
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
	return getJSONCustom(ctx, p, endpoint, mode, out)
}

// getJSONCustom is getJSON with the provider's static headers applied, for
// an endpoint the provider names itself — a plugin-contributed gateway that
// wants its tenant or version header on the model list too.
func getJSONCustom(ctx context.Context, p config.Provider, endpoint string, mode authMode, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("模型「%s」的地址无效：%s", p.Name, firstLine(err.Error()))
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range p.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
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
