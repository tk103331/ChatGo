package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/filemanagerapp"
)

// fileManagerClient is the filemanagerapp client bound to this window.
func (cw *ChatWindow) fileManagerClient() *filemanagerapp.Client {
	if cw.filesApp == nil {
		cw.filesApp = filemanagerapp.New(appHost{cw: cw})
	}
	return cw.filesApp
}

// fileManagerApp is the Apps menu entry for the file manager.
func fileManagerApp() appDef {
	return appDef{
		ID:        "files",
		Title:     "文件管理器",
		Desc:      "浏览工作区文件，预览内容，新建、重命名与删除，Agent 可代为操作",
		Icon:      "folder",
		System:    filemanagerapp.System,
		Static:    true,
		ChatW:     400,
		NewTab:    func(cw *ChatWindow) string { return "" },
		Workspace: func(cw *ChatWindow, c *mgo.Context) { cw.fileManagerClient().View(c) },
		Tools: func(cw *ChatWindow) []appTool {
			tools := cw.fileManagerClient().Tools()
			out := make([]appTool, 0, len(tools))
			for _, t := range tools {
				t := t
				params := make(map[string]appParam, len(t.Params))
				for name, p := range t.Params {
					params[name] = appParam{Desc: p.Desc, Required: p.Required}
				}
				out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: params, Run: func(cw *ChatWindow, args map[string]any) (string, error) {
					return t.Run(args)
				}})
			}
			return out
		},
	}
}
