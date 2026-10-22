package mailapp

import (
	"strings"
	"testing"
	"time"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

func TestHostPortParsing(t *testing.T) {
	if got := hostOf("imap.example.com:993", "x"); got != "imap.example.com" {
		t.Errorf("hostOf = %q", got)
	}
	if got := hostOf("", "imap.fallback"); got != "imap.fallback" {
		t.Errorf("hostOf empty = %q", got)
	}
	if got := portOf("imap.example.com:143", 993); got != 143 {
		t.Errorf("portOf = %d", got)
	}
	if got := portOf("bad", 465); got != 465 {
		t.Errorf("portOf bad = %d", got)
	}
}

func TestAccountAddr(t *testing.T) {
	a := Account{IMAPHost: "imap.x.com", SMTPHost: "smtp.x.com"}
	if got := a.imapAddr(); got != "imap.x.com:993" {
		t.Errorf("imapAddr = %q", got)
	}
	if got := a.smtpAddr(); got != "smtp.x.com:465" {
		t.Errorf("smtpAddr = %q", got)
	}
	a.IMAPPort, a.SMTPPort = 143, 587
	if got := a.imapAddr(); got != "imap.x.com:143" {
		t.Errorf("imapAddr = %q", got)
	}
	if got := a.smtpAddr(); got != "smtp.x.com:587" {
		t.Errorf("smtpAddr = %q", got)
	}
}

func TestUpsertRemoveAccount(t *testing.T) {
	var cfg mailConfig
	upsertAccount(&cfg, Account{Name: "a", Email: "a@x.com"})
	upsertAccount(&cfg, Account{Name: "b", Email: "b@x.com"})
	if len(cfg.Accounts) != 2 || cfg.Current != "b" {
		t.Fatalf("cfg = %+v", cfg)
	}
	upsertAccount(&cfg, Account{Name: "a", Email: "a2@x.com"})
	if len(cfg.Accounts) != 2 || cfg.Accounts[0].Email != "a2@x.com" || cfg.Current != "a" {
		t.Fatalf("upsert replace failed: %+v", cfg)
	}
	removeAccount(&cfg, "a")
	if len(cfg.Accounts) != 1 || cfg.Current != "b" {
		t.Fatalf("remove failed: %+v", cfg)
	}
}

func TestExtractText(t *testing.T) {
	raw := "From: a@x.com\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhello world"
	if got := extractText(strings.NewReader(raw)); got != "hello world" {
		t.Errorf("extractText = %q", got)
	}
	multi := "Content-Type: multipart/alternative; boundary=BB\r\n\r\n--BB\r\nContent-Type: text/html\r\n\r\n<p>x</p>\r\n--BB\r\nContent-Type: text/plain\r\n\r\nplain body\r\n--BB--\r\n"
	if got := extractText(strings.NewReader(multi)); got != "plain body\r\n" && got != "plain body" {
		t.Errorf("multipart extractText = %q", got)
	}
}

func TestStBodyEmail(t *testing.T) {
	cases := map[string]string{
		"Name <a@x.com>": "a@x.com",
		"a@x.com":        "a@x.com",
		"":               "",
	}
	for in, want := range cases {
		if got := stBodyEmail(in); got != want {
			t.Errorf("stBodyEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	now := time.Now()
	if got := formatDate(now); len(got) != 5 {
		t.Errorf("today format = %q, want HH:MM", got)
	}
	old := time.Date(now.Year()-1, 3, 4, 5, 6, 0, 0, time.Local)
	if got := formatDate(old); got != old.Format("2006-01-02") {
		t.Errorf("old format = %q", got)
	}
	if got := formatDate(time.Time{}); got != "" {
		t.Errorf("zero format = %q", got)
	}
}

func TestProviderFor(t *testing.T) {
	cases := []struct {
		email    string
		wantName string
		imap     string
		smtp     string
		smtpPort int
	}{
		{"me@163.com", "网易163", "imap.163.com", "smtp.163.com", 465},
		{"me@126.com", "网易126", "imap.126.com", "smtp.126.com", 465},
		{"me@qq.com", "腾讯QQ邮箱", "imap.qq.com", "smtp.qq.com", 465},
		{"me@foxmail.com", "腾讯QQ邮箱", "imap.qq.com", "smtp.qq.com", 465},
		{"me@gmail.com", "Gmail", "imap.gmail.com", "smtp.gmail.com", 465},
		{"me@outlook.com", "Outlook", "outlook.office365.com", "smtp.office365.com", 587},
		{"me@hotmail.com", "Outlook", "outlook.office365.com", "smtp.office365.com", 587},
		{"me@icloud.com", "iCloud", "imap.mail.me.com", "smtp.mail.me.com", 587},
	}
	for _, tc := range cases {
		p, ok := providerFor(tc.email)
		if !ok {
			t.Errorf("providerFor(%q) not found", tc.email)
			continue
		}
		if p.Name != tc.wantName || p.IMAPHost != tc.imap || p.SMTPHost != tc.smtp || p.SMTPPort != tc.smtpPort {
			t.Errorf("providerFor(%q) = %+v", tc.email, p)
		}
	}
	if _, ok := providerFor("me@unknown-domain.test"); ok {
		t.Error("unknown domain should not match")
	}
	if _, ok := providerFor("no-at-sign"); ok {
		t.Error("address without @ should not match")
	}
}

func TestProviderPasswordHints(t *testing.T) {
	for _, email := range []string{"a@163.com", "a@qq.com", "a@gmail.com", "a@outlook.com"} {
		p, ok := providerFor(email)
		if !ok || p.PasswordHint == "" {
			t.Errorf("%s should have a password hint", email)
		}
	}
}

func TestExtractBodiesMultipartAlternative(t *testing.T) {
	raw := "Content-Type: multipart/alternative; boundary=BB\r\n\r\n" +
		"--BB\r\nContent-Type: text/plain\r\n\r\nplain version\r\n" +
		"--BB\r\nContent-Type: text/html\r\n\r\n<p>html <b>version</b></p>\r\n" +
		"--BB--\r\n"
	text, htmlBody := extractBodies(strings.NewReader(raw))
	if !strings.Contains(text, "plain version") {
		t.Errorf("text = %q", text)
	}
	if !strings.Contains(htmlBody, "<b>version</b>") {
		t.Errorf("html = %q", htmlBody)
	}
}

func TestExtractBodiesNested(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=OUT\r\n\r\n" +
		"--OUT\r\nContent-Type: multipart/alternative; boundary=IN\r\n\r\n" +
		"--IN\r\nContent-Type: text/plain\r\n\r\nnested plain\r\n" +
		"--IN\r\nContent-Type: text/html\r\n\r\n<div>nested html</div>\r\n" +
		"--IN--\r\n" +
		"--OUT\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=a.pdf\r\n\r\nPDF\r\n" +
		"--OUT--\r\n"
	text, htmlBody := extractBodies(strings.NewReader(raw))
	if !strings.Contains(text, "nested plain") {
		t.Errorf("text = %q", text)
	}
	if !strings.Contains(htmlBody, "nested html") {
		t.Errorf("html = %q", htmlBody)
	}
}

func TestExtractBodiesHTMLOnly(t *testing.T) {
	raw := "Content-Type: text/html; charset=utf-8\r\n\r\n<h1>only html</h1>"
	text, htmlBody := extractBodies(strings.NewReader(raw))
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if !strings.Contains(htmlBody, "only html") {
		t.Errorf("html = %q", htmlBody)
	}
}

func TestDocumentSafety(t *testing.T) {
	doc := document("<p>hi</p>")
	if !strings.Contains(doc, `default-src 'none'`) {
		t.Error("document missing CSP")
	}
	if !strings.Contains(doc, "<p>hi</p>") {
		t.Error("document missing body")
	}
	plain := plainDocument("<script>x</script>")
	if strings.Contains(plain, "<script>") {
		t.Error("plain text must be escaped")
	}
	if !strings.Contains(plain, "&lt;script&gt;") {
		t.Error("plain text not escaped")
	}
	if !htmlAvailable(MessageBody{HTML: "<p>x</p>"}) {
		t.Error("htmlAvailable should be true")
	}
	if htmlAvailable(MessageBody{Text: "x"}) {
		t.Error("htmlAvailable should be false for text-only")
	}
}

type fakeHost struct{}

func (fakeHost) Root() string                            { return "" }
func (fakeHost) Workspace() string                       { return "" }
func (fakeHost) Post(fn func())                          { fn() }
func (fakeHost) Invalidate()                             {}
func (fakeHost) Toast(string, mcore.Severity)            {}
func (fakeHost) Confirm(_, _ string, a func())           { if a != nil { a() } }
func (fakeHost) Prompt(_, _, _ string, a func(string))   { if a != nil { a("") } }
func (fakeHost) OpenFile(string)                         {}
func (fakeHost) OpenFileDialog(string) string            { return "" }
func (fakeHost) SaveFileDialog(string) string            { return "" }
func (fakeHost) Language(ext string) string              { return "" }
func (fakeHost) NewID() string                           { return "" }
func (fakeHost) NativeWindow() *mygo.Window              { return nil }

func TestAccountFormRenders(t *testing.T) {
	cl := New(fakeHost{})
	cl.st.adding = true
	cl.st.formProvider = 0
	cl.st.formEmail = "me@163.com"
	img := mgo.Render(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Mode: mcore.System})
		cl.View(c)
	}, 900, 700, 1)
	if img == nil {
		t.Fatal("account form render returned nil")
	}
}

func TestApplyProviderFillsForm(t *testing.T) {
	cl := New(fakeHost{})
	cl.st.formEmail = "someone@163.com"
	cl.applyProvider(3)
	if cl.st.formIMAP != "imap.qq.com:993" || cl.st.formSMTP != "smtp.qq.com:465" {
		t.Fatalf("form = %q %q", cl.st.formIMAP, cl.st.formSMTP)
	}
	if cl.st.formEmail != "someone@qq.com" {
		t.Fatalf("email = %q, want someone@qq.com", cl.st.formEmail)
	}
	if cl.st.formProvider != 3 {
		t.Fatalf("provider = %d", cl.st.formProvider)
	}
}
