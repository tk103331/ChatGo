// A2UI surfaces from remote agents render in the reply. The protocol package
// keeps the surfaces; this file feeds envelopes from AG-UI and A2A into the
// hybrid router and returns user actions on the same transport.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/a2a"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/a2ui"
	"chatgo/pkg/a2ui/hybrid"
	"chatgo/pkg/a2ui/native"
	"chatgo/pkg/a2ui/web"
	"chatgo/pkg/models"
)

const a2uiSurfaceSep = "\x00"

// initA2UI builds the renderers once the window exists. The web renderer
// needs the window; the scheme serving it was registered before Show.
func (cw *ChatWindow) initA2UI(w *mygo.Window) {
	theme := native.DefaultTheme()
	if cw.themeMode == mcore.Dark {
		theme = native.DarkTheme()
	}
	nat := native.New(native.Options{
		Theme:             theme,
		Invalidate:        cw.invalidate,
		OnAction:          cw.onA2UIAction,
		OnRendererMessage: cw.onA2UIRendererMessage,
	})
	webR := web.New(w, cw.a2uiMgr, web.Options{
		ScrollToContainer: true,
		Invalidate:        cw.invalidate,
		OnAction:          cw.onA2UIWebAction,
		OnRendererMessage: cw.onA2UIRendererMessage,
	})
	cw.a2uiMu.Lock()
	cw.a2ui = hybrid.New(nat, webR)
	cw.a2uiNative = nat
	cw.a2uiWeb = webR
	cw.a2uiMu.Unlock()
	cw.refreshA2UITheme()
}

// refreshA2UITheme changes both renderers' palettes when the window's
// appearance changes. The web page follows the same light/dark choice.
func (cw *ChatWindow) refreshA2UITheme() {
	cw.a2uiMu.Lock()
	nat, wr := cw.a2uiNative, cw.a2uiWeb
	cw.a2uiMu.Unlock()
	if nat == nil && wr == nil {
		return
	}
	theme := native.DefaultTheme()
	webTheme := ""
	switch cw.themeMode {
	case mcore.Dark:
		theme = native.DarkTheme()
		webTheme = "dark"
	case mcore.Light:
		webTheme = "light"
	default:
		if systemIsDark() {
			theme = native.DarkTheme()
		}
	}
	if nat != nil {
		nat.SetTheme(theme)
	}
	if wr != nil {
		wr.SetTheme(webTheme)
	}
}

// systemIsDark reads the desktop's appearance, guarding the pre-run call
// that MyGo rejects.
func systemIsDark() (dark bool) {
	defer func() {
		if recover() != nil {
			dark = false
		}
	}()
	return mygo.Theme.IsDark()
}

// a2uiMessage renders the surfaces a reply currently owns, applying any
// envelopes appended since the last frame.
func (cw *ChatWindow) a2uiMessage(c *mgo.Context, msg models.Message) {
	if len(msg.A2UI) == 0 {
		return
	}
	cw.a2uiMu.Lock()
	router := cw.a2ui
	cw.a2uiMu.Unlock()
	if router == nil {
		return
	}
	cw.feedA2UI(msg)
	seen := map[string]bool{}
	mgo.Column(c).FillWidth().Gap(8).Children(func() {
		for _, raw := range msg.A2UI {
			for _, env := range decodeA2UI(raw) {
				id := a2uiSurfaceID(env)
				if id == "" {
					continue
				}
				local := msg.ID + a2uiSurfaceSep + id
				if seen[local] || router.RouteOf(local) == "" {
					continue
				}
				seen[local] = true
				router.View(c, local)
			}
		}
	})
}

// feedA2UI applies envelopes the message has not applied yet.
func (cw *ChatWindow) feedA2UI(msg models.Message) {
	cw.a2uiMu.Lock()
	router := cw.a2ui
	if cw.a2uiFed == nil {
		cw.a2uiFed = map[string]int{}
	}
	if cw.a2uiOwned == nil {
		cw.a2uiOwned = map[string][]string{}
	}
	if router == nil || cw.a2uiFed[msg.ID] >= len(msg.A2UI) {
		cw.a2uiMu.Unlock()
		return
	}
	start := cw.a2uiFed[msg.ID]
	cw.a2uiMu.Unlock()

	for _, raw := range msg.A2UI[start:] {
		for _, env := range decodeA2UI(raw) {
			env = prefixSurface(env, msg.ID)
			if env.CreateSurface != nil {
				cw.a2uiMu.Lock()
				cw.a2uiOwned[msg.ID] = append(cw.a2uiOwned[msg.ID], env.CreateSurface.SurfaceID)
				cw.a2uiMu.Unlock()
			}
			if errs := router.Apply(env); len(errs) > 0 {
				fmt.Printf("[A2UI] %v\n", errs)
			}
		}
	}
	cw.a2uiMu.Lock()
	cw.a2uiFed[msg.ID] = len(msg.A2UI)
	cw.a2uiMu.Unlock()
}

// decodeA2UI turns one CUSTOM/DataPart value into envelopes. It tolerates an
// array, which the A2A binding uses, as well as a single envelope.
func decodeA2UI(raw json.RawMessage) []a2ui.Message {
	raw = bytesTrim(raw)
	if len(raw) == 0 {
		return nil
	}
	var list []json.RawMessage
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil
		}
	} else {
		list = []json.RawMessage{raw}
	}
	out := make([]a2ui.Message, 0, len(list))
	for _, item := range list {
		var msg a2ui.Message
		if err := json.Unmarshal(item, &msg); err == nil {
			out = append(out, msg)
		}
	}
	return out
}

// prefixSurface scopes a surface ID to the message, so two replies may use
// the same agent-chosen ID.
func prefixSurface(msg a2ui.Message, messageID string) a2ui.Message {
	prefix := func(id string) string {
		if id == "" {
			return ""
		}
		return messageID + a2uiSurfaceSep + id
	}
	if msg.CreateSurface != nil {
		cp := *msg.CreateSurface
		cp.SurfaceID = prefix(cp.SurfaceID)
		msg.CreateSurface = &cp
	}
	if msg.UpdateComponents != nil {
		cp := *msg.UpdateComponents
		cp.SurfaceID = prefix(cp.SurfaceID)
		msg.UpdateComponents = &cp
	}
	if msg.UpdateDataModel != nil {
		cp := *msg.UpdateDataModel
		cp.SurfaceID = prefix(cp.SurfaceID)
		msg.UpdateDataModel = &cp
	}
	if msg.DeleteSurface != nil {
		cp := *msg.DeleteSurface
		cp.SurfaceID = prefix(cp.SurfaceID)
		msg.DeleteSurface = &cp
	}
	if msg.CallRendererFunction != nil {
		cp := *msg.CallRendererFunction
		cp.SurfaceID = prefix(cp.SurfaceID)
		msg.CallRendererFunction = &cp
	}
	return msg
}

// a2uiSurfaceID returns the surface a message addresses.
func a2uiSurfaceID(msg a2ui.Message) string {
	switch {
	case msg.CreateSurface != nil:
		return msg.CreateSurface.SurfaceID
	case msg.UpdateComponents != nil:
		return msg.UpdateComponents.SurfaceID
	case msg.UpdateDataModel != nil:
		return msg.UpdateDataModel.SurfaceID
	case msg.DeleteSurface != nil:
		return msg.DeleteSurface.SurfaceID
	case msg.CallRendererFunction != nil:
		return msg.CallRendererFunction.SurfaceID
	}
	return ""
}

// onA2UIAction receives a click from the native renderer and defers it to
// the UI thread, where a turn may be started.
func (cw *ChatWindow) onA2UIAction(a a2ui.Action) {
	cw.postMain(func() { cw.dispatchA2UIAction(a) })
}

// onA2UIWebAction keeps the page's data model with the action, then sends it
// back like a native action.
func (cw *ChatWindow) onA2UIWebAction(a a2ui.Action, data map[string]any) {
	cw.postMain(func() {
		if data != nil {
			cw.a2uiMu.Lock()
			if cw.a2uiData == nil {
				cw.a2uiData = map[string]map[string]any{}
			}
			cw.a2uiData[a.SurfaceID] = data
			cw.a2uiMu.Unlock()
		}
		cw.dispatchA2UIAction(a)
	})
}

// dispatchA2UIAction sends an action back over the conversation's transport.
func (cw *ChatWindow) dispatchA2UIAction(a a2ui.Action) {
	if cw.cur == nil {
		return
	}
	if cw.streamingCur() {
		cw.toast("正在回复，稍后再操作界面", mcore.SeverityInfo)
		return
	}
	local := a.SurfaceID
	msgID, original := splitA2UISurface(local)
	if msgID == "" || original == "" {
		return
	}
	a.SurfaceID = original
	var data map[string]any
	cw.a2uiMu.Lock()
	if saved := cw.a2uiData[local]; saved != nil {
		data = saved
	} else if cw.a2ui != nil {
		if raw, ok := cw.a2ui.DataModel(local); ok {
			data, _ = raw.(map[string]any)
		}
	}
	cw.a2uiMu.Unlock()
	conv := cw.cur
	text := strings.TrimSpace(a.UserMessage)
	if text == "" {
		text = "界面操作：" + a.Name
	}
	cw.sendA2UIEnvelope(conv, a.Envelope(), original, data, text)
}

// sendA2UIEnvelope puts a renderer-to-agent envelope on the next turn of the
// conversation's transport.
func (cw *ChatWindow) sendA2UIEnvelope(conv *models.Conversation, envelope map[string]any, original string, data map[string]any, text string) {
	capabilities := map[string]any{"1.0": map[string]any{
		"supportedCatalogIds": []string{a2ui.BasicCatalogID},
	}}
	cw.a2uiMu.Lock()
	if cw.a2uiFwd == nil {
		cw.a2uiFwd = map[string]map[string]any{}
	}
	if cw.a2uiParts == nil {
		cw.a2uiParts = map[string][]any{}
	}
	switch {
	case strings.HasPrefix(conv.Agent, "agui:"):
		payload := map[string]any{
			"clientMessage":      envelope,
			"clientCapabilities": capabilities,
		}
		if data != nil && original != "" {
			payload["clientDataModel"] = map[string]any{
				"version": "1.0", "surfaces": map[string]any{original: data},
			}
		}
		cw.a2uiFwd[conv.ID] = map[string]any{"a2ui": payload}
	case strings.HasPrefix(conv.Agent, "a2a:"):
		cw.a2uiParts[conv.ID] = []any{envelope}
	}
	cw.a2uiMu.Unlock()
	cw.draft = text
	cw.sendDraft()
}

// onA2UIRendererMessage forwards a renderer-initiated message back to the
// agent. A function response belongs to the run that asked for it; ChatGo's
// transports cannot inject into a live run, so it starts the next turn.
func (cw *ChatWindow) onA2UIRendererMessage(m a2ui.RendererMessage) {
	cw.postMain(func() {
		if cw.cur == nil {
			return
		}
		if cw.streamingCur() {
			// The transport cannot inject into a live run. Keep the response
			// and send it as the next turn when this one ends.
			cw.a2uiMu.Lock()
			cw.a2uiQueue = append(cw.a2uiQueue, m)
			cw.a2uiMu.Unlock()
			return
		}
		cw.sendRendererMessage(m)
	})
}

// sendRendererMessage sends one renderer-to-agent message as a turn.
func (cw *ChatWindow) sendRendererMessage(m a2ui.RendererMessage) {
	if cw.cur == nil {
		return
	}
	if m.CallAgentFunction != nil && m.CallAgentFunction.SurfaceID != "" {
		_, original := splitA2UISurface(m.CallAgentFunction.SurfaceID)
		m.CallAgentFunction.SurfaceID = original
	}
	raw, err := a2ui.MarshalRendererMessage(m)
	if err != nil {
		return
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}
	text := "界面函数回调"
	if m.FunctionResponse != nil {
		text = "界面函数结果"
	}
	cw.sendA2UIEnvelope(cw.cur, envelope, "", nil, text)
}

// flushA2UIRenderer pops one queued renderer message once the previous turn
// has finished.
func (cw *ChatWindow) flushA2UIRenderer() {
	if cw.streamingCur() {
		return
	}
	cw.a2uiMu.Lock()
	if len(cw.a2uiQueue) == 0 {
		cw.a2uiMu.Unlock()
		return
	}
	m := cw.a2uiQueue[0]
	cw.a2uiQueue = cw.a2uiQueue[1:]
	cw.a2uiMu.Unlock()
	cw.sendRendererMessage(m)
}

// scheduleA2UIFlush lets the finished turn settle before sending a queued
// function response, so the new turn starts after the old one's bookkeeping.
func (cw *ChatWindow) scheduleA2UIFlush() {
	cw.a2uiMu.Lock()
	has := len(cw.a2uiQueue) > 0
	cw.a2uiMu.Unlock()
	if !has {
		return
	}
	time.AfterFunc(200*time.Millisecond, func() {
		cw.postMain(func() {
			cw.flushA2UIRenderer()
			cw.scheduleA2UIFlush()
		})
	})
}

// takeA2UIForward consumes the AG-UI payload an action queued for the turn.
func (cw *ChatWindow) takeA2UIForward(convID string) map[string]any {
	cw.a2uiMu.Lock()
	defer cw.a2uiMu.Unlock()
	out := cw.a2uiFwd[convID]
	delete(cw.a2uiFwd, convID)
	return out
}

// takeA2UIParts consumes the A2A DataPart values an action queued for the turn.
func (cw *ChatWindow) takeA2UIParts(convID string) []any {
	cw.a2uiMu.Lock()
	defer cw.a2uiMu.Unlock()
	out := cw.a2uiParts[convID]
	delete(cw.a2uiParts, convID)
	return out
}

// dropA2UIMessages releases the surfaces of messages an edit or regeneration
// removed, so their web pages do not stay alive in the router.
func (cw *ChatWindow) dropA2UIMessages(msgs []models.Message) {
	cw.a2uiMu.Lock()
	router := cw.a2ui
	var surfaces []string
	for _, m := range msgs {
		for _, id := range cw.a2uiOwned[m.ID] {
			surfaces = append(surfaces, id)
			delete(cw.a2uiData, id)
		}
		delete(cw.a2uiOwned, m.ID)
		delete(cw.a2uiFed, m.ID)
	}
	cw.a2uiMu.Unlock()
	if router == nil {
		return
	}
	for _, id := range surfaces {
		router.Apply(a2ui.Message{Version: a2ui.Version, DeleteSurface: &a2ui.DeleteSurface{SurfaceID: id}})
	}
}

// splitA2UISurface undoes the per-message prefix.
func splitA2UISurface(local string) (messageID, surfaceID string) {
	if i := strings.LastIndex(local, a2uiSurfaceSep); i >= 0 {
		return local[:i], local[i+len(a2uiSurfaceSep):]
	}
	return "", local
}

// bytesTrim is strings.TrimSpace on raw JSON bytes without an allocation for
// the common case.
func bytesTrim(raw json.RawMessage) json.RawMessage {
	for len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\n' || raw[0] == '\r' || raw[0] == '\t') {
		raw = raw[1:]
	}
	for len(raw) > 0 {
		c := raw[len(raw)-1]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		raw = raw[:len(raw)-1]
	}
	return raw
}

// a2aResultA2UI copies a finished A2A turn's surfaces onto its reply.
func a2aResultA2UI(m *models.Message, res *a2a.Result) {
	if res == nil || len(res.A2UI) == 0 {
		return
	}
	// The live callback already appended these; only fill in for a turn that
	// produced surfaces without streaming a callback.
	if len(m.A2UI) > 0 {
		return
	}
	m.A2UI = append(m.A2UI, res.A2UI...)
}

// a2uiSinkKey carries the function a built-in tool uses to attach surfaces to
// the reply being written.
type a2uiSinkKey struct{}

func withA2UISink(ctx context.Context, fn func([]json.RawMessage)) context.Context {
	return context.WithValue(ctx, a2uiSinkKey{}, fn)
}

func a2uiSinkFrom(ctx context.Context) func([]json.RawMessage) {
	fn, _ := ctx.Value(a2uiSinkKey{}).(func([]json.RawMessage))
	return fn
}

// a2uiTool is the built-in tool a local agent calls to render a surface. It
// validates the envelopes and hands them to the running reply; the model sees
// a short confirmation, not the UI JSON.
func (cw *ChatWindow) a2uiTool(ctx context.Context, raw string) (string, error) {
	sink := a2uiSinkFrom(ctx)
	if sink == nil {
		return "", fmt.Errorf("当前运行不支持渲染 A2UI 界面")
	}
	var in struct {
		SurfaceID  string            `json:"surface_id"`
		CatalogID  string            `json:"catalog_id"`
		Components []json.RawMessage `json:"components"`
		DataModel  map[string]any    `json:"data_model"`
		DataModel2 map[string]any    `json:"dataModel"`
		Messages   []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return "", fmt.Errorf("A2UI 参数解析失败：%w", err)
	}
	catalog := strings.TrimSpace(in.CatalogID)
	if catalog == "" {
		catalog = a2ui.BasicCatalogID
	}
	var envelopes []json.RawMessage
	surface := ""
	if len(in.Messages) > 0 {
		envelopes = in.Messages
		for _, item := range envelopes {
			var msg a2ui.Message
			if err := json.Unmarshal(item, &msg); err != nil {
				return "", fmt.Errorf("A2UI 消息格式错误：%w", err)
			}
			if err := msg.Validate(); err != nil {
				return "", fmt.Errorf("A2UI 消息无效：%w", err)
			}
			if msg.CreateSurface != nil {
				surface = msg.CreateSurface.SurfaceID
			}
		}
	} else {
		if strings.TrimSpace(in.SurfaceID) == "" {
			return "", fmt.Errorf("缺少 surface_id")
		}
		if len(in.Components) == 0 {
			return "", fmt.Errorf("缺少 components")
		}
		hasRoot := false
		for _, c := range in.Components {
			var head struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(c, &head); err != nil {
				return "", fmt.Errorf("组件格式错误：%w", err)
			}
			if head.ID == "root" {
				hasRoot = true
			}
		}
		if !hasRoot {
			return "", fmt.Errorf("components 必须包含 id 为 root 的根组件")
		}
		data := in.DataModel
		if data == nil {
			data = in.DataModel2
		}
		create := map[string]any{
			"surfaceId":  in.SurfaceID,
			"catalogId":  catalog,
			"components": in.Components,
		}
		if data != nil {
			create["dataModel"] = data
		}
		env, err := json.Marshal(map[string]any{"version": a2ui.Version, "createSurface": create})
		if err != nil {
			return "", err
		}
		envelopes = []json.RawMessage{env}
		surface = in.SurfaceID
	}
	sink(envelopes)
	cw.invalidate()
	if surface != "" {
		return "已渲染 A2UI 界面：" + surface, nil
	}
	return fmt.Sprintf("已提交 %d 条 A2UI 消息", len(envelopes)), nil
}
