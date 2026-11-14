package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/config"
)

func TestACPExtractVersion(t *testing.T) {
	cases := []struct{ out, want string }{
		{"0.16.2", "0.16.2"},
		{"claude-code-acp 0.16.2 (agent: claude 1.0.100)", "0.16.2"},
		{"OpenCode v1.19.5", "1.19.5"},
		{"gemini-cli\n0.68.0\n", "0.68.0"},
		{"goose, version 1.0.2 (built 2024-01-01)", "1.0.2"},
		{"2026.03.1", "2026.03.1"},
		// npm's own chatter is skipped, however version-bearing it is.
		{"npm notice Newer version of npm available: 10.9.0\n0.16.2", "0.16.2"},
		{"npm@10.9.0 /Users/x\n2.1.0", "2.1.0"},
		{"no version here", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := acpExtractVersion(tc.out); got != tc.want {
			t.Errorf("acpExtractVersion(%q) = %q, want %q", tc.out, got, tc.want)
		}
	}
}

func TestACPSemverCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.16.2", "0.16.2", 0},
		{"0.16.2", "0.17.0", -1},
		{"1.0.0", "0.9.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0-beta", "1.0.0", 0},
		{"2026.03.1", "2026.02.9", 1},
		{"", "1.0.0", -1},
	}
	for _, tc := range cases {
		if got := acpSemverCompare(tc.a, tc.b); got != tc.want {
			t.Errorf("acpSemverCompare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestACPSourceFor(t *testing.T) {
	// A catalog name maps to its feed, with its upgrade script kept.
	s := acpSourceFor(config.ACPConnection{Name: "GeminiCLI", Command: "gemini"})
	if s == nil || s.Kind != acpUpdNPM || s.Package != "@google/gemini-cli" {
		t.Fatalf("GeminiCLI source = %+v", s)
	}
	if !strings.Contains(s.Upgrade, "@google/gemini-cli@latest") {
		t.Fatalf("GeminiCLI upgrade = %q", s.Upgrade)
	}
	// A GitHub feed carries no upgrade script.
	s = acpSourceFor(config.ACPConnection{Name: "Goose", Command: "goose"})
	if s == nil || s.Kind != acpUpdGitHub || s.Repo != "aaif-goose/goose" || s.Upgrade != "" {
		t.Fatalf("Goose source = %+v", s)
	}
	// A hand-added npx command falls back to the package it pulls; with no
	// known install script the check stays read-only.
	s = acpSourceFor(config.ACPConnection{Name: "Custom", Command: "npx",
		Args: []string{"-y", "@acme/acp-adapter"}})
	if s == nil || s.Package != "@acme/acp-adapter" || s.Upgrade != "" {
		t.Fatalf("npx fallback source = %+v", s)
	}
	// An unknown command has no feed at all.
	if acpSourceFor(config.ACPConnection{Name: "Custom", Command: "agent", Args: []string{"acp"}}) != nil {
		t.Fatal("unknown command should have no source")
	}
}

func TestACPNpxPackage(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-y", "@zed-industries/claude-code-acp"}, "@zed-industries/claude-code-acp"},
		{[]string{"--yes", "pi-acp"}, "pi-acp"},
		{[]string{}, ""},
		{[]string{"-y"}, ""},
	}
	for _, tc := range cases {
		if got := acpNpxPackage(tc.args); got != tc.want {
			t.Errorf("acpNpxPackage(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestACPUpgradeScript(t *testing.T) {
	if got := acpUpgradeScript(config.ACPConnection{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}}); got != "opencode upgrade" {
		t.Fatalf("OpenCode upgrade script = %q", got)
	}
	// The check stays possible without an upgrade script.
	if got := acpUpgradeScript(config.ACPConnection{Name: "ClaudeCode", Command: "npx", Args: []string{"-y", "@zed-industries/claude-code-acp"}}); got != "" {
		t.Fatalf("ClaudeCode upgrade script = %q, want empty", got)
	}
	if got := acpUpgradeScript(config.ACPConnection{Name: "Custom", Command: "agent"}); got != "" {
		t.Fatalf("unknown upgrade script = %q, want empty", got)
	}
}

func TestACPUpdStateNewer(t *testing.T) {
	cases := []struct {
		installed, latest string
		want              bool
	}{
		{"0.16.2", "0.17.0", true},
		{"0.17.0", "0.17.0", false},
		{"0.17.0", "0.16.2", false},
		// Without an installed version there is nothing to compare against.
		{"", "0.17.0", false},
		{"0.16.2", "", false},
	}
	for _, tc := range cases {
		st := &acpUpdState{installed: tc.installed, latest: tc.latest, known: true}
		if got := st.newer(); got != tc.want {
			t.Errorf("newer(%q, %q) = %v, want %v", tc.installed, tc.latest, got, tc.want)
		}
	}
}

func TestACPTailOutput(t *testing.T) {
	if got := acpTailOutput("hello\nworld\n"); got != "hello\nworld" {
		t.Fatalf("short tail = %q", got)
	}
	long := strings.Repeat("line\n", 1000)
	got := acpTailOutput(long)
	if len(got) > acpUpdLogCap {
		t.Fatalf("tail kept %d bytes", len(got))
	}
	if !strings.HasPrefix(got, "line\n") {
		t.Fatalf("tail starts mid-line: %q", got[:20])
	}
}

func TestACPUpdHeadline(t *testing.T) {
	if got := acpUpdHeadline(nil); got != "点击检查更新" {
		t.Fatalf("nil headline = %q", got)
	}
	if got := acpUpdHeadline(&acpUpdState{checking: true}); got != "正在检查…" {
		t.Fatalf("checking headline = %q", got)
	}
	if got := acpUpdHeadline(&acpUpdState{upgrading: true, checked: true}); got != "正在升级…" {
		t.Fatalf("upgrading headline = %q", got)
	}
	newer := &acpUpdState{checked: true, known: true, installed: "0.16.2", latest: "0.17.0"}
	if got := acpUpdHeadline(newer); got != "发现新版本 0.16.2 → 0.17.0" {
		t.Fatalf("newer headline = %q", got)
	}
	if got := acpUpdHeadline(&acpUpdState{checked: true, installed: "0.17.0"}); got != "已安装 0.17.0" {
		t.Fatalf("installed headline = %q", got)
	}
}

func TestACPUpdErr(t *testing.T) {
	inst := error(nil)
	lat := error(nil)
	if acpUpdErr(inst, lat) != "" {
		t.Fatal("no errors should give no message")
	}
	if got := acpUpdErr(instErr("未找到命令 x"), nil); got != "未找到命令 x" {
		t.Fatalf("installed error = %q", got)
	}
	// The newest-release failure wins when both sides fail.
	if got := acpUpdErr(instErr("无法获取版本"), instErr("请求失败（HTTP 404）")); got != "最新版本获取失败：请求失败（HTTP 404）" {
		t.Fatalf("combined error = %q", got)
	}
}

type instErr string

func (e instErr) Error() string { return string(e) }

// acpVersionStub writes a fake CLI answering --version and puts its
// directory on PATH, the way the rtk stubs do.
func acpVersionStub(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

// A plain CLI is probed with --version only: stored args such as "acp" would
// start a session instead, so the probe ignores them. The resolved path and
// the version travel back together.
func TestACPProbeVersionPlain(t *testing.T) {
	path := acpVersionStub(t, "fakecli", "case \"$1\" in\n"+
		"--version) echo 'fake 1.2.3' ;;\n"+
		"*) echo 'started a session'; sleep 30 ;;\n"+
		"esac\n")
	res, err := acpProbeVersion(config.ACPConnection{Name: "Fake", Command: path, Args: []string{"acp"}})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if res.Version != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", res.Version)
	}
	if res.Path != path {
		t.Fatalf("resolved path = %q, want %q", res.Path, path)
	}
	if res.Package != "" {
		t.Fatalf("package = %q, want empty", res.Package)
	}
}

// An npx command is probed with its package argument, so the adapter itself
// answers rather than npx, and the package is named in the result.
func TestACPProbeVersionNpx(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	path := filepath.Join(dir, "npx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + log + "\n" +
		"case \"$3\" in --version) echo 'acp-adapter 3.2.1' ;; esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	conn := config.ACPConnection{Name: "NpxFake", Command: "npx", Args: []string{"-y", "acp-adapter"}}
	res, err := acpProbeVersion(conn)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if res.Version != "3.2.1" {
		t.Fatalf("version = %q, want 3.2.1", res.Version)
	}
	if res.Package != "acp-adapter" {
		t.Fatalf("package = %q, want acp-adapter", res.Package)
	}
	if res.Path != path {
		t.Fatalf("resolved path = %q, want %q", res.Path, path)
	}
	args, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(args)) != "-y acp-adapter --version" {
		t.Fatalf("probed with args %q", args)
	}
}

// npm's own chatter carries version numbers of its own: it is filtered so a
// notice about a newer npm is never read as the agent's version. The answer
// is taken from stdout even when the CLI exits non-zero.
func TestACPProbeVersionIgnoresNPMNoise(t *testing.T) {
	path := acpVersionStub(t, "fakecli", "echo 'npm warn exec The following package was not found' >&2\n"+
		"echo 'npm notice Newer version of npm available: 10.9.0' >&2\n"+
		"echo 'adapter 0.16.2'\n"+
		"exit 1\n")
	res, err := acpProbeVersion(config.ACPConnection{Name: "Fake", Command: path})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if res.Version != "0.16.2" {
		t.Fatalf("version = %q, want the adapter's 0.16.2, not npm's 10.9.0", res.Version)
	}
}

// A CLI that does not know --version but answers the version subcommand gets
// that as a second try.
func TestACPProbeVersionSubcommandFallback(t *testing.T) {
	log := ""
	path := acpVersionStub(t, "fakecli", "log="+log+"\n"+
		"case \"$*\" in\n"+
		"--version) echo 'unknown flag' >&2; exit 2 ;;\n"+
		"version) echo 'fake 4.5.6' ;;\n"+
		"esac\n")
	res, err := acpProbeVersion(config.ACPConnection{Name: "Fake", Command: path})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if res.Version != "4.5.6" {
		t.Fatalf("version = %q, want 4.5.6 from the version subcommand", res.Version)
	}
}

// An npm-installed CLI answers from its package.json without being run at
// all — magpie's way: the binary is a symlink into node_modules, the shape
// npm -g links them with, and the script would print a wrong version (or
// worse, start something) if it were executed.
func TestACPProbeVersionReadsPackageJSON(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "node_modules", "fake-acp")
	bin := filepath.Join(pkgDir, "bin", "fake-acp")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'wrong 9.9.9'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"),
		[]byte(`{"name":"fake-acp","version":"1.4.2"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "fake-acp")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	res, err := acpProbeVersion(config.ACPConnection{Name: "Fake", Command: "fake-acp"})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if res.Version != "1.4.2" {
		t.Fatalf("version = %q, want the package.json's 1.4.2, not the script's 9.9.9", res.Version)
	}
	if res.Path != link {
		t.Fatalf("resolved path = %q, want %q", res.Path, link)
	}
}

// A scoped package sits one level deeper, under node_modules/@scope/pkg,
// and its package.json answers the same way.
func TestACPPackageVersionScoped(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "node_modules", "@zed-industries", "fake-acp")
	bin := filepath.Join(pkgDir, "bin", "cli.js")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("/* would run */"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"),
		[]byte(`{"name":"@zed-industries/fake-acp","version":"2.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := acpPackageVersion(bin); got != "2.0.0" {
		t.Fatalf("scoped package version = %q, want 2.0.0", got)
	}
	// A file outside any package reads nothing, so the probe falls back to
	// running the CLI.
	if got := acpPackageVersion(filepath.Join(root, "loose-file")); got != "" {
		t.Fatalf("a loose file has no package version, got %q", got)
	}
}

// An unknown command is reported with the name, not a bare failure.
func TestACPProbeVersionMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	res, err := acpProbeVersion(config.ACPConnection{Name: "Missing", Command: "no-such-cli-xyz"})
	if err == nil || !strings.Contains(err.Error(), "no-such-cli-xyz") {
		t.Fatalf("error = %v, want the command name", err)
	}
	if res.Path != "no-such-cli-xyz" {
		t.Fatalf("path = %q, want the raw name", res.Path)
	}
}

// The upgrade channel: an npm-channel script only runs when the resolved
// command lives in npm's own global bin directory.
func TestACPChannelOK(t *testing.T) {
	restore := acpNPMGlobalPrefix
	defer func() { acpNPMGlobalPrefix = restore }()
	acpNPMGlobalPrefix = func() (string, error) { return "/opt/npm", nil }

	if !acpChannelOK("/opt/npm/bin/gemini") {
		t.Fatal("a command in npm's global bin should pass the channel check")
	}
	if !acpChannelOK("/opt/npm/bin/gemini.cmd") {
		t.Fatal("the bin itself is fine too")
	}
	if acpChannelOK("/opt/homebrew/bin/gemini") {
		t.Fatal("a brew-installed command must not pass an npm-channel check")
	}
	// A symlinked command still resolves through the comparison of its path.
	if acpChannelOK("/opt/npm/bingemini") {
		t.Fatal("a sibling directory is not npm's bin")
	}
	// When npm cannot answer, the check stays optimistic.
	acpNPMGlobalPrefix = func() (string, error) { return "", instErr("no npm") }
	if !acpChannelOK("/opt/homebrew/bin/gemini") {
		t.Fatal("an unknown channel must not block the upgrade")
	}
}

// The npx cache names what a cached run would use: the package's cached
// copies, newest first, read straight out of the cache's package manifests.
func TestACPNpxCacheVersions(t *testing.T) {
	cache := t.TempDir()
	write := func(hash, pkg, version string) {
		t.Helper()
		dir := filepath.Join(cache, hash, "node_modules", filepath.FromSlash(pkg))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := `{"name":"` + pkg + `","version":"` + version + `"}`
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("h1", "acp", "0.16.1")
	write("h2", "acp", "0.16.2")
	write("h3", "acp", "0.16.2") // a duplicate copy counts once
	write("h4", "other", "9.9.9")

	if got := acpNpxCacheVersions(cache, "acp"); len(got) != 2 || got[0] != "0.16.2" || got[1] != "0.16.1" {
		t.Fatalf("cache versions = %v, want [0.16.2 0.16.1] newest first", got)
	}
	if got := acpNpxCacheVersions(cache, "missing"); got != nil {
		t.Fatalf("missing package gave %v", got)
	}
	if got := acpNpxCacheVersions(cache, ""); got != nil {
		t.Fatalf("empty package gave %v", got)
	}
}

// The npm-channel upgrade scripts are marked as such; self-updaters and
// check-only sources are not.
func TestACPUpgradeChannels(t *testing.T) {
	if acpUpgradeChannel(config.ACPConnection{Name: "GeminiCLI"}) != acpUpdChannelNPM {
		t.Fatal("GeminiCLI's npm install -g script must carry the npm channel")
	}
	if acpUpgradeChannel(config.ACPConnection{Name: "OpenCode"}) != "" {
		t.Fatal("opencode upgrade is a self-updater with no assumed channel")
	}
	if acpUpgradeChannel(config.ACPConnection{Name: "ClaudeCode"}) != "" {
		t.Fatal("check-only npx sources have no channel")
	}
}
