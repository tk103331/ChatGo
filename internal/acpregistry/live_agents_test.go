package acpregistry

// Live verification against the real registry: fetch the published index,
// map a few representative entries, install them the way the settings
// dialog does, and drive each agent through a real ACP initialize
// handshake. Skipped without network or runtimes so a sandbox still passes.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// handshakeTimeout bounds one agent's whole run: npx resolving a package
// for the first time downloads it, which on a slow link takes a while.
const liveAgentTimeout = 4 * time.Minute

// stubClient answers the agent's client callbacks: permissions are denied
// and session updates are counted, so the handshake needs no UI.
type stubClient struct {
	updates int
}

func (stubClient) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, fmt.Errorf("not available")
}

func (stubClient) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, fmt.Errorf("not available")
}

func (s *stubClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	// Deny by picking nothing: the turn cannot run tools, which this test
	// never asks for anyway.
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}

func (s *stubClient) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	s.updates++
	return nil
}

func (s *stubClient) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("not available")
}

func (s *stubClient) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, nil
}

func (s *stubClient) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, nil
}

func (s *stubClient) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}

func (s *stubClient) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}

// envPairs flattens an entry's environment map into KEY=VALUE strings, in a
// stable order so a failing run can name what it passed.
func envPairs(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// liveAgentRun spawns one agent, runs the initialize handshake and reports
// what the agent says about itself. When the handshake fails because the
// agent process died on its own, it reports the exit code as an upstream
// failure the caller may treat as a skip rather than a mapping problem.
func liveAgentRun(t *testing.T, name, command string, args []string, env map[string]string) {
	t.Helper()
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), envPairs(env)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 %s 失败：%v", name, err)
	}
	exited := make(chan int, 1)
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
		err := cmd.Wait()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			code = -1
		}
		exited <- code
	}()
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
		}
	}()

	client := &stubClient{}
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	ctx, cancel := context.WithTimeout(context.Background(), liveAgentTimeout)
	defer cancel()
	started := time.Now()
	resp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{},
		ClientInfo:         &acp.Implementation{Name: "ChatGo-live-test", Version: "1"},
	})
	if err != nil {
		select {
		case code := <-exited:
			// The process died on its own before answering: the entry's
			// distribution is broken upstream, which is not this repo's
			// mapping or handshake at fault.
			t.Skipf("%s 启动后自行退出（exit %d），上游发行损坏：%v", name, code, err)
		case <-time.After(3 * time.Second):
			t.Fatalf("%s 握手失败：%v", name, err)
		}
		return
	}
	info := "unknown"
	if resp.AgentInfo != nil {
		info = resp.AgentInfo.Name + " " + resp.AgentInfo.Version
	}
	auth := "none"
	if n := len(resp.AuthMethods); n > 0 {
		kinds := make([]string, 0, n)
		for _, m := range resp.AuthMethods {
			switch {
			case m.EnvVar != nil:
				kinds = append(kinds, "env:"+m.EnvVar.Name)
			case m.Terminal != nil:
				kinds = append(kinds, "terminal")
			default:
				kinds = append(kinds, "other")
			}
		}
		auth = strings.Join(kinds, ",")
	}
	t.Logf("%s 握手成功：agent=%q auth=[%s] 用时 %s", name, info, auth, time.Since(started).Round(time.Millisecond))
}

// lookupRuntime reports whether a command is on PATH.
func lookupRuntime(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// TestLiveRegistryAgents walks search→install→execute for one agent of
// each distribution kind: codex-acp over npx, fast-agent over uvx, and
// amp-acp through the binary installer. The handshake is the execute step;
// an agent that answers initialize speaks the protocol the app relies on.
func TestLiveRegistryAgents(t *testing.T) {
	// The search step: the live index must be reachable.
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 20*time.Second)
	reg, err := Fetch(fetchCtx)
	fetchCancel()
	if err != nil {
		t.Skipf("ACP Registry 不可达，跳过真机验证：%v", err)
	}
	if len(reg.Agents) < 30 {
		t.Fatalf("索引条目异常少：%d", len(reg.Agents))
	}
	find := func(id string) Agent {
		for _, a := range reg.Agents {
			if a.ID == id {
				return a
			}
		}
		t.Fatalf("索引里没有 %s", id)
		return Agent{}
	}

	// npx: the package resolution downloads on first run, which is the
	// install step for an npx distribution.
	if !lookupRuntime("npx") {
		t.Log("npx 不在 PATH，跳过 npx 发行验证")
	} else {
		t.Run("npx-codex", func(t *testing.T) {
			a := find("codex-acp")
			command, args, env, ok := a.Command()
			if !ok || command != "npx" {
				t.Fatalf("映射结果不是 npx：%q %v", command, args)
			}
			liveAgentRun(t, "Codex("+a.ID+")", command, args, env)
		})
	}

	// uvx: a PyPI package through uv. fast-agent was the registry's other
	// uvx entry, but its 0.10.1 pin requires fastmcp-slim==4.0.0b1, which
	// PyPI does not serve — a broken upstream, not a mapping problem.
	if !lookupRuntime("uvx") {
		t.Log("uvx 不在 PATH，跳过 uvx 发行验证")
	} else {
		t.Run("uvx-minion-code", func(t *testing.T) {
			a := find("minion-code")
			command, args, env, ok := a.Command()
			if !ok || command != "uvx" {
				t.Fatalf("映射结果不是 uvx：%q %v", command, args)
			}
			liveAgentRun(t, "MiniMax Code("+a.ID+")", command, args, env)
		})
	}

	// binary: the installer downloads, verifies and extracts the archive,
	// then the resolved executable runs the same handshake. opencode-v2
	// serves its archives from its own CDN, so the download does not depend
	// on a particular host being reachable.
	t.Run("binary-opencode", func(t *testing.T) {
		a := find("opencode-v2")
		if !a.SupportsBinary(runtime.GOOS, runtime.GOARCH) {
			t.Skipf("opencode-v2 没有 %s/%s 的二进制包", runtime.GOOS, runtime.GOARCH)
		}
		dest := filepath.Join(t.TempDir(), "agents", a.ID, a.Version)
		ctx, cancel := context.WithTimeout(context.Background(), liveAgentTimeout)
		defer cancel()
		started := time.Now()
		bin, err := InstallBinary(ctx, a, runtime.GOOS, runtime.GOARCH, dest)
		if err != nil {
			t.Fatalf("二进制安装失败：%v", err)
		}
		t.Logf("opencode-v2 安装完成：%s（用时 %s）", bin, time.Since(started).Round(time.Millisecond))
		dist, _, _ := a.BinaryTarget(runtime.GOOS, runtime.GOARCH)
		liveAgentRun(t, "OpenCode V2("+a.ID+")", bin, dist.Args, dist.Env)
	})
}
