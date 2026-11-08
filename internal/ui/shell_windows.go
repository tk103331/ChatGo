//go:build windows

package ui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// explorerMenuSupported reports whether the folder context menu exists here.
func explorerMenuSupported() bool { return true }

// setExplorerMenu adds or removes the "用 ChatGo 快捷任务打开" entries on
// folders and on a folder's background, so a right click opens the panel
// with that directory as the working directory.
func setExplorerMenu(enable bool) error {
	const (
		dirKey = `Software\Classes\Directory\shell\ChatGo`
		bgKey  = `Software\Classes\Directory\Background\shell\ChatGo`
	)
	if !enable {
		for _, key := range []string{dirKey, bgKey} {
			_ = registry.DeleteKey(registry.CURRENT_USER, key+`\command`)
			_ = registry.DeleteKey(registry.CURRENT_USER, key)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return fmt.Errorf("找不到程序路径")
	}
	entries := []struct {
		key string
		arg string
	}{
		{dirKey, "%1"},
		{bgKey, "%V"},
	}
	for _, entry := range entries {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, entry.key, registry.SET_VALUE)
		if err != nil {
			return err
		}
		_ = key.SetStringValue("", "用 ChatGo 快捷任务打开")
		_ = key.SetStringValue("Icon", exe+",0")
		key.Close()

		command, _, err := registry.CreateKey(registry.CURRENT_USER, entry.key+`\command`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = command.SetStringValue("", fmt.Sprintf("\"%s\" --dir \"%s\"", exe, entry.arg))
		command.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
