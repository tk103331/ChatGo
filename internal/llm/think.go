package llm

import "strings"

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// thinkStream separates inline  thinking blocks from visible text as chunks
// arrive. Some OpenAI-compatible providers put the model's reasoning in the
// content stream instead of reasoning_content, wrapped in <think> tags; the
// parser keeps a partial tag in the buffer until the next chunk completes it.
type thinkStream struct {
	buf     string
	inThink bool
}

func (s *thinkStream) feed(text string, onContent, onReasoning func(string)) {
	if text == "" {
		return
	}
	s.buf += text
	s.drain(false, onContent, onReasoning)
}

func (s *thinkStream) flush(onContent, onReasoning func(string)) {
	s.drain(true, onContent, onReasoning)
}

func (s *thinkStream) drain(final bool, onContent, onReasoning func(string)) {
	for {
		tag, emit := thinkOpen, onContent
		if s.inThink {
			tag, emit = thinkClose, onReasoning
		}
		if i := strings.Index(s.buf, tag); i >= 0 {
			if i > 0 {
				emit(s.buf[:i])
			}
			s.buf = s.buf[i+len(tag):]
			s.inThink = !s.inThink
			continue
		}
		if final {
			if s.buf != "" {
				emit(s.buf)
				s.buf = ""
			}
			return
		}
		hold := heldSuffix(s.buf, tag)
		if n := len(s.buf) - hold; n > 0 {
			emit(s.buf[:n])
			s.buf = s.buf[n:]
		}
		return
	}
}

// heldSuffix is the longest suffix of s that could still become tag.
func heldSuffix(s, tag string) int {
	max := len(tag) - 1
	if len(s) < max {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}

// splitThinkContent moves <think> blocks out of content and into reasoning.
// It is the non-streaming counterpart of thinkStream.
func splitThinkContent(content, reasoning string) (string, string) {
	var text, thought strings.Builder
	st := thinkStream{}
	st.feed(content, func(s string) { text.WriteString(s) }, func(s string) { thought.WriteString(s) })
	st.flush(func(s string) { text.WriteString(s) }, func(s string) { thought.WriteString(s) })
	if thought.Len() > 0 && !strings.Contains(reasoning, thought.String()) {
		if strings.TrimSpace(reasoning) != "" {
			reasoning = strings.TrimRight(reasoning, "\n") + "\n" + thought.String()
		} else {
			reasoning = thought.String()
		}
	}
	return text.String(), stripThinkTags(reasoning)
}

func stripThinkTags(s string) string {
	if !strings.Contains(s, "<think") {
		return s
	}
	s = strings.ReplaceAll(s, thinkOpen, "")
	return strings.ReplaceAll(s, thinkClose, "")
}
