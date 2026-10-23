package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// webviewActionScripts map a webview tool action to the JavaScript it runs in
// the preview. Each returns the text the model sees.
var webviewActionScripts = map[string]string{
	"text":  `return document.body ? document.body.innerText.slice(0, 20000) : ""`,
	"html":  `return document.documentElement ? document.documentElement.outerHTML.slice(0, 20000) : ""`,
	"title": `return document.title`,
	"url":   `return location.href`,
	"elements": `
const sel = __SEL__ || 'a,button,input,select,textarea,[role],[onclick]';
const out = [];
document.querySelectorAll(sel).forEach((el, i) => {
  if (i >= 200) return;
  const r = el.getBoundingClientRect();
  out.push({
    index: i,
    tag: el.tagName.toLowerCase(),
    id: el.id || '',
    cls: typeof el.className === 'string' ? el.className : '',
    text: (el.innerText || el.value || el.getAttribute('aria-label') || el.getAttribute('placeholder') || '').trim().slice(0, 120),
    visible: r.width > 0 && r.height > 0,
  });
});
return out;`,
	"click": `
const sel = __SEL__;
if (!sel) throw new Error('点击需要提供 selector');
const el = document.querySelector(sel);
if (!el) throw new Error('未找到元素: ' + sel);
el.scrollIntoView({block: 'center'});
el.click();
return '已点击 ' + sel;`,
	"input": `
const sel = __SEL__;
if (!sel) throw new Error('输入需要提供 selector');
const el = document.querySelector(sel);
if (!el) throw new Error('未找到元素: ' + sel);
el.focus();
if (el.isContentEditable) {
  el.textContent = __TEXT__;
} else {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
  setter.call(el, __TEXT__);
}
el.dispatchEvent(new Event('input', {bubbles: true}));
el.dispatchEvent(new Event('change', {bubbles: true}));
return '已输入 ' + __TEXT__ + ' 到 ' + sel;`,
	"scroll": `
const y = __NUM__ || 0;
window.scrollTo(0, y);
return '已滚动到 y=' + window.scrollY;`,
	"eval":    `return (0, eval)(__TEXT__);`,
	"back":    `history.back(); return '已后退';`,
	"forward": `history.forward(); return '已前进';`,
	"reload":  `location.reload(); return '已刷新';`,
}

// webviewParams is the schema the webview tool exposes to the model.
func webviewParams() map[string]*schema.ParameterInfo {
	return map[string]*schema.ParameterInfo{
		"action": {
			Type:     schema.String,
			Desc:     "操作：open（打开 URL 或本地 HTML）、close（关闭标签）、list（列出标签）、text（取正文）、html（取 HTML）、title（取标题）、url（取当前地址）、elements（列元素）、click（点元素）、input（填内容）、scroll（滚动）、eval（执行 JS）、back/forward/reload",
			Required: true,
		},
		"url":      {Type: schema.String, Desc: "action=open 时要打开的地址或相对工作目录的 HTML 路径"},
		"selector": {Type: schema.String, Desc: "CSS 选择器，click/input/elements 用"},
		"text":     {Type: schema.String, Desc: "input 要填入的文本；eval 要执行的 JavaScript"},
		"scroll_y": {Type: schema.Integer, Desc: "scroll 的目标纵向位置，默认 0"},
		"webview":  {Type: schema.String, Desc: "目标 webview 标识（如 wv1），留空用当前显示的；open 时留空则新建，close 时留空则关闭当前"},
	}
}

// webviewTool runs one webview action against the preview's web view.
func (cw *ChatWindow) webviewTool(ctx context.Context, raw string) (string, error) {
	var in struct {
		Action   string `json:"action"`
		URL      string `json:"url"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
		ScrollY  int    `json:"scroll_y"`
		Target   string `json:"webview"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		return "", fmt.Errorf("缺少必填参数 action")
	}
	switch action {
	case "open":
		if strings.TrimSpace(in.URL) == "" {
			return "", fmt.Errorf("open 需要 url")
		}
		return cw.previewURL(ctx, in.Target, in.URL)
	case "back":
		_, err := cw.evalInWebTab(ctx, in.Target, "history.back()")
		return "已后退", err
	case "forward":
		_, err := cw.evalInWebTab(ctx, in.Target, "history.forward()")
		return "已前进", err
	case "reload":
		_, err := cw.evalInWebTab(ctx, in.Target, "location.reload()")
		return "已刷新", err
	case "close":
		return cw.closeWebTab(in.Target)
	case "list":
		return cw.listWebTabs()
	}
	script, ok := webviewActionScripts[action]
	if !ok {
		return "", fmt.Errorf("未知 action %q", action)
	}
	script = strings.ReplaceAll(script, "__SEL__", jsString(in.Selector))
	script = strings.ReplaceAll(script, "__TEXT__", jsString(in.Text))
	script = strings.ReplaceAll(script, "__NUM__", fmt.Sprintf("%d", in.ScrollY))
	res, err := cw.evalInWebTab(ctx, in.Target, script)
	if err != nil {
		return "", err
	}
	switch v := res.(type) {
	case string:
		return v, nil
	default:
		out, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v), nil
		}
		return string(out), nil
	}
}

// jsString renders s as a JavaScript string literal.
func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
