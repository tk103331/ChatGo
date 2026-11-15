// Package netx builds the HTTP clients the app's outbound requests share.
//
// A proxy configured in settings is applied to every client built through
// the package, loopback targets always bypass it, and with no proxy
// configured the environment's HTTP_PROXY / HTTPS_PROXY / NO_PROXY still
// apply, as they did before.
package netx

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// proxySchemes are the address kinds a proxy may be given as. socks5h is
// accepted as an alias of socks5: Go's SOCKS support sends the target's
// hostname to the proxy, which is what the h stands for.
var proxySchemes = map[string]bool{
	"http": true, "https": true, "socks5": true, "socks5h": true,
}

// configured holds the proxy address set from the config, nil when the
// proxy is off. Reading it per request keeps a config change effective for
// clients built before it.
var configured atomic.Pointer[url.URL]

// ValidateProxyURL reports why an address cannot serve as the app's proxy.
// The empty string passes: it is how the field is cleared.
func ValidateProxyURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("地址无效：%v", err)
	}
	if !proxySchemes[strings.ToLower(u.Scheme)] {
		return fmt.Errorf("不支持的协议 %q：可用 http://、https://、socks5:// 或 socks5h://", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("缺少主机和端口，例如 http://127.0.0.1:7890")
	}
	return nil
}

// SetProxy applies the configured proxy address to every client built
// through the package. An address that does not validate turns the proxy
// off rather than breaking all egress.
func SetProxy(raw string) {
	configured.Store(nil)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if err := ValidateProxyURL(raw); err != nil {
		return
	}
	if u, err := url.Parse(raw); err == nil {
		configured.Store(u)
	}
}

// ProxyURL returns the configured proxy address, "" when it is off.
func ProxyURL() string {
	if u := configured.Load(); u != nil {
		return u.String()
	}
	return ""
}

// proxyFor keeps local targets out of the proxy and falls back to the
// environment's proxy variables when none is configured.
func proxyFor(req *http.Request) (*url.URL, error) {
	if isLocalHost(req.URL.Hostname()) {
		return nil, nil
	}
	if u := configured.Load(); u != nil {
		return u, nil
	}
	return http.ProxyFromEnvironment(req)
}

// isLocalHost reports whether a target is on this machine, so the proxy
// never stands between the app and a local gateway (magpie, Ollama, a
// locally proxied MCP server…).
func isLocalHost(host string) bool {
	host = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(host), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast())
}

// transport is the single RoundTripper every client of the package shares,
// so SetProxy reaches clients built before it. It starts as a clone of
// http.DefaultTransport, which keeps the platform's own dialing — the
// fetch-based one in the browser build included.
var transport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = proxyFor
	return t
}()

// Transport returns the shared proxy-aware round tripper, for callers that
// build their own client around a redirect policy.
func Transport() http.RoundTripper { return transport }

// ProxyFunc returns the address resolver shared clients use: the configured
// proxy, loopback bypass, or the environment's proxy variables. Exposed for
// dialers that ask for a function instead of a transport, such as a
// websocket gateway.
func ProxyFunc() func(*http.Request) (*url.URL, error) { return proxyFor }

// Default is the proxy-aware replacement for http.DefaultClient: no
// timeout of its own, so callers bound their requests with a context.
func Default() *http.Client { return &http.Client{Transport: transport} }

// Client builds a proxy-aware client with the given timeout. Zero leaves
// the client unbounded, which streaming responses need.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transport}
}
