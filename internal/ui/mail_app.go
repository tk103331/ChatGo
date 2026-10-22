package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/mailapp"
)

// mailClient is the mailapp client bound to this window.
func (cw *ChatWindow) mailClient() *mailapp.Client {
	if cw.mailApp == nil {
		cw.mailApp = mailapp.New(appHost{cw: cw})
	}
	return cw.mailApp
}

// mailApp is the Apps menu entry for the mail client.
func mailApp() appDef {
	return appDef{
		ID:        "mail",
		Title:     "邮件",
		Desc:      "通过 IMAP 浏览与阅读邮件、通过 SMTP 发送邮件，Agent 可代为收发",
		Icon:      "inbox",
		System:    mailapp.System,
		Static:    true,
		ChatW:     400,
		NewTab:    func(cw *ChatWindow) string { return "" },
		Workspace: func(cw *ChatWindow, c *mgo.Context) { cw.mailClient().View(c) },
		Tools: func(cw *ChatWindow) []appTool {
			tools := cw.mailClient().Tools()
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
