package mailapp

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/smtp"
	"sort"
	"strings"
	"time"

	imap "github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

// Message is one mail header shown in the list.
type Message struct {
	UID     uint32
	From    string
	Subject string
	Date    time.Time
	SeqNum  uint32
	Unread  bool
}

// MessageBody is a fully read message.
type MessageBody struct {
	Message
	To   string
	CC   string
	Text string
	HTML string
}

// dial opens an IMAP connection over TLS.
func dial(acc Account) (*imapclient.Client, error) {
	c, err := imapclient.DialTLS(acc.imapAddr(), &tls.Config{ServerName: acc.IMAPHost})
	if err != nil {
		return nil, err
	}
	if err := c.Login(acc.Username, acc.Password); err != nil {
		_ = c.Logout()
		return nil, err
	}
	return c, nil
}

// listMessages returns the newest messages of a mailbox.
func listMessages(acc Account, mailbox string, limit int) ([]Message, error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if limit <= 0 {
		limit = 50
	}
	c, err := dial(acc)
	if err != nil {
		return nil, err
	}
	defer c.Logout()
	mbox, err := c.Select(mailbox, true)
	if err != nil {
		return nil, err
	}
	if mbox.Messages == 0 {
		return nil, nil
	}
	from := uint32(1)
	if mbox.Messages > uint32(limit) {
		from = mbox.Messages - uint32(limit) + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, section.FetchItem()}
	ch := make(chan *imap.Message, 32)
	done := make(chan error, 1)
	go func() { done <- c.Fetch(seqset, items, ch) }()
	var out []Message
	for msg := range ch {
		if msg == nil {
			continue
		}
		m := Message{UID: msg.Uid, SeqNum: msg.SeqNum}
		if msg.Envelope != nil {
			m.Subject = msg.Envelope.Subject
			if len(msg.Envelope.From) > 0 {
				m.From = formatAddress(msg.Envelope.From[0])
			}
			m.Date = msg.Envelope.Date
		}
		m.Unread = !hasFlag(msg.Flags, imap.SeenFlag)
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeqNum > out[j].SeqNum })
	return out, nil
}

// hasFlag reports whether flags contains want.
func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, want) {
			return true
		}
	}
	return false
}

// formatAddress renders one envelope address.
func formatAddress(a *imap.Address) string {
	if a == nil {
		return ""
	}
	name := a.PersonalName
	if name == "" {
		return a.MailboxName + "@" + a.HostName
	}
	return fmt.Sprintf("%s <%s@%s>", name, a.MailboxName, a.HostName)
}

// readMessage fetches one message's headers and text body.
func readMessage(acc Account, mailbox string, uid uint32) (MessageBody, error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	c, err := dial(acc)
	if err != nil {
		return MessageBody{}, err
	}
	defer c.Logout()
	if _, err := c.Select(mailbox, false); err != nil {
		return MessageBody{}, err
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, section.FetchItem()}
	ch := make(chan *imap.Message, 2)
	done := make(chan error, 1)
	go func() { done <- c.Fetch(seqset, items, ch) }()
	var body MessageBody
	var raw io.Reader
	for msg := range ch {
		if msg == nil {
			continue
		}
		body.UID = msg.Uid
		if msg.Envelope != nil {
			body.Subject = msg.Envelope.Subject
			body.Date = msg.Envelope.Date
			if len(msg.Envelope.From) > 0 {
				body.From = formatAddress(msg.Envelope.From[0])
			}
			body.To = formatAddressList(msg.Envelope.To)
			body.CC = formatAddressList(msg.Envelope.Cc)
		}
		if r := msg.GetBody(section); r != nil {
			raw = r
		}
	}
	if err := <-done; err != nil {
		return MessageBody{}, err
	}
	if raw != nil {
		body.Text, body.HTML = extractBodies(raw)
	}
	return body, nil
}

// formatAddressList renders address lists joined by commas.
func formatAddressList(list []*imap.Address) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		parts = append(parts, formatAddress(a))
	}
	return strings.Join(parts, ", ")
}

// extractText returns the message's plain-text body.
func extractText(r io.Reader) string {
	text, _ := extractBodies(r)
	return text
}

// extractBodies returns a message's plain-text and HTML bodies.
func extractBodies(r io.Reader) (text, html string) {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		data, _ := io.ReadAll(r)
		return string(data), ""
	}
	ctype := msg.Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(ctype)
	if strings.HasPrefix(mediaType, "multipart/") {
		return bodiesFromMultipart(msg.Body, params["boundary"], 0)
	}
	data, _ := io.ReadAll(io.LimitReader(msg.Body, 4<<20))
	decoded := decodeBody(msg.Header.Get("Content-Transfer-Encoding"), string(data))
	if mediaType == "text/html" {
		return "", decoded
	}
	return decoded, ""
}

// bodiesFromMultipart walks a multipart body collecting text/plain and
// text/html parts, recursing into nested multiparts.
func bodiesFromMultipart(r io.Reader, boundary string, depth int) (text, html string) {
	if boundary == "" || depth > 4 {
		return "", ""
	}
	mr := multipart.NewReader(r, boundary)
	for {
		p, err := mr.NextPart()
		if err != nil {
			return text, html
		}
		mediaType, params, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if strings.HasPrefix(mediaType, "multipart/") {
			t, h := bodiesFromMultipart(p, params["boundary"], depth+1)
			if text == "" {
				text = t
			}
			if html == "" {
				html = h
			}
			continue
		}
		switch mediaType {
		case "text/plain":
			if text == "" {
				data, _ := io.ReadAll(io.LimitReader(p, 4<<20))
				text = decodeBody(p.Header.Get("Content-Transfer-Encoding"), string(data))
			}
		case "text/html":
			if html == "" {
				data, _ := io.ReadAll(io.LimitReader(p, 4<<20))
				html = decodeBody(p.Header.Get("Content-Transfer-Encoding"), string(data))
			}
		}
	}
}

// decodeBody applies base64 or quoted-printable decoding.
func decodeBody(encoding, s string) string {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s)); err == nil {
			return string(raw)
		}
	case "quoted-printable":
		if raw, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(s))); err == nil {
			return string(raw)
		}
	}
	return s
}

// sendMessage sends a plain-text mail over SMTP.
func sendMessage(acc Account, to, subject, body string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("缺少收件人")
	}
	from := mail.Address{Name: acc.displayName(), Address: acc.Email}
	rcpt := mail.Address{Address: strings.TrimSpace(to)}
	msg := buildMessage(from, rcpt, subject, body)
	auth := smtp.PlainAuth("", acc.Username, acc.Password, acc.SMTPHost)

	addr := acc.smtpAddr()
	if acc.SMTPPort == 587 || acc.SMTPPort == 25 {
		c, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer c.Close()
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: acc.SMTPHost}); err != nil {
				return err
			}
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
		return deliver(c, from.Address, rcpt.Address, msg)
	}

	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: acc.SMTPHost})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, acc.SMTPHost)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Auth(auth); err != nil {
		return err
	}
	return deliver(c, from.Address, rcpt.Address, msg)
}

// deliver runs the SMTP conversation for one built message.
func deliver(c *smtp.Client, from, to string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// buildMessage assembles a UTF-8 plain-text message.
func buildMessage(from, to mail.Address, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to.String())
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// listMailboxes returns the account's mailbox names.
func listMailboxes(acc Account) ([]string, error) {
	c, err := dial(acc)
	if err != nil {
		return nil, err
	}
	defer c.Logout()
	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() { done <- c.List("", "*", ch) }()
	var out []string
	for info := range ch {
		if info != nil {
			out = append(out, info.Name)
		}
	}
	if err := <-done; err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
