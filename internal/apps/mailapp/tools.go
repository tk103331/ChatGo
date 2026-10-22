package mailapp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// Param is one tool parameter.
type Param struct {
	Desc     string
	Required bool
}

// Tool is one operation exposed to the Agent.
type Tool struct {
	Name   string
	Desc   string
	Params map[string]Param
	Run    func(args map[string]any) (string, error)
}

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "mail_accounts",
			Desc: "列出已配置的邮件账户",
			Run:  func(map[string]any) (string, error) { return cl.toolAccounts() },
		},
		{
			Name: "mail_list",
			Desc: "列出某个文件夹的最近邮件（默认 INBOX，50 封）",
			Params: map[string]Param{
				"folder": {Desc: "文件夹名，默认 INBOX"},
				"limit":  {Desc: "条数，默认 50"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolList(strArg(args, "folder"), strArg(args, "limit"))
			},
		},
		{
			Name: "mail_read",
			Desc: "读取指定 UID 的邮件正文",
			Params: map[string]Param{
				"uid":    {Desc: "邮件 UID（来自 mail_list）", Required: true},
				"folder": {Desc: "文件夹名，默认 INBOX"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRead(strArg(args, "uid"), strArg(args, "folder"))
			},
		},
		{
			Name: "mail_send",
			Desc: "发送纯文本邮件（发送前先确认收件人、主题和正文）",
			Params: map[string]Param{
				"to":      {Desc: "收件人地址", Required: true},
				"subject": {Desc: "主题"},
				"body":    {Desc: "正文"},
				"account": {Desc: "账户名，留空用当前账户"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolSend(strArg(args, "to"), strArg(args, "subject"), strArg(args, "body"), strArg(args, "account"))
			},
		},
		{
			Name: "mail_mailboxes",
			Desc: "列出账户的文件夹",
			Run: func(map[string]any) (string, error) { return cl.toolMailboxes() },
		},
	}
}

// currentAccount returns the active account.
func (cl *Client) currentAccount(name string) (Account, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return findAccount(cl.st.cfg, name)
}

// toolAccounts lists accounts.
func (cl *Client) toolAccounts() (string, error) {
	cl.mu.Lock()
	cfg := cl.st.cfg
	cl.mu.Unlock()
	if len(cfg.Accounts) == 0 {
		return "(未配置账户)", nil
	}
	var b strings.Builder
	for _, a := range cfg.Accounts {
		mark := ""
		if a.Name == cfg.Current {
			mark = " [当前]"
		}
		fmt.Fprintf(&b, "- %s <%s> IMAP=%s SMTP=%s%s\n", a.displayName(), a.Email, a.imapAddr(), a.smtpAddr(), mark)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolList lists a mailbox.
func (cl *Client) toolList(folder, limit string) (string, error) {
	acc, ok := cl.currentAccount("")
	if !ok {
		return "", fmt.Errorf("未配置账户")
	}
	if folder == "" {
		folder = "INBOX"
	}
	n := 50
	if v, err := strconv.Atoi(strings.TrimSpace(limit)); err == nil && v > 0 {
		n = v
	}
	msgs, err := listMessages(acc, folder, n)
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "(没有邮件)", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s（%d 封）\n", folder, len(msgs))
	for _, m := range msgs {
		flag := " "
		if m.Unread {
			flag = "*"
		}
		fmt.Fprintf(&b, "%s %d  %s  %s  %s\n", flag, m.UID, formatDate(m.Date), m.From, m.Subject)
	}
	cl.host.Post(func() { cl.mu.Lock(); cl.st.mailbox = folder; cl.mu.Unlock(); cl.refresh() })
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolRead reads one message.
func (cl *Client) toolRead(uidStr, folder string) (string, error) {
	acc, ok := cl.currentAccount("")
	if !ok {
		return "", fmt.Errorf("未配置账户")
	}
	uid, err := strconv.ParseUint(strings.TrimSpace(uidStr), 10, 32)
	if err != nil {
		return "", fmt.Errorf("无效 uid %q", uidStr)
	}
	if folder == "" {
		folder = "INBOX"
	}
	body, err := readMessage(acc, folder, uint32(uid))
	if err != nil {
		return "", err
	}
	cl.host.Post(func() { cl.openMessage(uint32(uid)) })
	var b strings.Builder
	fmt.Fprintf(&b, "主题: %s\n发件人: %s\n收件人: %s\n时间: %s\n\n%s",
		body.Subject, body.From, body.To, body.Date.Format("2006-01-02 15:04"), body.Text)
	return b.String(), nil
}

// toolSend sends a message.
func (cl *Client) toolSend(to, subject, body, account string) (string, error) {
	acc, ok := cl.currentAccount(account)
	if !ok {
		return "", fmt.Errorf("未配置账户")
	}
	if err := sendMessage(acc, to, subject, body); err != nil {
		return "", err
	}
	cl.host.Post(func() { cl.host.Toast("已发送给 "+to, mcore.SeveritySuccess) })
	return "已发送给 " + to, nil
}

// toolMailboxes lists mailboxes.
func (cl *Client) toolMailboxes() (string, error) {
	acc, ok := cl.currentAccount("")
	if !ok {
		return "", fmt.Errorf("未配置账户")
	}
	names, err := listMailboxes(acc)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "(没有文件夹)", nil
	}
	return strings.Join(names, "\n"), nil
}

// strArg reads a string argument.
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if b, err := json.Marshal(v); err == nil {
			return strings.Trim(string(b), "\"")
		}
	}
	return ""
}
