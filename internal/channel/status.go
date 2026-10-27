package channel

import (
	"fmt"
	"strings"
)

// Summary is a connection state rendered for a person: the words come from one
// place for every platform, so a Telegram channel and a DingTalk one describe
// the same condition the same way. A driver supplies only the fact — the bot's
// name, the platform's error — never the sentence around it.
type Summary struct {
	// State is the lifecycle state the summary describes.
	State State
	// Label is the short state word: 已连接, 连接中, 重连中, 连接失败, 未连接.
	Label string
	// Detail is the platform's own fact, empty when there is none.
	Detail string
}

// String renders the summary as one line: "已连接 · @chatgo_bot".
func (s Summary) String() string {
	if s.Detail == "" {
		return s.Label
	}
	return s.Label + " · " + s.Detail
}

// Summarize builds the canonical summary for a state and a driver's detail.
// Drivers pass the fact only; this decides the wording, so the same condition
// never reads two ways across platforms.
func Summarize(st State, detail string) Summary {
	detail = strings.TrimSpace(detail)
	switch st {
	case StateConnected:
		return Summary{State: st, Label: "已连接", Detail: detail}
	case StateConnecting:
		return Summary{State: st, Label: "连接中"}
	case StateReconnecting:
		return Summary{State: st, Label: "重连中", Detail: detail}
	case StateError:
		return Summary{State: st, Label: "连接失败", Detail: detail}
	default:
		return Summary{State: st, Label: "未连接"}
	}
}

// SummaryFor is Summarize for a driver, reporting the stopped state when the
// driver does not track one.
func SummaryFor(d Driver) Summary {
	if reporter, ok := d.(StatusReporter); ok {
		st, detail := reporter.Status()
		return Summarize(st, detail)
	}
	return Summarize(StateStopped, "")
}

// DetailConnected names the identity a platform confirmed, in one form across
// drivers: a bot name where the platform has one, empty otherwise.
func DetailConnected(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// A bot name reads best with its @ prefix on the platforms that use one;
	// the drivers pass the bare name and this adds the mark.
	if strings.HasPrefix(name, "@") {
		return name
	}
	return "@" + name
}

// DetailError keeps a platform error to one line, so a multi-line server
// response does not break the settings card's layout.
func DetailError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	// The limit counts characters, not bytes: a Chinese error is three bytes
	// per character and would otherwise be cut far too early.
	const limit = 160
	runes := []rune(text)
	if len(runes) > limit {
		text = string(runes[:limit]) + "…"
	}
	return text
}

// Detailf formats a detail string, keeping the same one-line rule.
func Detailf(format string, args ...any) string {
	return DetailError(fmt.Errorf(format, args...))
}
