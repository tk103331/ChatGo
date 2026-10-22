package mailapp

import (
	"fmt"
	"strings"
	"sync"
	"time"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是邮件助手。你可以通过 mail_accounts 查看已配置账户，通过 mail_list 列出某个文件夹的最近邮件，通过 mail_read 读取指定 UID 的邮件正文，通过 mail_send 发送纯文本邮件，通过 mail_mailboxes 列出文件夹。发送邮件前先向用户确认收件人、主题和正文。"

// state is the client's retained model.
type state struct {
	cfg     mailConfig
	mailbox string
	msgs    []Message
	picked  uint32
	body    MessageBody
	loading bool
	err     string
	list    data.ListState[uint32]

	composing bool
	to        string
	subject   string
	text      string

	adding       bool
	formProvider int
	formEmail    string
	formPass     string
	formUser     string
	formIMAP     string
	formSMTP     string

	splitW float32
}

// Client is one mail client bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	st   *state

	view    *mygo.WebView
	viewUID uint32
	viewErr string
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a mail client driving the given host.
func New(h Host) *Client {
	cfg := loadConfig()
	cl := &Client{host: h, st: &state{cfg: cfg, mailbox: "INBOX", splitW: 420}}
	return cl
}

// View renders the mail client.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	st := cl.snapshot()
	acc, hasAcc := findAccount(st.cfg, "")
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			if hasAcc {
				label := acc.displayName()
				if minput.Button(c, label, minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("inbox")}).Clicked() {
					cl.nextAccount()
				}
			} else {
				mgo.Text(c, "未配置账户").FontSize(caption).TextColor(k.TextMuted)
				mgo.Text(c, "支持 "+providerNames()).FontSize(caption).TextColor(k.TextMuted).
					Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			}
			if hasAcc {
				minput.TextInput(c, &cl.st.mailbox, minput.TextInputOptions{Placeholder: "文件夹"}).Element.Width(110).Shrink(0)
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
					cl.refresh()
				}
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("pencil"), Label: "写邮件"}).Clicked() {
					cl.openCompose()
				}
			}
			mgo.Box(c).Grow(1)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("plus"), Label: "添加账户"}).Clicked() {
				cl.openAccountForm()
			}
		})
		if st.err != "" {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mgo.Text(c, st.err).FontSize(caption).TextColor(k.Danger).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			})
		}
		if st.loading {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "加载中…").FontSize(caption).TextColor(k.TextMuted)
			})
		}
		if st.adding {
			cl.accountForm(c, st, k, u, caption)
			return
		}
		if st.composing {
			cl.composePane(c, st, k, u, caption)
			return
		}
		if !hasAcc {
			mgo.Column(c).Fill().Center().Children(func() {
				mgo.Text(c, "点击右上角「添加账户」配置 IMAP/SMTP").FontSize(caption).TextColor(k.TextMuted)
			})
			return
		}
		mgo.Split(c, &cl.st.splitW, func() {
			cl.listPane(c, st, k, u, caption)
		}, func() {
			cl.detailPane(c, st, k, u, caption)
		}).Grow(1).MinHeight(0)
	})
}

// snapshot returns a copy of the state for rendering.
func (cl *Client) snapshot() *state {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cp := *cl.st
	return &cp
}

// listPane draws the message list.
func (cl *Client) listPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	entries := st.msgs
	v := data.List(c, &cl.st.list, len(entries), data.ListOptions[uint32]{
		Key:   func(i int) uint32 { return entries[i].UID },
		Label: func(i int) string { return entries[i].Subject },
		Empty: "没有邮件",
	}, func(i int) {
		m := entries[i]
		mgo.Column(c).FillWidth().MinWidth(0).Gap(2).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				if m.Unread {
					mgo.Box(c).Width(6).Height(6).Shrink(0).Radius(3).Background(k.AccentText)
				}
				mgo.Text(c, m.From).FontSize(caption).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				mgo.Text(c, formatDate(m.Date)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			})
			subject := m.Subject
			if strings.TrimSpace(subject) == "" {
				subject = "(无主题)"
			}
			mgo.Text(c, subject).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
		})
	})
	v.Element.FillWidth().Grow(1).MinWidth(0).MinHeight(0)
	if v.Changed() {
		if uid, ok := cl.st.list.Selected(); ok {
			cl.openMessage(uid)
		}
	}
}

// detailPane draws the selected message.
func (cl *Client) detailPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		if st.picked == 0 {
			mgo.Column(c).Fill().Center().Children(func() {
				mgo.Text(c, "选择一封邮件查看").FontSize(caption).TextColor(k.TextMuted)
			})
			return
		}
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			mgo.Text(c, st.body.Subject).FontSize(caption).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("reply"), Label: "回复"}).Clicked() {
				cl.openReply()
			}
		})
		mgo.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().Padding(u*2).Gap(u).Children(func() {
				mgo.Text(c, "发件人: "+st.body.From).FontSize(caption).TextColor(k.TextMuted)
				if st.body.To != "" {
					mgo.Text(c, "收件人: "+st.body.To).FontSize(caption).TextColor(k.TextMuted)
				}
				if !st.body.Date.IsZero() {
					mgo.Text(c, "时间: "+st.body.Date.Format("2006-01-02 15:04")).FontSize(caption).TextColor(k.TextMuted)
				}
				mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Background(k.Border)
				if strings.TrimSpace(st.body.Text) == "" && !htmlAvailable(st.body) {
					mgo.Text(c, "(无正文)").FontSize(caption).TextColor(k.TextMuted)
					return
				}
				if cl.canRender() {
					mgo.Box(c).FillWidth().Height(600).Children(func() {
						mgo.WebView(c, cl.view).Fill()
					})
					return
				}
				if strings.TrimSpace(st.body.Text) == "" {
					mgo.Text(c, "(该邮件为 HTML，WebView 不可用)").FontSize(caption).TextColor(k.TextMuted)
					return
				}
				mgo.Text(c, st.body.Text).FontSize(caption).TextColor(k.Text)
			})
		})
	})
}

// composePane draws the compose form.
func (cl *Client) composePane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u*2).Gap(u*2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "写邮件").FontSize(caption).FontWeight(600).Grow(1)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("x"), Label: "取消"}).Clicked() {
				cl.cancelCompose()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("upload"), Label: "发送"}).Clicked() {
				cl.send()
			}
		})
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "收件人").FontSize(caption).TextColor(k.TextMuted).Width(52).Shrink(0)
			minput.TextInput(c, &cl.st.to, minput.TextInputOptions{Placeholder: "to@example.com"}).Element.Grow(1).MinWidth(0)
		})
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "主题").FontSize(caption).TextColor(k.TextMuted).Width(52).Shrink(0)
			minput.TextInput(c, &cl.st.subject, minput.TextInputOptions{Placeholder: "主题"}).Element.Grow(1).MinWidth(0)
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			minput.TextArea(c, &cl.st.text, minput.TextAreaOptions{Placeholder: "正文"}).Element.Fill()
		})
	})
}

// nextAccount switches to the next configured account.
func (cl *Client) nextAccount() {
	cl.mu.Lock()
	if len(cl.st.cfg.Accounts) < 2 {
		cl.mu.Unlock()
		return
	}
	cur := cl.st.cfg.Current
	idx := 0
	for i, a := range cl.st.cfg.Accounts {
		if a.Name == cur {
			idx = (i + 1) % len(cl.st.cfg.Accounts)
			break
		}
	}
	cl.st.cfg.Current = cl.st.cfg.Accounts[idx].Name
	cl.mu.Unlock()
	cl.refresh()
}

// refresh reloads the current mailbox.
func (cl *Client) refresh() {
	cl.mu.Lock()
	if cl.st.loading {
		cl.mu.Unlock()
		return
	}
	acc, ok := findAccount(cl.st.cfg, "")
	mailbox := cl.st.mailbox
	cl.st.loading = true
	cl.st.err = ""
	cl.mu.Unlock()
	if !ok {
		cl.mu.Lock()
		cl.st.loading = false
		cl.mu.Unlock()
		cl.host.Invalidate()
		return
	}
	go func() {
		msgs, err := listMessages(acc, mailbox, 50)
		cl.mu.Lock()
		cl.st.loading = false
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.msgs = msgs
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// openMessage loads one message's body.
func (cl *Client) openMessage(uid uint32) {
	cl.mu.Lock()
	acc, ok := findAccount(cl.st.cfg, "")
	mailbox := cl.st.mailbox
	cl.st.picked = uid
	cl.mu.Unlock()
	if !ok {
		return
	}
	cl.host.Invalidate()
	go func() {
		body, err := readMessage(acc, mailbox, uid)
		cl.mu.Lock()
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.body = body
		}
		cl.mu.Unlock()
		cl.host.Post(func() {
			cl.renderBody(uid, body)
			cl.host.Invalidate()
		})
	}()
}

// openCompose shows a blank compose form.
func (cl *Client) openCompose() {
	cl.mu.Lock()
	cl.st.composing = true
	cl.st.to = ""
	cl.st.subject = ""
	cl.st.text = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// openReply prefills the compose form for a reply.
func (cl *Client) openReply() {
	cl.mu.Lock()
	to := stBodyEmail(cl.st.body.From)
	subject := cl.st.body.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	cl.st.composing = true
	cl.st.to = to
	cl.st.subject = subject
	cl.st.text = "\n\n--- 原邮件 ---\n" + cl.st.body.Text
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// stBodyEmail extracts the bare address from a formatted sender.
func stBodyEmail(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j > 0 {
			return from[i+1 : i+j]
		}
	}
	return strings.TrimSpace(from)
}

// cancelCompose closes the compose form.
func (cl *Client) cancelCompose() {
	cl.mu.Lock()
	cl.st.composing = false
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// send sends the composed message.
func (cl *Client) send() {
	cl.mu.Lock()
	acc, ok := findAccount(cl.st.cfg, "")
	to, subject, text := cl.st.to, cl.st.subject, cl.st.text
	cl.mu.Unlock()
	if !ok {
		return
	}
	if strings.TrimSpace(to) == "" {
		cl.setErr(fmt.Errorf("请填写收件人"))
		return
	}
	go func() {
		err := sendMessage(acc, to, subject, text)
		cl.host.Post(func() {
			if err != nil {
				cl.host.Toast("发送失败: "+shortErr(err), mcore.SeverityError)
				return
			}
			cl.host.Toast("已发送", mcore.SeveritySuccess)
			cl.mu.Lock()
			cl.st.composing = false
			cl.mu.Unlock()
			cl.host.Invalidate()
		})
	}()
}

// openAccountForm shows the add-account form.
func (cl *Client) openAccountForm() {
	cl.mu.Lock()
	cl.st.adding = true
	cl.st.formProvider = -1
	cl.st.formEmail = ""
	cl.st.formPass = ""
	cl.st.formUser = ""
	cl.st.formIMAP = ""
	cl.st.formSMTP = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// closeAccountForm hides the add-account form.
func (cl *Client) closeAccountForm() {
	cl.mu.Lock()
	cl.st.adding = false
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// applyProvider fills the form from a service preset.
func (cl *Client) applyProvider(idx int) {
	cl.mu.Lock()
	cl.st.formProvider = idx
	if idx >= 0 && idx < len(providers) {
		p := providers[idx]
		cl.st.formIMAP = fmt.Sprintf("%s:%d", p.IMAPHost, p.IMAPPort)
		cl.st.formSMTP = fmt.Sprintf("%s:%d", p.SMTPHost, p.SMTPPort)
		if at := strings.Index(cl.st.formEmail, "@"); at >= 0 {
			cl.st.formEmail = cl.st.formEmail[:at] + "@" + p.Domains[0]
		} else if cl.st.formEmail == "" {
			cl.st.formEmail = "@" + p.Domains[0]
		}
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// saveAccountForm saves the form's account when it is complete.
func (cl *Client) saveAccountForm() {
	cl.mu.Lock()
	email := strings.TrimSpace(cl.st.formEmail)
	pass := cl.st.formPass
	user := strings.TrimSpace(cl.st.formUser)
	imapAddr := strings.TrimSpace(cl.st.formIMAP)
	smtpAddr := strings.TrimSpace(cl.st.formSMTP)
	providerIdx := cl.st.formProvider
	cl.mu.Unlock()
	if email == "" {
		cl.setErr(fmt.Errorf("请填写邮箱地址"))
		return
	}
	if user == "" {
		user = email
	}
	if imapAddr == "" || smtpAddr == "" {
		if providerIdx >= 0 && providerIdx < len(providers) {
			p := providers[providerIdx]
			imapAddr = fmt.Sprintf("%s:%d", p.IMAPHost, p.IMAPPort)
			smtpAddr = fmt.Sprintf("%s:%d", p.SMTPHost, p.SMTPPort)
		}
	}
	if imapAddr == "" || smtpAddr == "" {
		cl.setErr(fmt.Errorf("请填写 IMAP 与 SMTP 服务器"))
		return
	}
	acc := Account{
		Name: email, Email: email,
		IMAPHost: hostOf(imapAddr, "imap.example.com"), IMAPPort: portOf(imapAddr, 993),
		SMTPHost: hostOf(smtpAddr, "smtp.example.com"), SMTPPort: portOf(smtpAddr, 465),
		Username: user, Password: pass,
	}
	cl.mu.Lock()
	cl.st.adding = false
	cl.mu.Unlock()
	cl.saveAccount(acc)
}

// saveAccount persists an account and refreshes.
func (cl *Client) saveAccount(acc Account) {
	cl.mu.Lock()
	upsertAccount(&cl.st.cfg, acc)
	err := saveConfig(cl.st.cfg)
	cl.mu.Unlock()
	if err != nil {
		cl.setErr(err)
		return
	}
	cl.host.Toast("已添加账户 "+acc.displayName(), mcore.SeveritySuccess)
	cl.refresh()
}

// setErr shows a one-line error.
func (cl *Client) setErr(err error) {
	cl.mu.Lock()
	cl.st.err = err.Error()
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// shortErr trims an error to one line.
func shortErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 140 {
		msg = msg[:140] + "…"
	}
	return msg
}

// hostOf splits "host:port" into a host name.
func hostOf(addr, fallback string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fallback
	}
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

// portOf splits "host:port" into a port.
func portOf(addr string, fallback int) int {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, ":"); i > 0 && i < len(addr)-1 {
		port := 0
		for _, ch := range addr[i+1:] {
			if ch < '0' || ch > '9' {
				return fallback
			}
			port = port*10 + int(ch-'0')
		}
		if port > 0 {
			return port
		}
	}
	return fallback
}

// formatDate renders a message date compactly.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("01-02")
	}
	return t.Format("2006-01-02")
}

// canRender reports whether the mail body WebView is available.
func (cl *Client) canRender() bool {
	return cl.view != nil && !cl.view.IsDestroyed()
}

// ensureView creates the body WebView once the window exists.
func (cl *Client) ensureView() {
	if cl.canRender() {
		return
	}
	w := cl.host.NativeWindow()
	if w == nil {
		return
	}
	v, err := w.NewWebView(mygo.WebViewOptions{BackgroundColor: "transparent", Transparent: true})
	if err != nil {
		cl.viewErr = err.Error()
		return
	}
	cl.view = v
	v.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		if e.UserInitiated && e.URL != "" && !strings.HasPrefix(e.URL, "about:") {
			e.PreventDefault()
			cl.host.OpenFile(e.URL)
		}
	})
}

// renderBody loads a message's HTML into the body WebView.
func (cl *Client) renderBody(uid uint32, body MessageBody) {
	cl.ensureView()
	if !cl.canRender() {
		return
	}
	if cl.viewUID == uid && cl.viewUID != 0 {
		return
	}
	doc := ""
	if htmlAvailable(body) {
		doc = document(body.HTML)
	} else if strings.TrimSpace(body.Text) != "" {
		doc = plainDocument(body.Text)
	}
	if doc == "" {
		return
	}
	cl.viewUID = uid
	cl.view.Page().LoadHTML(doc, "")
}

// Close releases the body WebView.
func (cl *Client) Close() {
	if cl.view != nil && !cl.view.IsDestroyed() {
		cl.view.Destroy()
		cl.view = nil
	}
}
