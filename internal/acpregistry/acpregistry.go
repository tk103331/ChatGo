// Package acpregistry reads the official ACP Registry
// (https://github.com/agentclientprotocol/registry), the curated index of
// agents that speak the Agent Client Protocol. ChatGo turns an entry into an
// ACP connection: an npx or uvx distribution maps to the command, arguments
// and environment a connection already stores, while a binary-only entry is
// listed but not offered, because it needs a download-and-extract step the
// first integration does not carry.
package acpregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"chatgo/internal/netx"
)

// IndexURL is the published registry index. The registry updates itself
// hourly, so a plain fetch is always current enough.
const IndexURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

// darwinArm64Key is the index's platform key for macOS Apple Silicon. The
// published JSON names it with this literal string rather than
// "darwin-aarch64" like every other platform, so the mapping must match it
// exactly as written.
const darwinArm64Key = "darwin-aarch64"

// maxIndexBytes bounds the index download; the real file is a few hundred
// kilobytes.
const maxIndexBytes = 8 << 20

// Registry is the index document.
type Registry struct {
	Version string  `json:"version"`
	Agents  []Agent `json:"agents"`
}

// Agent is one registry entry.
type Agent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Repository   string       `json:"repository"`
	Website      string       `json:"website"`
	Authors      []string     `json:"authors"`
	License      string       `json:"license"`
	LicenseURL   string       `json:"license_url"`
	Icon         string       `json:"icon"`
	Distribution Distribution `json:"distribution"`
}

// Distribution is how an agent runs: an npm package, a PyPI package, or a
// platform-specific binary archive. An entry may offer more than one.
type Distribution struct {
	Npx    *NpxDist              `json:"npx"`
	Uvx    *UvxDist              `json:"uvx"`
	Binary map[string]BinaryDist `json:"binary"`
}

// NpxDist runs the agent through npx. Package carries the pinned version the
// registry verified, as in "@scope/name@1.2.0".
type NpxDist struct {
	Package string            `json:"package"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// UvxDist runs the agent through uvx. Package carries the pinned version as
// in "name==1.2.0".
type UvxDist struct {
	Package string            `json:"package"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// BinaryDist is one platform's downloadable archive. Not installed by this
// integration; kept so the list can name what an entry needs.
type BinaryDist struct {
	Archive string            `json:"archive"`
	SHA256  string            `json:"sha256"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Fetch downloads and parses the registry index, through the shared
// proxy-aware HTTP client.
func Fetch(ctx context.Context) (Registry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, IndexURL, nil)
	if err != nil {
		return Registry{}, err
	}
	resp, err := netx.Default().Do(req)
	if err != nil {
		return Registry{}, fmt.Errorf("无法访问 ACP Registry：%v", firstLine(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Registry{}, fmt.Errorf("ACP Registry 返回 HTTP %d", resp.StatusCode)
	}
	return Parse(io.LimitReader(resp.Body, maxIndexBytes))
}

// Parse reads the index, keeping only entries with a name and a runtime
// distribution, sorted by name so the dialog lists them in a stable order.
func Parse(r io.Reader) (Registry, error) {
	var reg Registry
	if err := json.NewDecoder(r).Decode(&reg); err != nil {
		return Registry{}, fmt.Errorf("解析 ACP Registry 失败：%v", err)
	}
	out := reg.Agents[:0]
	for _, a := range reg.Agents {
		if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.ID) == "" {
			continue
		}
		// An entry with no distribution at all has nothing to show or run;
		// a binary-only one stays, listed but not installable.
		if a.Distribution.Npx == nil && a.Distribution.Uvx == nil &&
			len(a.Distribution.Binary) == 0 {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	reg.Agents = out
	return reg, nil
}

// SupportsRuntime reports whether the entry can be added as a connection:
// it must offer an npx or uvx distribution. A binary-only entry is shown in
// the list but not installable.
func (a Agent) SupportsRuntime() bool {
	return a.Distribution.Npx != nil || a.Distribution.Uvx != nil
}

// SupportsBinary reports whether the entry ships an archive for the platform.
func (a Agent) SupportsBinary(goos, goarch string) bool {
	_, _, ok := a.BinaryTarget(goos, goarch)
	return ok
}

// BinaryOnly reports whether the entry offers nothing but binary archives.
func (a Agent) BinaryOnly() bool {
	return !a.SupportsRuntime() && len(a.Distribution.Binary) > 0
}

// Command is the runtime an entry's distribution maps to, with the args and
// env of a connection. ok is false for a binary-only entry.
//
// An npx package keeps the registry's pinned version, so the connection runs
// the verified release rather than whatever happens to be newest; the -y
// flag is ChatGo's own, so a first run never stops at npx's install prompt.
func (a Agent) Command() (command string, args []string, env map[string]string, ok bool) {
	if d := a.Distribution.Npx; d != nil && strings.TrimSpace(d.Package) != "" {
		rest := append([]string(nil), d.Args...)
		return "npx", append([]string{"-y", d.Package}, rest...), d.Env, true
	}
	if d := a.Distribution.Uvx; d != nil && strings.TrimSpace(d.Package) != "" {
		rest := append([]string(nil), d.Args...)
		return "uvx", append([]string{d.Package}, rest...), d.Env, true
	}
	return "", nil, nil, false
}

// PkgName strips the pinned version from a distribution package spec:
// "@scope/name@1.2.0" and "name==1.2.0" both reduce to their bare package
// name, so an existing connection added from an older pin still counts as
// the same agent.
func PkgName(pkg string) string {
	// uvx pins with ==.
	if i := strings.Index(pkg, "=="); i >= 0 {
		return pkg[:i]
	}
	// npm pins with a trailing @version; a scoped name's leading @ is not
	// one, so only the last @ whose tail starts with a digit is a version.
	if i := strings.LastIndex(pkg, "@"); i > 0 && versionLike(pkg[i+1:]) {
		return pkg[:i]
	}
	return pkg
}

// versionLike reports whether s reads as a version pin: a digit first, as
// every semver starts with one.
func versionLike(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// PlatformKey maps GOOS/GOARCH onto the index's binary platform keys. Used
// only to describe an entry, not to install one.
func PlatformKey(goos, goarch string) string {
	switch goos {
	case "darwin":
		if goarch == "amd64" {
			return "darwin-x86_64"
		}
		return darwinArm64Key
	case "linux":
		arch := "x86_64"
		if goarch == "arm64" {
			arch = "aarch64"
		}
		// The glibc build is the default target; the host's libc decides
		// whether the -musl variant is the right one.
		return "linux-" + arch
	case "windows":
		if goarch == "arm64" {
			return "windows-aarch64"
		}
		return "windows-x86_64"
	}
	return ""
}

// PlatformLabel names a binary platform key the way the details view reads
// it: the index's own key otherwise.
func PlatformLabel(key string) string {
	switch key {
	case darwinArm64Key:
		return "macOS · Apple Silicon"
	case "darwin-x86_64":
		return "macOS · Intel"
	case "linux-aarch64":
		return "Linux · ARM64"
	case "linux-aarch64-musl":
		return "Linux · ARM64 (musl)"
	case "linux-x86_64":
		return "Linux · x86_64"
	case "linux-x86_64-musl":
		return "Linux · x86_64 (musl)"
	case "windows-aarch64":
		return "Windows · ARM64"
	case "windows-x86_64":
		return "Windows · x86_64"
	}
	return key
}

// BinaryPlatforms lists the entry's binary archive targets, in the index's
// own order, named for display.
func (a Agent) BinaryPlatforms() []string {
	keys := make([]string, 0, len(a.Distribution.Binary))
	for k := range a.Distribution.Binary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, PlatformLabel(k))
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
