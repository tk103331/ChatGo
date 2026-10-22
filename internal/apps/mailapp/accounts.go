// Package mailapp is the Apps menu's mail client: IMAP inbox browsing,
// message reading and SMTP sending with Agent tools.
package mailapp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Account is one mail account.
type Account struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	IMAPHost string `json:"imap_host"`
	IMAPPort int    `json:"imap_port"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// hostPort returns a dialable address for IMAP.
func (a Account) imapAddr() string {
	port := a.IMAPPort
	if port == 0 {
		port = 993
	}
	return fmt.Sprintf("%s:%d", a.IMAPHost, port)
}

// smtpAddr returns a dialable address for SMTP.
func (a Account) smtpAddr() string {
	port := a.SMTPPort
	if port == 0 {
		port = 465
	}
	return fmt.Sprintf("%s:%d", a.SMTPHost, port)
}

// mailConfig is the on-disk account list.
type mailConfig struct {
	Accounts []Account `json:"accounts"`
	Current  string    `json:"current"`
}

// configPath returns ~/.chatgo/mail.json.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "mail.json"), nil
}

// loadConfig reads the saved accounts, or an empty config.
func loadConfig() mailConfig {
	var cfg mailConfig
	path, err := configPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// saveConfig writes the accounts with owner-only permissions.
func saveConfig(cfg mailConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// findAccount returns the named account, or the current one when name is
// empty.
func findAccount(cfg mailConfig, name string) (Account, bool) {
	want := name
	if want == "" {
		want = cfg.Current
	}
	for _, a := range cfg.Accounts {
		if a.Name == want || a.Email == want {
			return a, true
		}
	}
	if want == cfg.Current && len(cfg.Accounts) > 0 {
		return cfg.Accounts[0], true
	}
	return Account{}, false
}

// accountNames lists the account display names.
func accountNames(cfg mailConfig) []string {
	out := make([]string, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		name := a.Name
		if name == "" {
			name = a.Email
		}
		out = append(out, name)
	}
	return out
}

// upsertAccount adds or replaces an account by name.
func upsertAccount(cfg *mailConfig, acc Account) {
	for i, a := range cfg.Accounts {
		if a.Name == acc.Name {
			cfg.Accounts[i] = acc
			cfg.Current = acc.Name
			return
		}
	}
	cfg.Accounts = append(cfg.Accounts, acc)
	cfg.Current = acc.Name
}

// removeAccount drops an account by name.
func removeAccount(cfg *mailConfig, name string) {
	out := cfg.Accounts[:0]
	for _, a := range cfg.Accounts {
		if a.Name != name {
			out = append(out, a)
		}
	}
	cfg.Accounts = out
	if cfg.Current == name {
		cfg.Current = ""
		if len(cfg.Accounts) > 0 {
			cfg.Current = cfg.Accounts[0].Name
		}
	}
}

// displayName is the account's label in the UI.
func (a Account) displayName() string {
	if a.Name != "" {
		return a.Name
	}
	if a.Email != "" {
		return a.Email
	}
	return strings.TrimSpace(a.IMAPHost)
}
