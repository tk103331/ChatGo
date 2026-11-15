package netx

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// A proxy address must name a scheme the app can actually dial through, and
// the empty string passes: it is how the field is cleared.
func TestValidateProxyURL(t *testing.T) {
	ok := []string{"", "http://127.0.0.1:7890", "https://proxy.corp:8443",
		"socks5://127.0.0.1:1080", "socks5h://localhost:1080", "  http://p:1  "}
	for _, raw := range ok {
		if err := ValidateProxyURL(raw); err != nil {
			t.Fatalf("ValidateProxyURL(%q) = %v, want nil", raw, err)
		}
	}
	bad := map[string]string{
		"ftp://p:21":        "协议",
		"proxy.example.com": "协议",
		"http://":           "主机",
		"http:///nohost":    "主机",
		"://broken":         "地址",
	}
	for raw, want := range bad {
		err := ValidateProxyURL(raw)
		if err == nil {
			t.Fatalf("ValidateProxyURL(%q) = nil, want an error", raw)
		}
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateProxyURL(%q) = %q, want it to mention %q", raw, err, want)
		}
	}
}

// SetProxy publishes the address the shared transport dials through, an
// invalid address turns the proxy off, and clearing it restores direct
// connections.
func TestSetProxyDrivesTheSharedTransport(t *testing.T) {
	t.Cleanup(func() { SetProxy("") })

	SetProxy("socks5://127.0.0.1:1080")
	if got := ProxyURL(); got != "socks5://127.0.0.1:1080" {
		t.Fatalf("ProxyURL() = %q", got)
	}
	remote, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	if u, err := ProxyFunc()(remote); err != nil || u == nil || u.String() != "socks5://127.0.0.1:1080" {
		t.Fatalf("a remote target should use the proxy, got %v %v", u, err)
	}

	// Local targets stay direct so the proxy never stands between the app
	// and a local gateway (magpie, Ollama, a locally proxied MCP server…).
	local := []string{
		"http://127.0.0.1:3425/v1", "http://localhost:11434/api/tags",
		"http://[::1]:9000/mcp", "http://[fe80::1]:9000/mcp",
		"http://example.localhost:8080", "http://LOCALHOST:8080",
	}
	for _, target := range local {
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		if !isLocalHost(req.URL.Hostname()) {
			t.Fatalf("%s should count as local", target)
		}
		if u, err := ProxyFunc()(req); err != nil || u != nil {
			t.Fatalf("local target %s should bypass the proxy, got %v %v", target, u, err)
		}
	}

	// A LAN address is not loopback: it goes through the proxy like any
	// remote address, since a proxy is often the route to such hosts.
	req, _ := http.NewRequest(http.MethodGet, "http://192.168.1.5:8080", nil)
	if isLocalHost(req.URL.Hostname()) {
		t.Fatal("a LAN address should not count as local")
	}
	if u, err := ProxyFunc()(req); err != nil || u == nil {
		t.Fatalf("a LAN address should use the proxy, got %v %v", u, err)
	}

	// An invalid address never reaches the transport.
	SetProxy("ftp://127.0.0.1:21")
	if got := ProxyURL(); got != "" {
		t.Fatalf("an invalid proxy address should be ignored, got %q", got)
	}

	// Clearing restores direct connections.
	SetProxy("http://127.0.0.1:7890")
	SetProxy("")
	if got := ProxyURL(); got != "" {
		t.Fatalf("ProxyURL() = %q after clearing", got)
	}
	remote2, _ := http.NewRequest(http.MethodGet, "https://api.example.com", nil)
	if _, err := ProxyFunc()(remote2); err != nil {
		t.Fatalf("a cleared proxy should fall back cleanly: %v", err)
	}
}

// Client and Default build clients on the shared transport, so a later
// SetProxy reaches them too.
func TestClientsUseTheSharedTransport(t *testing.T) {
	t.Cleanup(func() { SetProxy("") })

	c := Client(3 * time.Second)
	if c.Timeout != 3*time.Second {
		t.Fatalf("Client(3s).Timeout = %v", c.Timeout)
	}
	if c.Transport == nil || c.Transport != Default().Transport {
		t.Fatal("clients should share the transport")
	}
}
