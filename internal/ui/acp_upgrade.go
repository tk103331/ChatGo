package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/shellenv"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	mgo "github.com/egoist/mygo/ui"
)

// How the newest release of an ACP agent's CLI is located.
const (
	acpUpdNPM    = "npm"    // the package's latest tag on the npm registry
	acpUpdGitHub = "github" // the repository's latest published release
)

// acpUpdChannelNPM marks an upgrade script that runs npm install -g: it only
// updates a CLI that npm itself installed globally.
const acpUpdChannelNPM = "npm"

const (
	acpVersionTimeout = 20 * time.Second
	acpLatestTimeout  = 15 * time.Second
	acpUpgradeTimeout = 15 * time.Minute
	// acpUpdLogCap bounds the upgrade output kept for the settings row.
	acpUpdLogCap = 2000
)

// acpUpgradeSource tells where the newest release of a connection's CLI comes
// from and how to install it.
type acpUpgradeSource struct {
	Kind string
	// Package is the npm package name, used when Kind is acpUpdNPM.
	Package string
	// Repo is an "owner/name" GitHub repository, used when Kind is
	// acpUpdGitHub.
	Repo string
	// Upgrade is the shell script installing the newest release. Empty marks
	// a check-only agent: versions are reported but no upgrade button shows.
	Upgrade string
	// Channel names the install channel the Upgrade script assumes: "npm"
	// for npm install -g scripts, empty for self-updaters and check-only
	// agents. An npm script cannot update a CLI that came from somewhere
	// else, so the picker checks the resolved path against npm's prefix.
	Channel string
}

// acpUpgradeSources maps the connections shipped in acpCatalog to their
// release feed. Adapters behind npx are left without an Upgrade script: npx
// resolves the package's latest version on every run, so there is nothing to
// install by hand. A connection added by hand falls back to reading the npm
// package its npx command pulls.
var acpUpgradeSources = map[string]acpUpgradeSource{
	"ClaudeCode":      {Kind: acpUpdNPM, Package: "@zed-industries/claude-code-acp"},
	"ClaudeAgentSDK":  {Kind: acpUpdNPM, Package: "@zed-industries/claude-agent-acp"},
	"OpenCode":        {Kind: acpUpdNPM, Package: "opencode-ai", Upgrade: "opencode upgrade"},
	"CodexCLI":        {Kind: acpUpdNPM, Package: "@agentclientprotocol/codex-acp"},
	"GeminiCLI":       {Kind: acpUpdNPM, Package: "@google/gemini-cli", Upgrade: "npm install -g @google/gemini-cli@latest", Channel: acpUpdChannelNPM},
	"GitHubCopilot":   {Kind: acpUpdNPM, Package: "@github/copilot", Upgrade: "npm install -g @github/copilot@latest", Channel: acpUpdChannelNPM},
	"QwenCode":        {Kind: acpUpdNPM, Package: "@qwen-code/qwen-code", Upgrade: "npm install -g @qwen-code/qwen-code@latest", Channel: acpUpdChannelNPM},
	"Cline":           {Kind: acpUpdNPM, Package: "cline"},
	"PiAgent":         {Kind: acpUpdNPM, Package: "pi-acp"},
	"DeepSeekHarness": {Kind: acpUpdNPM, Package: "deepseek-acp"},
	"Goose":           {Kind: acpUpdGitHub, Repo: "aaif-goose/goose"},
	"KimiCLI":         {Kind: acpUpdGitHub, Repo: "MoonshotAI/kimi-cli"},
	"Dockercagent":    {Kind: acpUpdGitHub, Repo: "docker/docker-agent"},
	"OpenHands":       {Kind: acpUpdGitHub, Repo: "OpenHands/OpenHands"},
}

// acpSourceFor picks the release feed of a connection: the catalog table
// first, then the npm package an npx command pulls. A nil result means the
// CLI's release feed is unknown and only the installed version can be probed.
func acpSourceFor(conn config.ACPConnection) *acpUpgradeSource {
	if s, ok := acpUpgradeSources[conn.Name]; ok {
		return &s
	}
	if acpIsNpx(conn.Command, conn.Args) {
		return &acpUpgradeSource{Kind: acpUpdNPM, Package: acpNpxPackage(conn.Args)}
	}
	return nil
}

// acpUpgradeScript returns the shell script installing the newest release of
// a connection, empty when none is known.
func acpUpgradeScript(conn config.ACPConnection) string {
	if s := acpSourceFor(conn); s != nil {
		return s.Upgrade
	}
	return ""
}

// acpUpgradeChannel names the install channel an upgrade script assumes.
func acpUpgradeChannel(conn config.ACPConnection) string {
	if s := acpSourceFor(conn); s != nil {
		return s.Channel
	}
	return ""
}

// acpNPMGlobalPrefix asks npm where global installs go. Overridable so tests
// can pin the answer.
var acpNPMGlobalPrefix = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "npm", "config", "get", "prefix").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// acpNPMGlobalBin is where npm puts global commands for a prefix. Windows
// keeps them in the prefix itself.
func acpNPMGlobalBin(prefix string) string {
	if runtime.GOOS == "windows" {
		return prefix
	}
	return filepath.Join(prefix, "bin")
}

// acpUnderDir reports whether path sits inside dir (or is dir itself).
func acpUnderDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// acpChannelOK decides whether an npm-channel upgrade script would actually
// touch the command that was resolved: it must live in npm's own global bin.
// When npm cannot answer (not installed), the check stays optimistic — the
// upgrade's own log will speak if it goes wrong.
func acpChannelOK(path string) bool {
	prefix, err := acpNPMGlobalPrefix()
	if err != nil || prefix == "" {
		return true
	}
	return acpUnderDir(path, acpNPMGlobalBin(prefix))
}

// acpNPXCacheDir is where npm exec keeps the packages npx has run.
func acpNPXCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".npm", "_npx")
}

// acpNpxCacheVersions reads the versions of a package's cached copies in the
// npx cache, newest first. Several cache entries can hold the package; the
// newest is what a cached run would use.
func acpNpxCacheVersions(cacheDir, pkg string) []string {
	if pkg == "" || cacheDir == "" {
		return nil
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, e.Name(), "node_modules", filepath.FromSlash(pkg), "package.json"))
		if err != nil {
			continue
		}
		var meta struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &meta) != nil || meta.Version == "" {
			continue
		}
		if seen[meta.Version] {
			continue
		}
		seen[meta.Version] = true
		out = append(out, meta.Version)
	}
	sort.Slice(out, func(i, j int) bool { return acpSemverCompare(out[i], out[j]) > 0 })
	return out
}

// acpIsNpx reports whether the command line runs a package through npx, such
// as "npx -y @zed-industries/claude-code-acp".
func acpIsNpx(command string, args []string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(fields[0])), ".exe")
	if base != "npx" && base != "npx.cmd" {
		return false
	}
	return acpNpxPackage(args) != ""
}

// acpNpxPackage returns the package an npx invocation runs: the first
// argument that is not a flag, skipping -y and its kin.
func acpNpxPackage(args []string) string {
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

// acpUpdState is what the settings tab knows about one connection's CLI
// version and the newest release. Every field is touched on the main thread
// only; the probes run in the background and post their results back.
type acpUpdState struct {
	// checked marks a finished check, so the row stops saying "not checked".
	checked bool
	// checking marks a running version check.
	checking bool
	// installed is the version the CLI itself reports, empty when unknown.
	installed string
	// latest is the newest release from the source, empty when unknown.
	latest string
	// known reports whether a release feed exists for this connection.
	known bool
	// err explains a failed check, empty on success.
	err string
	// path is the resolved command the probe ran, package the npm package an
	// npx command pulls; both make the check's subject visible.
	path string
	pkg  string
	// cache is the newest copy of an npx package in the local npx cache, so
	// a stale cache is visible against the registry's latest.
	cache string
	// channel names where the resolved command came from ("npm 全局", "非
	// npm 全局"), and npmOK reports whether an npm-channel upgrade script
	// would actually touch it.
	channel string
	npmOK   bool
	// upgrading marks a running upgrade; log and upgErr hold its outcome.
	upgrading bool
	log       string
	upgErr    string
}

// newer reports whether a checked connection has a newer release than the
// version its CLI reported. An unknown installed version never counts as
// outdated, because there is nothing to compare against.
func (st *acpUpdState) newer() bool {
	return st.installed != "" && st.latest != "" && acpSemverCompare(st.installed, st.latest) < 0
}

// acpUpdFor returns the check state stored under a connection's name, nil
// before the first check. The map is written only from main-thread code.
func (cw *ChatWindow) acpUpdFor(name string) *acpUpdState {
	return cw.acpUpd[name]
}

// acpFormConn is the connection the form's version row describes: the saved
// record with its command fields as currently typed, so a just-located
// command can be probed before saving.
func (cw *ChatWindow) acpFormConn(f *acpForm) config.ACPConnection {
	conn := config.ACPConnection{Name: "ACP", Command: strings.TrimSpace(f.command)}
	if f.editing >= 0 && f.editing < len(cw.cfg.ACPAgents) {
		conn = cw.cfg.ACPAgents[f.editing]
		conn.Command = strings.TrimSpace(f.command)
		conn.Args = splitLines(f.args)
	}
	return conn
}

// checkACPUpdate probes the installed version and fetches the newest release
// of a connection in the background, then shows both on the version row.
func (cw *ChatWindow) checkACPUpdate(conn config.ACPConnection) {
	if strings.TrimSpace(conn.Command) == "" {
		cw.toast("请先填写命令", mcore.SeverityInfo)
		return
	}
	if cw.acpUpd == nil {
		cw.acpUpd = map[string]*acpUpdState{}
	}
	st := cw.acpUpdFor(conn.Name)
	if st == nil {
		st = &acpUpdState{}
		cw.acpUpd[conn.Name] = st
	}
	if st.checking || st.upgrading {
		return
	}
	st.checking, st.checked, st.err = true, false, ""
	st.installed, st.latest, st.known = "", "", false
	st.path, st.pkg = "", ""
	st.cache, st.channel, st.npmOK = "", "", true
	cw.invalidate()

	src := acpSourceFor(conn)
	go func() {
		probe, instErr := acpProbeVersion(conn)
		latest, latErr := "", error(nil)
		if src != nil {
			latest, latErr = src.latestVersion()
		}
		// Where the resolved command came from decides whether an npm
		// install -g script would really update it.
		channel, npmOK := "", true
		if src != nil && src.Channel == acpUpdChannelNPM && probe.Path != "" && instErr == nil {
			if acpChannelOK(probe.Path) {
				channel = "npm 全局"
			} else {
				channel = "非 npm 全局"
				npmOK = false
			}
		}
		// A cached npx copy can lag the registry; name it when one exists.
		cache := ""
		if probe.Package != "" {
			if vs := acpNpxCacheVersions(acpNPXCacheDir(), probe.Package); len(vs) > 0 {
				cache = vs[0]
			}
		}
		cw.postMain(func() {
			st := cw.acpUpdFor(conn.Name)
			if st == nil {
				return
			}
			st.checking, st.checked, st.known = false, true, src != nil
			st.installed, st.latest = probe.Version, latest
			st.path, st.pkg = probe.Path, probe.Package
			st.cache, st.channel, st.npmOK = cache, channel, npmOK
			st.err = acpUpdErr(instErr, latErr)
			cw.invalidate()
		})
	}()
}

// acpUpdErr turns the two possible check failures into one message, keeping
// the newest-release error when both fail, since the release feed is the
// part an installed CLI cannot explain.
func acpUpdErr(instErr, latErr error) string {
	switch {
	case latErr != nil:
		return "最新版本获取失败：" + latErr.Error()
	case instErr != nil:
		return instErr.Error()
	}
	return ""
}

// upgradeACP runs a connection's upgrade script in the background, shows the
// outcome on the version row and re-checks the version when it succeeds.
func (cw *ChatWindow) upgradeACP(conn config.ACPConnection) {
	script := acpUpgradeScript(conn)
	if script == "" {
		return
	}
	st := cw.acpUpdFor(conn.Name)
	if st == nil || st.checking || st.upgrading {
		return
	}
	st.upgrading, st.upgErr, st.log = true, "", ""
	cw.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), acpUpgradeTimeout)
		defer cancel()
		// The environment is inherited, so the process PATH shellenv merged
		// at startup still applies.
		c := shellCommandContext(ctx, script, "")
		out, err := c.CombinedOutput()
		cw.postMain(func() {
			st := cw.acpUpdFor(conn.Name)
			if st == nil {
				return
			}
			st.upgrading = false
			st.log = acpTailOutput(string(out))
			if err != nil {
				switch {
				case ctx.Err() == context.DeadlineExceeded:
					st.upgErr = "升级超时"
				case st.log != "":
					// "exit status 1" alone says little; the CLI's own
					// complaint names the cause.
					st.upgErr = acpFirstLine(st.log)
				default:
					st.upgErr = acpFirstLine(err.Error())
				}
				cw.toast("升级 "+conn.Name+" 失败", mcore.SeverityError)
			} else {
				cw.toast("已升级 "+conn.Name+"，正在重新检查版本", mcore.SeveritySuccess)
				cw.checkACPUpdate(conn)
			}
			cw.invalidate()
		})
	}()
}

// acpProbeResult is what one version probe learned: the CLI's own version,
// the executable that actually answered, and the npm package an npx command
// runs. Path and Package are filled even when the version is not, so the
// settings row can show exactly what was probed.
type acpProbeResult struct {
	Version string
	Path    string
	Package string
}

// acpPackageVersion reads the version out of the package.json of the npm
// package the file at path is part of — magpie's trick (proc.Version): an
// npm-installed CLI already carries in its own package.json the version its
// --version would print, so nothing is run at all: no slow first run of an
// npx wrapper, no macOS notice for a flagged binary, and no side effects a
// --version can have (Cline checks for an update and self-installs on every
// start, its --version included). "" when the file is in no package.
func acpPackageVersion(path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	// Walk up from the file: the first directory whose parent is
	// node_modules is the package — a scoped one sits one level deeper,
	// under node_modules/@scope/pkg.
	for d := filepath.Dir(real); ; d = filepath.Dir(d) {
		up := filepath.Dir(d)
		if up == d {
			return ""
		}
		if filepath.Base(up) == "node_modules" ||
			strings.HasPrefix(filepath.Base(up), "@") && filepath.Base(filepath.Dir(up)) == "node_modules" {
			var p struct {
				Version string `json:"version"`
			}
			if b, err := os.ReadFile(filepath.Join(d, "package.json")); err == nil && json.Unmarshal(b, &p) == nil {
				return p.Version
			}
			return ""
		}
	}
}

// acpProbeVersion asks the connection's CLI for its version. The command is
// resolved from the system first, and the resolved path is reported back with
// the answer. The npm package the CLI lives in answers from its package.json
// without a process being started; only a CLI outside any package is run
// with --version, because stored args such as "acp" would start a session
// instead. npx commands are probed with their package argument so the
// adapter itself answers. A CLI that only answers the version subcommand
// gets that as a second try.
func acpProbeVersion(conn config.ACPConnection) (acpProbeResult, error) {
	fields := strings.Fields(strings.TrimSpace(conn.Command))
	if len(fields) == 0 {
		return acpProbeResult{}, fmt.Errorf("命令未填写")
	}
	path, ok := shellenv.ResolveCommand(fields[0])
	if !ok {
		return acpProbeResult{Path: fields[0]}, fmt.Errorf("未找到命令 %s", fields[0])
	}
	res := acpProbeResult{Path: path}
	pkg := ""
	if acpIsNpx(conn.Command, conn.Args) {
		res.Package = acpNpxPackage(conn.Args)
		pkg = res.Package
	} else if v := acpPackageVersion(path); v != "" {
		// The package the resolved binary belongs to has already said its
		// version; the row's subject is that binary, so the path is
		// reported as resolved rather than through its symlinks.
		res.Version = v
		return res, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), acpVersionTimeout)
	defer cancel()
	var lastOut, lastErrOut string
	for _, sub := range []string{"--version", "version"} {
		args := []string{sub}
		if pkg != "" {
			args = append(append([]string{}, conn.Args...), sub)
		}
		out, errOut := acpRunCapture(ctx, path, args)
		lastOut, lastErrOut = out, errOut
		if v := acpExtractVersion(out, errOut); v != "" {
			res.Version = v
			return res, nil
		}
		if ctx.Err() == context.DeadlineExceeded {
			return res, fmt.Errorf("获取版本超时")
		}
	}
	return res, fmt.Errorf("无法获取版本: %s", acpFirstLine(lastOut+lastErrOut))
}

// acpRunCapture runs the resolved command with its stdout and stderr kept
// apart: npx and npm print their own version-bearing notices on stderr, and
// mixing the two streams is how a check could read the wrong version.
// NO_COLOR keeps ANSI sequences out of the answer, so a decorated "1.2.3"
// still parses.
func acpRunCapture(ctx context.Context, path string, args []string) (string, string) {
	c := acpExec(ctx, path, args)
	c.Env = append(os.Environ(), "NO_COLOR=1")
	var out, errOut strings.Builder
	c.Stdout = &out
	c.Stderr = &errOut
	_ = c.Run()
	return out.String(), errOut.String()
}

// acpExec wraps the invocation of a resolved command. Windows cannot execute
// .cmd and .bat files directly, so they go through cmd.exe.
func acpExec(ctx context.Context, path string, args []string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		if ext := strings.ToLower(filepath.Ext(path)); ext == ".cmd" || ext == ".bat" {
			return exec.CommandContext(ctx, "cmd", append([]string{"/c", path}, args...)...)
		}
	}
	return exec.CommandContext(ctx, path, args...)
}

// acpVersionRe finds the first version-looking token, e.g. "0.16.2" from
// "claude-code-acp 0.16.2" or "1.19.5" from "OpenCode v1.19.5".
var acpVersionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)*`)

// acpNoiseLine reports whether a line is npm's own chatter rather than the
// CLI's answer. npx and npm write warnings and notices on stderr — some of
// which carry version numbers of their own — and those must never be read as
// the agent's version.
func acpNoiseLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return true
	}
	return strings.EqualFold(fields[0], "npm") || strings.HasPrefix(fields[0], "npm@")
}

// acpExtractVersion picks the first version number out of a CLI's output,
// skipping npm's noise lines; stdout is read before stderr, because stderr
// carries the notices.
func acpExtractVersion(outs ...string) string {
	for _, out := range outs {
		for _, line := range strings.Split(out, "\n") {
			if acpNoiseLine(line) {
				continue
			}
			if v := acpVersionRe.FindString(line); v != "" {
				return v
			}
		}
	}
	return ""
}

// acpSemverCompare compares two dotted versions numerically: -1 when a is
// older, 0 when equal, 1 when newer. Missing components count as zero and
// non-numeric suffixes such as "1.0.0-beta" are ignored.
func acpSemverCompare(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = acpAtoiPrefix(as[i])
		}
		if i < len(bs) {
			bv = acpAtoiPrefix(bs[i])
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// acpAtoiPrefix parses the leading digits of s ("2" out of "2-beta"),
// returning 0 when there are none.
func acpAtoiPrefix(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0
	}
	return n
}

// acpHTTPClient fetches release feeds. Like every HTTP call of the app it
// honors the process proxy environment.
var acpHTTPClient = &http.Client{Timeout: acpLatestTimeout}

// latestVersion fetches the newest released version from the source's feed.
func (s acpUpgradeSource) latestVersion() (string, error) {
	switch s.Kind {
	case acpUpdNPM:
		return acpNPMLatest(s.Package)
	case acpUpdGitHub:
		return acpGitHubLatest(s.Repo)
	}
	return "", fmt.Errorf("未知的版本来源")
}

// acpNPMLatest reads the version of a package's latest tag from the npm
// registry.
func acpNPMLatest(pkg string) (string, error) {
	var body struct {
		Version string `json:"version"`
	}
	if err := acpGetJSON("https://registry.npmjs.org/"+url.PathEscape(pkg)+"/latest", &body); err != nil {
		return "", err
	}
	if body.Version == "" {
		return "", fmt.Errorf("npm 没有返回版本号")
	}
	return body.Version, nil
}

// acpGitHubLatest reads the tag of a repository's latest published release.
// GitHub's unauthenticated API allows 60 lookups per hour per address, which
// button-driven checks stay far below. Renamed repositories are followed
// through their redirect.
func acpGitHubLatest(repo string) (string, error) {
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := acpGetJSON("https://api.github.com/repos/"+repo+"/releases/latest", &body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("GitHub 没有已发布的版本")
	}
	return strings.TrimPrefix(body.TagName, "v"), nil
}

// acpGetJSON performs a GET and decodes the JSON body into out. The Accept
// header is GitHub's release-feed one; npm ignores it.
func acpGetJSON(addr string, out any) error {
	req, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := acpHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败：%s", acpFirstLine(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败（HTTP %d）", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取响应失败：%s", acpFirstLine(err.Error()))
	}
	return json.Unmarshal(data, out)
}

// acpTailOutput keeps the last acpUpdLogCap bytes of a command's output,
// starting at the next intact line.
func acpTailOutput(out string) string {
	out = strings.TrimSpace(out)
	if len(out) <= acpUpdLogCap {
		return out
	}
	out = out[len(out)-acpUpdLogCap:]
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		out = out[i+1:]
	}
	return out
}

// acpFirstLine trims a command's output or error to its first non-empty line.
func acpFirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// acpVersionRow renders the version-and-upgrade row of an expanded ACP
// connection form. It shows the result of a previous check; nothing here
// probes in the background on its own, checks follow the button.
func (cw *ChatWindow) acpVersionRow(c *mgo.Context, f *acpForm) {
	conn := cw.acpFormConn(f)
	if conn.Name == "ACP" && conn.Command == "" {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	st := cw.acpUpdFor(conn.Name)
	busy := st != nil && (st.checking || st.upgrading)
	mgo.Column(c).FillWidth().Gap(u).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "版本").FontSize(small).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, acpUpdHeadline(st)).Grow(1).MinWidth(0).FontSize(small).
				TextColor(acpUpdColor(st, k)).SingleLine().Ellipsis("…")
			if kit.Button(c, "检查更新", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("refresh-cw"), Disabled: busy}).Clicked() {
				cw.checkACPUpdate(conn)
			}
			if st != nil && st.newer() && acpUpgradeScript(conn) != "" && st.npmOK {
				if kit.Button(c, "升级", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("arrow-up"), Disabled: busy}).Clicked() {
					cw.upgradeACP(conn)
				}
			}
		})
		if st != nil && st.checked && !st.checking && !st.upgrading {
			mgo.Row(c).FillWidth().Gap(u * 3).AlignItems(mgo.Center).Children(func() {
				switch {
				case st.installed != "":
					mgo.Text(c, "已安装 "+st.installed).FontSize(small).TextColor(k.TextMuted).Shrink(0)
				case st.pkg != "":
					// An npx adapter answers no --version at all; npx resolves
					// the package's latest release on every run, so there is
					// no installed copy to name.
					mgo.Text(c, "npx 运行时自动拉取最新包").FontSize(small).TextColor(k.TextMuted).Shrink(0)
				default:
					mgo.Text(c, "未获取到已安装版本").FontSize(small).TextColor(k.TextMuted).Shrink(0)
				}
				if st.known && st.latest != "" {
					latest := mgo.Text(c, "最新 "+st.latest).FontSize(small).Shrink(0)
					if st.newer() {
						latest.TextColor(k.Accent)
					}
				} else {
					mgo.Text(c, "未收录该 CLI 的发布源").FontSize(small).TextColor(k.TextMuted)
				}
				// A cached npx copy can lag the registry; name it when one
				// exists, so an offline run's version is no surprise.
				if st.cache != "" {
					mgo.Text(c, "缓存 "+st.cache).FontSize(small).TextColor(k.TextMuted).Shrink(0)
				}
				if st.err != "" {
					mgo.Text(c, st.err).FontSize(small).TextColor(k.Danger).SingleLine().Ellipsis("…")
				}
				if st.upgErr != "" {
					mgo.Text(c, "升级失败："+st.upgErr).FontSize(small).TextColor(k.Danger).SingleLine().Ellipsis("…")
				}
			})
			// The command the probe actually resolved, so the check's subject
			// is never a guess: the system-resolved path, and for npx the
			// package being run.
			if st.path != "" {
				mgo.Row(c).FillWidth().Gap(u * 3).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, "命令路径").FontSize(small).TextColor(k.TextMuted).Shrink(0)
					mgo.Text(c, st.path).Font(mtheme.MonoFont).FontSize(small).
						TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					if st.pkg != "" {
						mgo.Text(c, "包 "+st.pkg).FontSize(small).TextColor(k.TextMuted).Shrink(0)
					}
				})
			}
			// An npm-channel script upgrades only an npm-global command; say
			// so when the resolved path came from somewhere else.
			if acpUpgradeChannel(conn) == acpUpdChannelNPM && st.checked && st.channel != "" && !st.npmOK {
				mgo.Row(c).FillWidth().Gap(u * 3).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, "安装渠道 "+st.channel).FontSize(small).TextColor(k.Danger).Shrink(0)
					mgo.Text(c, "npm install -g 不会更新此命令").FontSize(small).
						TextColor(k.Danger).SingleLine().Ellipsis("…")
				})
			}
			if st.log != "" {
				mgo.Box(c).FillWidth().Padding(u, u*2).Radius(mtheme.CardRadius).
					Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Children(func() {
					mgo.Text(c, st.log).Font(mtheme.MonoFont).FontSize(small).TextColor(k.TextMuted).MaxLines(4)
				})
			}
		}
	})
}

// acpUpdHeadline summarizes a check for the row beside the buttons. The
// running states come first: checking clears checked, so a plain "not
// checked yet" test would otherwise swallow it.
func acpUpdHeadline(st *acpUpdState) string {
	switch {
	case st == nil:
		return "点击检查更新"
	case st.upgrading:
		return "正在升级…"
	case st.checking:
		return "正在检查…"
	case !st.checked:
		return "点击检查更新"
	case st.newer():
		return "发现新版本 " + st.installed + " → " + st.latest
	case st.installed != "":
		return "已安装 " + st.installed
	case st.latest != "":
		return "最新 " + st.latest
	case st.err != "":
		return "检查失败"
	default:
		return "点击检查更新"
	}
}

// acpUpdColor picks the headline color: accent when a newer release exists,
// danger when the check failed, muted otherwise.
func acpUpdColor(st *acpUpdState, k *mgo.Theme) mgo.Color {
	switch {
	case st == nil, !st.checked:
		return k.TextMuted
	case st.newer():
		return k.Accent
	case st.err != "":
		return k.Danger
	default:
		return k.TextMuted
	}
}

// acpUpdBadge marks a connection whose last check found a newer release. It
// only reads a previous check's result; rendering never starts a probe.
func (cw *ChatWindow) acpUpdBadge(c *mgo.Context, name string) {
	st := cw.acpUpdFor(name)
	if st == nil || !st.newer() {
		return
	}
	mgo.Text(c, "可升级").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
		TextColor(mcore.Tokens(c).Accent).Shrink(0)
}
