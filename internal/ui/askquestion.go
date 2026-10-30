package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	"chatgo/pkg/models"
)

// askQuestionTimeout bounds how long a question waits for an answer, so a
// forgotten dialog cannot hold the turn forever.
const askQuestionTimeout = 10 * time.Minute

// askQuestionItem is one question of a pending ask, with the answer chosen so
// far.
type askQuestionItem struct {
	question string
	options  []string
	value    string
	answer   string
}

type builtinToolEnv struct {
	channel *config.Channel
	msgCtx  *channelMsgContext
}

// askQuestion is one pending ask from an agent, blocking its tool call until
// the user answers or cancels.
type askQuestion struct {
	items  []*askQuestionItem
	reply  chan string
	convID string
}

// answered reports whether every question has an answer.
func (q *askQuestion) answered() bool {
	for _, it := range q.items {
		if strings.TrimSpace(it.answer) == "" {
			return false
		}
	}
	return len(q.items) > 0
}

// result formats the answers for the tool result.
func (q *askQuestion) result() string {
	if len(q.items) == 1 {
		return q.items[0].answer
	}
	var b strings.Builder
	for i, it := range q.items {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("问题 %d：%s\n回答：%s", i+1, it.question, it.answer))
	}
	return b.String()
}

// askQuestionTool asks the user one or more questions and returns the
// answers as the tool result. It is the agent's way to stop and check a
// decision instead of guessing.
func (cw *ChatWindow) askQuestionTool(env *builtinToolEnv, conv ...*models.Conversation) func(context.Context, string) (string, error) {
	convID := ""
	if len(conv) > 0 && conv[0] != nil {
		convID = conv[0].ID
	}
	return func(ctx context.Context, args string) (string, error) {
		var in struct {
			Questions []struct {
				Question string   `json:"question"`
				Options  []string `json:"options"`
			} `json:"questions"`
			Question string   `json:"question"`
			Options  []string `json:"options"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
		items := make([]*askQuestionItem, 0, len(in.Questions)+1)
		add := func(question string, options []string) {
			question = strings.TrimSpace(question)
			if question == "" {
				return
			}
			opts := make([]string, 0, len(options))
			for _, o := range options {
				if s := strings.TrimSpace(o); s != "" {
					opts = append(opts, s)
				}
			}
			items = append(items, &askQuestionItem{question: question, options: opts})
		}
		for _, q := range in.Questions {
			add(q.Question, q.Options)
		}
		if len(items) == 0 {
			add(in.Question, in.Options)
		}
		if len(items) == 0 {
			return "", fmt.Errorf("缺少必填参数 questions")
		}

		if env != nil && env.channel != nil {
			return cw.askChannelQuestion(ctx, env, convID, items)
		}

		if !cw.hasUI() {
			// Without a window the panel never renders, so waiting would
			// hang the tool call forever.
			return "", fmt.Errorf("当前没有可交互界面，无法向用户提问，请自行判断后继续")
		}
		reply := make(chan string, 1)
		ready := make(chan struct{})
		cw.postMain(func() {
			defer close(ready)
			if convID == "" && cw.cur != nil {
				convID = cw.cur.ID
			}
			cw.askQ = &askQuestion{items: items, reply: reply, convID: convID}
			cw.invalidate()
			cw.notifyAttention(convID, "Agent 提问", items[0].question)
		})
		<-ready

		ctx, cancel := context.WithTimeout(ctx, askQuestionTimeout)
		defer cancel()
		select {
		case answer := <-reply:
			return answer, nil
		case <-ctx.Done():
			cw.postMain(func() {
				cw.askQ = nil
				cw.invalidate()
			})
			return "", fmt.Errorf("提问未获回答")
		}
	}
}

// askQuestionPanel shows the pending questions above the composer, in the
// same card style as the plan panel: one tab per question, options as
// buttons, and a free-text field for an answer none of them cover.
func (cw *ChatWindow) askQuestionPanel(c *mgo.Context) {
	q := cw.askQ
	if q == nil || (q.convID != "" && (cw.cur == nil || cw.cur.ID != q.convID)) {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)

	tab := mgo.Local(c.Root(), "askq:tab", func() int { return 0 })
	if *tab >= len(q.items) {
		*tab = 0
	}
	tabs := make([]kit.Tab, 0, len(q.items))
	for i, it := range q.items {
		label := fmt.Sprintf("问题 %d", i+1)
		if strings.TrimSpace(it.answer) != "" {
			label += " ✓"
		}
		tabs = append(tabs, kit.Tab{Label: label})
	}

	cancelled, submitted := false, false
	mgo.Column(c).FillWidth().Shrink(0).AlignItems(mgo.Start).Padding(u*2, u*4).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Radius(mtheme.ControlRadius).
			Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
			Padding(u * 2).Gap(u).Children(func() {
			mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("message-square")).Size(14, 14).
					Shrink(0).TextColor(k.Accent)
				mgo.Text(c, "Agent 提问").FontWeight(600).FontSize(caption).Grow(1)
				if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					cancelled = true
				}
			})
			if len(tabs) > 1 {
				kit.Tabs(c, tab, tabs, kit.TabsOptions{})
			}
			it := q.items[*tab]
			mgo.Column(c).FillWidth().Gap(u).Padding(u).Radius(mtheme.ControlRadius).
				Background(k.Background).Border(mtheme.BorderWidth, k.Border).Children(func() {
				mgo.Text(c, it.question).FontSize(mcore.FontSize(c, mtheme.BodySize)).
					TextColor(k.Text).LineHeight(1.5)
				for _, opt := range it.options {
					opt := opt
					chosen := it.answer == opt
					b := mgo.ButtonBase(c).FillWidth().Gap(u).AlignItems(mgo.Center).
						Padding(u/2, u).Radius(mtheme.ControlRadius).
						Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
						Label(opt)
					if chosen {
						b.Background(k.Selection)
					} else if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					b.Children(func() {
						if chosen {
							mgo.Icon(c, icons.Must("circle-check")).Size(14, 14).
								Shrink(0).TextColor(k.Success)
						}
						mgo.Text(c, opt).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
							FontSize(caption)
					})
					if b.Clicked() {
						it.answer = opt
					}
				}
				in := kit.TextInput(c, &it.value, kit.TextInputOptions{
					Label: "回答", Placeholder: "输入你的回答",
				})
				if in.Changed() {
					// Typing overrides a picked option, so the field is the
					// answer rather than a stale choice.
					it.answer = strings.TrimSpace(it.value)
				}
				if in.Submitted() {
					if v := strings.TrimSpace(it.value); v != "" {
						it.answer = v
						if *tab < len(q.items)-1 {
							*tab++
						}
					}
				}
				if len(it.options) > 0 {
					mgo.Text(c, "或直接输入回答").FontSize(caption).TextColor(k.TextMuted)
				}
			})
			mgo.Row(c).FillWidth().Justify(mgo.End).Children(func() {
				if kit.Button(c, "提交", kit.ButtonOptions{
					Variant: kit.Primary, Disabled: !q.answered(),
				}).Clicked() && q.answered() {
					submitted = true
				}
			})
		})
	})

	if !cancelled && !submitted {
		return
	}
	cw.askQ = nil
	if cancelled {
		q.reply <- "用户取消了提问"
	} else {
		q.reply <- q.result()
	}
	cw.invalidate()
}

// askChannelQuestion sends the questions through the channel that delivered
// the message and blocks until its user replies or the timeout expires.
func (cw *ChatWindow) askChannelQuestion(ctx context.Context, env *builtinToolEnv, convID string, items []*askQuestionItem) (string, error) {
	if convID == "" {
		return "", fmt.Errorf("渠道会话缺少会话 ID，无法提问")
	}
	key := channelAskKey(env.channel.ID, env.msgCtx)
	q := &askQuestion{items: items, reply: make(chan string, 1), convID: convID}
	cw.askMu.Lock()
	if cw.channelAsks == nil {
		cw.channelAsks = map[string]*askQuestion{}
	}
	cw.channelAsks[key] = q
	cw.askMu.Unlock()
	if err := cw.channelSendText(env.channel, env.msgCtx, channelQuestionText(items)); err != nil {
		cw.dropChannelAsk(key, q)
		return "", fmt.Errorf("发送提问失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, askQuestionTimeout)
	defer cancel()
	select {
	case answer := <-q.reply:
		return answer, nil
	case <-ctx.Done():
		cw.dropChannelAsk(key, q)
		return "", fmt.Errorf("提问未获回答")
	}
}

func (cw *ChatWindow) dropChannelAsk(key string, q *askQuestion) {
	cw.askMu.Lock()
	if cw.channelAsks[key] == q {
		delete(cw.channelAsks, key)
	}
	cw.askMu.Unlock()
}

// channelAskKey scopes a pending question to the chat that asked it, so two
// users of one bot never take each other's answer.
func channelAskKey(channelID string, msgCtx *channelMsgContext) string {
	chatID := ""
	if msgCtx != nil {
		chatID = strings.TrimSpace(msgCtx.ChatID)
	}
	return channelID + "\x00" + chatID
}

// deliverChannelAsk hands a channel message to the question waiting on that
// chat, and reports whether it took it.
func (cw *ChatWindow) deliverChannelAsk(channelID string, msgCtx *channelMsgContext, text string) bool {
	key := channelAskKey(channelID, msgCtx)
	cw.askMu.Lock()
	q := cw.channelAsks[key]
	if q != nil {
		delete(cw.channelAsks, key)
	}
	cw.askMu.Unlock()
	if q == nil {
		return false
	}
	select {
	case q.reply <- text:
	default:
	}
	return true
}

func (cw *ChatWindow) channelAskWaiting(convID string) bool {
	cw.askMu.Lock()
	defer cw.askMu.Unlock()
	for _, q := range cw.channelAsks {
		if q.convID == convID {
			return true
		}
	}
	return false
}

func (cw *ChatWindow) channelSendText(ch *config.Channel, msgCtx *channelMsgContext, text string) error {
	tool := &config.ChannelTool{Type: ch.Type + "_send_message"}
	handler := cw.channelToolHandler(ch, tool, msgCtx)
	_, err := handler(context.Background(), fmt.Sprintf(`{"text":%s}`, mustJSON(text)))
	return err
}

func channelQuestionText(items []*askQuestionItem) string {
	var b strings.Builder
	b.WriteString("需要你确认：")
	for i, it := range items {
		if len(items) == 1 {
			b.WriteString("\n" + it.question)
		} else {
			fmt.Fprintf(&b, "\n%d. %s", i+1, it.question)
		}
		if len(it.options) > 0 {
			b.WriteString("\n选项：" + strings.Join(it.options, " / "))
		}
	}
	b.WriteString("\n\n请直接回复答案。")
	return b.String()
}
