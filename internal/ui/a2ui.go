// A2UI surfaces from remote agents render in the reply. The protocol package
// keeps the surfaces; this file feeds envelopes from AG-UI and A2A into the
// hybrid router and returns user actions on the same transport.
package ui

import (
	"encoding/json"
	"fmt"
	"strings"

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
		Theme:      theme,
		Invalidate: cw.invalidate,
		OnAction:   cw.onA2UIAction,
	})
	webR := web.New(w, cw.a2uiMgr, web.Options{
		ScrollToContainer: true,
		Invalidate:        cw.invalidate,
		OnAction:          cw.onA2UIWebAction,
	})
	cw.a2uiMu.Lock()
	cw.a2ui = hybrid.New(nat, webR)
	cw.a2uiMu.Unlock()
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
	envelope := a.Envelope()
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
		if data != nil {
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
