package mailapp

import (
	"strings"
	"testing"
	"time"
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
