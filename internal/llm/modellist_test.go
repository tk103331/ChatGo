package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"chatgo/internal/config"
)

// listModelsOn points a provider at a test server and fetches its models.
func listModelsOn(t *testing.T, p config.Provider, url string) ([]string, error) {
	t.Helper()
	p.BaseURL = url
	return ListModels(context.Background(), p)
}

// TestListModelsParsesEveryResponseShape covers the three wire formats behind
// the eight provider types: OpenAI's data[].id, Ollama's models[].name and
// Gemini's models[].name under a "models/" prefix.
func TestListModelsParsesEveryResponseShape(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		path string
		body string
		want []string
	}{
		{
			name: "openai",
			typ:  config.ProviderTypeOpenAI,
			path: "/models",
			body: `{"data":[{"id":"gpt-4o"},{"id":"o1-mini"}]}`,
			want: []string{"gpt-4o", "o1-mini"},
		},
		{
			name: "anthropic",
			typ:  config.ProviderTypeAnthropic,
			path: "/v1/models",
			body: `{"data":[{"id":"claude-sonnet-4-5"}],"has_more":false}`,
			want: []string{"claude-sonnet-4-5"},
		},
		{
			name: "ollama",
			typ:  config.ProviderTypeOllama,
			path: "/api/tags",
			body: `{"models":[{"name":"llama3.2:latest","model":"llama3.2"}]}`,
			want: []string{"llama3.2:latest"},
		},
		{
			name: "gemini",
			typ:  config.ProviderTypeGemini,
			path: "/v1beta/models",
			body: `{"models":[{"name":"models/gemini-2.0-flash"}]}`,
			want: []string{"gemini-2.0-flash"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			got, err := listModelsOn(t, config.Provider{Name: tc.name, Type: tc.typ, APIKey: "k", Model: "m"}, srv.URL)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s models: got %v, want %v", tc.name, got, tc.want)
			}
			if gotPath != tc.path {
				t.Fatalf("%s path: got %q, want %q", tc.name, gotPath, tc.path)
			}
		})
	}
}

// TestListModelsSendsTheProtocolCredential pins each protocol's own way of
// carrying the key, verified against the vendored SDKs: a Bearer header for
// OpenAI, X-Api-Key plus a version for Anthropic, x-goog-api-key for Gemini
// and nothing at all for Ollama. Sending the wrong one is a 401 the user
// cannot diagnose.
func TestListModelsSendsTheProtocolCredential(t *testing.T) {
	cases := []struct {
		typ   string
		check func(t *testing.T, r *http.Request)
	}{
		{config.ProviderTypeOpenAI, func(t *testing.T, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Fatalf("Authorization: got %q", got)
			}
		}},
		{config.ProviderTypeAnthropic, func(t *testing.T, r *http.Request) {
			if got := r.Header.Get("X-Api-Key"); got != "secret" {
				t.Fatalf("X-Api-Key: got %q", got)
			}
			if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
				t.Fatalf("anthropic-version: got %q", got)
			}
		}},
		{config.ProviderTypeGemini, func(t *testing.T, r *http.Request) {
			if got := r.Header.Get("x-goog-api-key"); got != "secret" {
				t.Fatalf("x-goog-api-key: got %q", got)
			}
			if r.URL.Query().Get("key") != "" {
				t.Fatal("the key belongs in the header, not the query")
			}
		}},
		{config.ProviderTypeOllama, func(t *testing.T, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("a local endpoint must get no credential, got %q", got)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.check(t, r)
				fmt.Fprint(w, `{"data":[{"id":"m"}],"models":[{"name":"m"}]}`)
			}))
			defer srv.Close()
			if _, err := listModelsOn(t, config.Provider{Name: tc.typ, Type: tc.typ, APIKey: "secret", Model: "m"}, srv.URL); err != nil {
				t.Fatalf("%s: %v", tc.typ, err)
			}
		})
	}
}

// TestListModelsFollowsAnthropicPaging guards the silent-truncation bug: the
// Anthropic models endpoint pages at 20 by default, so a single GET would
// return a fraction of the list and look complete.
func TestListModelsFollowsAnthropicPaging(t *testing.T) {
	var afterIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after_id")
		afterIDs = append(afterIDs, after)
		switch after {
		case "":
			fmt.Fprint(w, `{"data":[{"id":"a"}],"has_more":true,"last_id":"a"}`)
		case "a":
			fmt.Fprint(w, `{"data":[{"id":"b"}],"has_more":true,"last_id":"b"}`)
		default:
			fmt.Fprint(w, `{"data":[{"id":"c"}],"has_more":false,"last_id":"c"}`)
		}
	}))
	defer srv.Close()
	got, err := listModelsOn(t, config.Provider{Name: "c", Type: config.ProviderTypeClaude, APIKey: "k", Model: "m"}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paging should collect every page: got %v, want %v", got, want)
	}
	if len(afterIDs) != 3 {
		t.Fatalf("expected three requests, got %v", afterIDs)
	}
}

// TestListModelsFollowsGeminiPaging is the same guard for nextPageToken.
func TestListModelsFollowsGeminiPaging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("pageToken") {
		case "":
			fmt.Fprint(w, `{"models":[{"name":"models/one"}],"nextPageToken":"t2"}`)
		default:
			fmt.Fprint(w, `{"models":[{"name":"models/two"}]}`)
		}
	}))
	defer srv.Close()
	got, err := listModelsOn(t, config.Provider{Name: "g", Type: config.ProviderTypeGemini, APIKey: "k", Model: "m"}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestListModelsStopsRunawayPaging keeps a server that always says "more"
// from looping forever.
func TestListModelsStopsRunawayPaging(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprintf(w, `{"data":[{"id":"m%d"}],"has_more":true,"last_id":"m%d"}`, calls, calls)
	}))
	defer srv.Close()
	got, err := listModelsOn(t, config.Provider{Name: "c", Type: config.ProviderTypeAnthropic, APIKey: "k", Model: "m"}, srv.URL)
	if err != nil {
		t.Fatalf("hitting the page cap is not an error: %v", err)
	}
	if calls > maxModelPages {
		t.Fatalf("made %d requests, the cap is %d", calls, maxModelPages)
	}
	if len(got) != calls {
		t.Fatalf("got %d models from %d pages", len(got), calls)
	}
}

// TestListModelsReportsUpstreamRejection keeps the provider's own wording, so
// an invalid key says so instead of "request failed".
func TestListModelsReportsUpstreamRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Incorrect API key provided"}}`)
	}))
	defer srv.Close()
	_, err := listModelsOn(t, config.Provider{Name: "线上", Type: config.ProviderTypeOpenAI, APIKey: "bad", Model: "m"}, srv.URL)
	if err == nil {
		t.Fatal("a 401 must be an error")
	}
	if !strings.Contains(err.Error(), "Incorrect API key provided") {
		t.Fatalf("the upstream message should survive: %v", err)
	}
}

// TestListModelsSurvivesBadPayloads keeps malformed or empty responses from
// panicking or inventing models.
func TestListModelsSurvivesBadPayloads(t *testing.T) {
	for _, body := range []string{``, `not json`, `{}`, `{"data":null}`, `{"data":[{"id":""}]}`, `[]`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		got, err := listModelsOn(t, config.Provider{Name: "p", Type: config.ProviderTypeOpenAI, APIKey: "k", Model: "m"}, srv.URL)
		srv.Close()
		if err == nil && len(got) != 0 {
			t.Fatalf("body %q produced models %v", body, got)
		}
		for _, m := range got {
			if strings.TrimSpace(m) == "" {
				t.Fatalf("body %q produced a blank model name", body)
			}
		}
	}
}

// TestListModelsDeduplicatesAndSorts keeps the picker's order stable and free
// of repeats, whatever the upstream returns.
func TestListModelsDeduplicatesAndSorts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"zeta"},{"id":"alpha"},{"id":"zeta"},{"id":"  "}]}`)
	}))
	defer srv.Close()
	got, err := listModelsOn(t, config.Provider{Name: "p", Type: config.ProviderTypeOpenAI, APIKey: "k", Model: "m"}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestSupportsListModelsCoversEveryOfferedType derives its cases from the
// settings dropdown, so a new provider type must declare whether it can list
// models rather than silently reporting that it cannot.
func TestSupportsListModelsCoversEveryOfferedType(t *testing.T) {
	for _, typ := range config.ProviderTypes() {
		p := config.Provider{Name: typ, Type: typ, Model: "m"}
		if !SupportsListModels(p) {
			t.Fatalf("provider type %q should be able to list models", typ)
		}
	}
	// An unknown protocol has no endpoint to ask.
	if SupportsListModels(config.Provider{Type: config.ProviderTypeCustom, APIType: "no-such-protocol"}) {
		t.Fatal("an unknown api_type cannot list models")
	}
}

// TestCustomProviderUsesItsAPITypeEndpoint checks a custom provider asks the
// endpoint of the protocol it declares, not of its own type name.
func TestCustomProviderUsesItsAPITypeEndpoint(t *testing.T) {
	for apiType, wantPath := range map[string]string{
		config.ProviderTypeOpenAI:     "/models",
		config.APITypeOpenAIResponses: "/models",
		config.ProviderTypeClaude:     "/v1/models",
		config.ProviderTypeGemini:     "/v1beta/models",
	} {
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			fmt.Fprint(w, `{"data":[{"id":"m"}],"models":[{"name":"m"}]}`)
		}))
		p := config.Provider{Name: "c", Type: config.ProviderTypeCustom, APIType: apiType, APIKey: "k", Model: "m"}
		if _, err := listModelsOn(t, p, srv.URL); err != nil {
			t.Fatalf("api_type %q: %v", apiType, err)
		}
		srv.Close()
		if gotPath != wantPath {
			t.Fatalf("api_type %q asked %q, want %q", apiType, gotPath, wantPath)
		}
	}
}

// TestMagpieListsWithoutACredential records that the local gateway is asked
// with no key, the way it is configured.
func TestMagpieListsWithoutACredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("magpie needs no credential, sent %q", got)
		}
		enc, _ := json.Marshal(map[string]any{"data": []map[string]string{{"id": "group/joycode"}}})
		w.Write(enc)
	}))
	defer srv.Close()
	got, err := listModelsOn(t, config.Provider{Name: "magpie", Type: config.ProviderTypeMagpie, Model: "m"}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"group/joycode"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
