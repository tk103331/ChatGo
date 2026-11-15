package ui

import (
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/netx"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	mgo "github.com/egoist/mygo/ui"
)

// proxyForm is the draft of the network proxy card in the 系统 tab. The
// address is kept as a draft because it needs validation before it can be
// saved, unlike the switches around it.
type proxyForm struct {
	// url is the draft of the proxy address.
	url string
	// err is the last validation failure of the draft.
	err string
	// ready marks that the draft was seeded from the config.
	ready bool
}

// proxyCard configures the network proxy: the switch turns it on, the field
// names the server, and saving applies it to every outbound request the
// app makes without a restart.
func (cw *ChatWindow) proxyCard(c *mgo.Context) {
	k := mcore.Tokens(c)
	small := mcore.FontSize(c, mtheme.CaptionSize)

	if !cw.settings.proxy.ready {
		cw.settings.proxy.ready = true
		cw.settings.proxy.url = cw.cfg.Proxy.URL
	}

	mgo.Text(c, "网络代理").FontWeight(600)
	enabled := cw.cfg.Proxy.Enabled
	if kit.SwitchRow(c, &enabled, "使用网络代理", kit.SwitchRowOptions{
		Hint: "模型请求、模型列表、插件市场下载、渠道消息、远程 MCP 服务与版本检查走此代理；本地地址（127.0.0.1、localhost 等）始终直连。关闭时沿用系统环境变量 HTTP_PROXY / HTTPS_PROXY。",
	}).Changed() {
		cw.cfg.Proxy.Enabled = enabled
		_ = config.SaveConfig(cw.cfg)
		if enabled && cw.cfg.Proxy.EffectiveProxyURL() == "" {
			cw.toast("代理已开启，请填写并保存代理地址", mcore.SeverityWarning)
		} else {
			cw.toast("网络代理已保存", mcore.SeveritySuccess)
		}
	}

	parts := kit.InputGroup(c, &cw.settings.proxy.url, kit.InputGroupOptions{
		Placeholder: "http://127.0.0.1:7890 · https:// · socks5://",
		Label:       "代理地址",
		Error:       cw.settings.proxy.err,
		Buttons: []kit.InputGroupButton{
			{Label: "保存"},
		},
	})
	if parts.Clicked == 0 {
		if err := netx.ValidateProxyURL(cw.settings.proxy.url); err != nil {
			cw.settings.proxy.err = err.Error()
			cw.toastErrorDetail("代理地址无效", err.Error())
		} else {
			cw.settings.proxy.err = ""
			cw.settings.proxy.url = strings.TrimSpace(cw.settings.proxy.url)
			cw.cfg.Proxy.URL = cw.settings.proxy.url
			_ = config.SaveConfig(cw.cfg)
			if cw.cfg.Proxy.Enabled {
				cw.toast("网络代理已保存并生效", mcore.SeveritySuccess)
			} else {
				cw.toast("代理地址已保存，开启开关后生效", mcore.SeveritySuccess)
			}
		}
	}
	if parts.Changed() && cw.settings.proxy.err != "" {
		// A new edit clears the stale failure so the field does not stay
		// marked invalid while the user fixes it.
		cw.settings.proxy.err = ""
	}
	// The framework draws an input's Error only inside a Field, so the
	// card spells the failure out itself.
	if cw.settings.proxy.err != "" {
		mgo.Text(c, cw.settings.proxy.err).TextColor(k.Danger).FontSize(small)
	} else {
		mgo.Text(c, "支持 http、https、socks5、socks5h；保存后立即生效，无需重启。").
			FontSize(small).TextColor(k.TextMuted)
	}
}
