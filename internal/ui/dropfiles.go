package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/ui/kit"
	mcore "chatgo/internal/ui/uikit"
)

// addDroppedFiles turns files dropped on the composer into @-mentions, the
// same references the context picker inserts. Paths outside the workspace stay
// absolute so the agent can still find them.
func (cw *ChatWindow) addDroppedFiles(paths []string) {
	added := 0
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		rel := cw.workspaceRel(path)
		if rel == "" {
			continue
		}
		mention := "@" + rel + " "
		if strings.Contains(cw.draft, mention) {
			continue
		}
		cw.insertContextMention(kit.ContextItem{ID: rel, Label: rel, Kind: kit.ContextFile})
		added++
	}
	if added > 0 {
		cw.toast(fmt.Sprintf("已添加 %d 个文件引用", added), mcore.SeveritySuccess)
	}
	cw.invalidate()
}
