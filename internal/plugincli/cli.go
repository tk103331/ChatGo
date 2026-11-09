// Package plugincli implements `chatgo plugin <subcommand>`, so plugins can be
// installed and managed without opening the window.
package plugincli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/plugin"
)

// Run handles one `chatgo plugin` invocation.
func Run(args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	m := plugin.NewManager(cfg)
	m.LoadAll()
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "list":
		infos := m.List()
		if len(infos) == 0 {
			fmt.Println("尚未安装插件")
			return nil
		}
		for _, in := range infos {
			fmt.Printf("%-24s %-8s v%-8s %s\n", in.ID, in.Status, in.Version, in.Name)
		}
		return nil
	case "install":
		return install(m, args[1:])
	case "remove", "uninstall":
		if len(args) < 2 {
			return fmt.Errorf("用法: chatgo plugin remove <id>")
		}
		return m.Uninstall(args[1], containsArg(args[2:], "--data"))
	case "enable":
		if len(args) < 2 {
			return fmt.Errorf("用法: chatgo plugin enable <id>")
		}
		return enable(m, args[1])
	case "disable":
		if len(args) < 2 {
			return fmt.Errorf("用法: chatgo plugin disable <id>")
		}
		return m.SetEnabled(args[1], false, nil, nil)
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf("用法: chatgo plugin <list|install|remove|enable|disable> ...")
}

// install installs from a local directory, zip, git URL or npm package.
func install(m *plugin.Manager, args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	kind := fs.String("kind", "dir", "dir | zip | git | npm")
	src := fs.String("src", "", "目录/zip 路径、git 地址或 npm 包名")
	ref := fs.String("ref", "", "git 分支或标签")
	enableAfter := fs.Bool("enable", false, "安装后立即启用（授予清单声明的全部权限）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*src) == "" {
		return fmt.Errorf("缺少 -src")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	opts := plugin.InstallOptions{Kind: *kind, Enabled: false}
	if *kind == "dir" || *kind == "zip" {
		opts.Path = *src
	} else {
		opts.URL = *src
		opts.Ref = *ref
	}
	manifest, err := m.Install(ctx, opts)
	if err != nil {
		return err
	}
	fmt.Printf("已安装 %s (%s)\n", manifest.Name, manifest.ID)
	if *enableAfter {
		return enable(m, manifest.ID)
	}
	fmt.Println("插件处于禁用状态；用 `chatgo plugin enable " + manifest.ID + "` 授权并启用")
	return nil
}

// enable grants the manifest's declared permissions and loads the plugin. The
// CLI is an explicit user action, so it grants them all.
func enable(m *plugin.Manager, id string) error {
	info, ok := m.Info(id)
	if !ok {
		return fmt.Errorf("插件不存在: %s", id)
	}
	perms := append([]string{}, info.Permissions...)
	perms = append(perms, info.Missing...)
	if err := m.SetEnabled(id, true, perms, info.Values); err != nil {
		return err
	}
	fmt.Printf("已启用 %s；授予权限: %s\n", info.Name, strings.Join(perms, ", "))
	return nil
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
