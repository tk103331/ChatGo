// Command add-registry-agents adds two real ACP Registry agents to the
// user's real config the way the settings dialog does — Codex over its
// pinned npx package, and OpenCode V2 through the real binary installer —
// so the app can be opened and driven from its actual UI.
//
// Usage:
//
//	go run ./tools/add-registry-agents
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"chatgo/internal/acpregistry"
	"chatgo/internal/config"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("· 拉取 ACP Registry 索引…")
	reg, err := acpregistry.Fetch(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "索引拉取失败：", err)
		os.Exit(1)
	}
	var codex, opencode acpregistry.Agent
	for _, a := range reg.Agents {
		switch a.ID {
		case "codex-acp":
			codex = a
		case "opencode-v2":
			opencode = a
		}
	}
	if codex.ID == "" || opencode.ID == "" {
		fmt.Fprintln(os.Stderr, "索引缺少 codex-acp 或 opencode-v2")
		os.Exit(1)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取配置失败：", err)
		os.Exit(1)
	}

	// Codex：npx 映射，pin 注册表校验过的版本。
	command, args, env, ok := codex.Command()
	if !ok {
		fmt.Fprintln(os.Stderr, "codex-acp 没有 npx 发行")
		os.Exit(1)
	}
	cfg.ACPAgents = append(cfg.ACPAgents, config.ACPConnection{
		Name: codex.Name, Command: command, Args: args, Env: env,
		RegistryID: codex.ID,
	})
	fmt.Printf("· 已添加 %s v%s：%s %v\n", codex.Name, codex.Version, command, args)

	// OpenCode V2：真实二进制安装链路。
	fmt.Printf("· 安装 %s v%s 二进制（%s/%s）…\n", opencode.Name, opencode.Version, runtime.GOOS, runtime.GOARCH)
	home, _ := os.UserHomeDir()
	dest := filepath.Join(home, ".chatgo", "agents", opencode.ID, opencode.Version)
	bin, err := acpregistry.InstallBinary(ctx, opencode, runtime.GOOS, runtime.GOARCH, dest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "二进制安装失败：", err)
		os.Exit(1)
	}
	fmt.Println("· 安装完成：", bin)
	dist, _, _ := opencode.BinaryTarget(runtime.GOOS, runtime.GOARCH)
	cfg.ACPAgents = append(cfg.ACPAgents, config.ACPConnection{
		Name: opencode.Name, Command: bin, Args: dist.Args, Env: dist.Env,
		RegistryID: opencode.ID,
	})

	if err := config.SaveConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "保存配置失败：", err)
		os.Exit(1)
	}
	fmt.Println("· 已写入 ~/.chatgo/config.yaml。启动 ChatGo → 设置 → ACP 查看两个新连接，")
	fmt.Println("  或在设置 → ACP → 从 Registry 添加 里确认它们显示「已添加」。")
}
