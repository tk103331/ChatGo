package mailapp

import "strings"

// Provider is a mail service preset.
type Provider struct {
	Name      string
	Domains   []string
	IMAPHost  string
	IMAPPort  int
	SMTPHost  string
	SMTPPort  int
	// PasswordHint tells the user which credential the service requires.
	PasswordHint string
}

// providers are the common mail services, matched by address domain.
var providers = []Provider{
	{Name: "网易163", Domains: []string{"163.com"}, IMAPHost: "imap.163.com", IMAPPort: 993,
		SMTPHost: "smtp.163.com", SMTPPort: 465, PasswordHint: "客户端授权码"},
	{Name: "网易126", Domains: []string{"126.com"}, IMAPHost: "imap.126.com", IMAPPort: 993,
		SMTPHost: "smtp.126.com", SMTPPort: 465, PasswordHint: "客户端授权码"},
	{Name: "网易yeah", Domains: []string{"yeah.net"}, IMAPHost: "imap.yeah.net", IMAPPort: 993,
		SMTPHost: "smtp.yeah.net", SMTPPort: 465, PasswordHint: "客户端授权码"},
	{Name: "腾讯QQ邮箱", Domains: []string{"qq.com", "foxmail.com"}, IMAPHost: "imap.qq.com", IMAPPort: 993,
		SMTPHost: "smtp.qq.com", SMTPPort: 465, PasswordHint: "授权码"},
	{Name: "腾讯企业邮", Domains: []string{"exmail.qq.com"}, IMAPHost: "imap.exmail.qq.com", IMAPPort: 993,
		SMTPHost: "smtp.exmail.qq.com", SMTPPort: 465, PasswordHint: "客户端专用密码"},
	{Name: "Gmail", Domains: []string{"gmail.com", "googlemail.com"}, IMAPHost: "imap.gmail.com", IMAPPort: 993,
		SMTPHost: "smtp.gmail.com", SMTPPort: 465, PasswordHint: "应用专用密码"},
	{Name: "Outlook", Domains: []string{"outlook.com", "hotmail.com", "live.com", "msn.com"},
		IMAPHost: "outlook.office365.com", IMAPPort: 993,
		SMTPHost: "smtp.office365.com", SMTPPort: 587, PasswordHint: "账户密码或应用密码"},
	{Name: "iCloud", Domains: []string{"icloud.com", "me.com", "mac.com"}, IMAPHost: "imap.mail.me.com", IMAPPort: 993,
		SMTPHost: "smtp.mail.me.com", SMTPPort: 587, PasswordHint: "App 专用密码"},
	{Name: "Yahoo", Domains: []string{"yahoo.com"}, IMAPHost: "imap.mail.yahoo.com", IMAPPort: 993,
		SMTPHost: "smtp.mail.yahoo.com", SMTPPort: 465, PasswordHint: "应用密码"},
	{Name: "阿里企业邮", Domains: []string{"aliyun.com"}, IMAPHost: "imap.qiye.aliyun.com", IMAPPort: 993,
		SMTPHost: "smtp.qiye.aliyun.com", SMTPPort: 465, PasswordHint: "客户端密码"},
}

// providerFor finds the preset matching an address domain.
func providerFor(email string) (Provider, bool) {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return Provider{}, false
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	for _, p := range providers {
		for _, d := range p.Domains {
			if d == domain {
				return p, true
			}
		}
	}
	return Provider{}, false
}

// providerNames lists preset names for the help text.
func providerNames() string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
	}
	return strings.Join(names, "、")
}
