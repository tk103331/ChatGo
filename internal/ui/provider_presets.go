package ui

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// providerPreset is one model provider the app knows how to configure, the
// way magpie's presets do: a name, the protocol it speaks and the address
// its API lives at, so adding one only asks for the key.
type providerPreset struct {
	Name    string
	Type    string // a config.ProviderType, or "custom" with APIType below
	APIType string // the upstream protocol of a custom preset, empty otherwise
	BaseURL string
	Note    string
	KeysURL string
	// Kind groups the catalog: the model's own maker, an aggregator
	// reselling many, or something on this machine.
	Kind string
}

// The catalog's groups.
const (
	presetVendor = "厂商直连"
	presetRelay  = "聚合中转"
	presetLocal  = "本地模型"
)

// providerPresets lists the mainstream providers, checked against magpie's
// preset table (internal/provider/presets.go): every BaseURL is the one the
// vendor's own API answers at, so a fresh provider only needs its key.
// Custom presets speak OpenAI's protocol against a vendor that offers an
// OpenAI-compatible endpoint.
func providerPresets() []providerPreset {
	return []providerPreset{
		// —— 厂商直连 ——
		{Name: "OpenAI", Type: config.ProviderTypeOpenAI, BaseURL: "https://api.openai.com/v1",
			KeysURL: "https://platform.openai.com/api-keys", Kind: presetVendor},
		{Name: "Anthropic", Type: config.ProviderTypeAnthropic, BaseURL: "https://api.anthropic.com",
			KeysURL: "https://console.anthropic.com/settings/keys", Kind: presetVendor},
		{Name: "Gemini", Type: config.ProviderTypeGemini, BaseURL: "https://generativelanguage.googleapis.com",
			KeysURL: "https://aistudio.google.com/apikey", Kind: presetVendor},
		{Name: "DeepSeek", Type: config.ProviderTypeDeepSeek, BaseURL: "https://api.deepseek.com/v1",
			KeysURL: "https://platform.deepseek.com/api_keys", Kind: presetVendor},
		{Name: "Qwen", Type: config.ProviderTypeQwen, BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Note: "阿里云百炼 · 国内站", KeysURL: "https://bailian.console.aliyun.com/?tab=model#/api-key", Kind: presetVendor},
		{Name: "Qwen International", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
			Note: "DashScope 国际站", KeysURL: "https://modelstudio.console.alibabacloud.com/?tab=playground#/api-key", Kind: presetVendor},
		{Name: "Kimi", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.moonshot.cn/v1",
			Note: "月之暗面 Moonshot", KeysURL: "https://platform.moonshot.cn/console/api-keys", Kind: presetVendor},
		{Name: "Kimi (Global)", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.moonshot.ai/v1",
			Note: "Kimi 国际站", KeysURL: "https://platform.kimi.ai/console/api-keys", Kind: presetVendor},
		{Name: "智谱 GLM", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://open.bigmodel.cn/api/paas/v4",
			Note:    "按量计费；Coding Plan 用 https://open.bigmodel.cn/api/coding/paas/v4",
			KeysURL: "https://open.bigmodel.cn/usercenter/proj-mgmt/apikeys", Kind: presetVendor},
		{Name: "Z.ai", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.z.ai/api/paas/v4",
			Note: "GLM 海外站", KeysURL: "https://z.ai/manage-apikey/apikey-list", Kind: presetVendor},
		{Name: "MiniMax", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.minimaxi.com/v1",
			Note: "MiniMax 国内站", KeysURL: "https://platform.minimaxi.com/user-center/basic-information/interface-key", Kind: presetVendor},
		{Name: "MiniMax (Global)", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.minimax.io/v1",
			Note: "MiniMax 国际站", KeysURL: "https://platform.minimax.io/user-center/basic-information/interface-key", Kind: presetVendor},
		{Name: "xAI Grok", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.x.ai/v1",
			KeysURL: "https://console.x.ai", Kind: presetVendor},
		{Name: "Mistral", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.mistral.ai/v1",
			KeysURL: "https://console.mistral.ai/api-keys", Kind: presetVendor},
		{Name: "StepFun 阶跃星辰", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.stepfun.com/v1",
			KeysURL: "https://platform.stepfun.com/interface-key", Kind: presetVendor},
		{Name: "火山方舟 Ark", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
			Note:    "按量计费；Coding Plan 用 https://ark.cn-beijing.volces.com/api/coding/v3",
			KeysURL: "https://ark.volcengine.com/region:cn-beijing/apikey", Kind: presetVendor},
		{Name: "腾讯混元", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.hunyuan.cloud.tencent.com/v1",
			KeysURL: "https://console.cloud.tencent.com/hunyuan/api-key", Kind: presetVendor},
		{Name: "LongCat 美团", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.longcat.chat/openai/v1",
			Note: "LongCat API 平台", KeysURL: "https://longcat.chat/platform/api_keys", Kind: presetVendor},

		// —— 聚合中转 ——
		{Name: "OpenRouter", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://openrouter.ai/api/v1",
			Note: "一家密钥聚合各家模型", KeysURL: "https://openrouter.ai/keys", Kind: presetRelay},
		{Name: "SiliconFlow 硅基流动", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.siliconflow.cn/v1",
			Note: "国内站；国际站为 api.siliconflow.com", KeysURL: "https://cloud.siliconflow.cn/account/ak", Kind: presetRelay},
		{Name: "Together AI", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.together.xyz/v1",
			KeysURL: "https://api.together.ai/settings/api-keys", Kind: presetRelay},
		{Name: "Fireworks", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.fireworks.ai/inference/v1",
			KeysURL: "https://app.fireworks.ai/settings/users/api-keys", Kind: presetRelay},
		{Name: "Groq", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.groq.com/openai/v1",
			Note: "高速推理", KeysURL: "https://console.groq.com/keys", Kind: presetRelay},
		{Name: "NVIDIA NIM", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://integrate.api.nvidia.com/v1",
			KeysURL: "https://build.nvidia.com/settings/api-keys", Kind: presetRelay},
		{Name: "ModelScope 魔搭", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api-inference.modelscope.cn/v1",
			Note: "魔搭 API-Inference，Access Token 作密钥", KeysURL: "https://modelscope.cn/my/myaccesstoken", Kind: presetRelay},
		{Name: "302.AI", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://api.302.ai/v1",
			KeysURL: "https://302.ai/api-keys/list", Kind: presetRelay},
		{Name: "AiHubMix", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://aihubmix.com/v1",
			KeysURL: "https://console.aihubmix.com/token", Kind: presetRelay},
		{Name: "OpenCode Zen", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "https://opencode.ai/zen/v1",
			Note: "免费模型无需密钥", KeysURL: "https://opencode.ai/auth", Kind: presetRelay},

		// —— 本地模型 ——
		{Name: "Ollama", Type: config.ProviderTypeOllama, BaseURL: "http://localhost:11434",
			Note: "本地模型服务", Kind: presetLocal},
		{Name: "LM Studio", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "http://localhost:1234/v1",
			Note: "本地服务器，默认端口 1234", Kind: presetLocal},
		{Name: "vLLM", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "http://localhost:8000/v1",
			Note: "本地推理服务器，默认端口 8000", Kind: presetLocal},
		{Name: "LM Engine", Type: "custom", APIType: config.ProviderTypeOpenAI, BaseURL: "http://localhost:8000/v1",
			Note: "MLX 本地推理，默认端口 8000", Kind: presetLocal},
	}
}

// providerPresetKinds lists the groups in catalog order.
func providerPresetKinds() []string {
	return []string{presetVendor, presetRelay, presetLocal}
}

// addProviderFromPreset appends the preset as a new provider and opens its
// form, so the next thing to do is paste the key. The provider starts
// disabled — the switch flips once the key is in.
func (cw *ChatWindow) addProviderFromPreset(e providerPreset) {
	name := e.Name
	// A taken name gets a numeric suffix rather than a clash, the way a
	// file copy does.
	for n := 2; cw.providerNameTaken(name); n++ {
		name = e.Name + " " + itoa(n)
	}
	typ := e.Type
	if typ == "" {
		typ = config.ProviderTypeCustom
	}
	p := config.Provider{
		Name: name, Type: typ, BaseURL: e.BaseURL,
		Enabled: false,
	}
	if typ == config.ProviderTypeCustom {
		p.APIType = e.APIType
		if p.APIType == "" {
			p.APIType = config.ProviderTypeOpenAI
		}
	}
	cw.cfg.Providers = append(cw.cfg.Providers, p)
	_ = config.SaveConfig(cw.cfg)
	cw.loadProviderForm(len(cw.cfg.Providers) - 1)
	cw.presetCatalogOpen = false
	cw.toast("已添加 "+name+"，请填写 API Key 后启用", mcore.SeveritySuccess)
}

// providerNameTaken reports whether a provider of that name exists.
func (cw *ChatWindow) providerNameTaken(name string) bool {
	for _, p := range cw.cfg.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

// providerPresetCatalogDialog lists the presets grouped the way magpie's
// add sheet does: the model's own maker, the aggregators, the local
// servers. One click appends the provider preconfigured with its address.
func (cw *ChatWindow) providerPresetCatalogDialog(c *mgo.Context) {
	if !cw.presetCatalogOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	kit.Dialog(c, &cw.presetCatalogOpen, kit.DialogOptions{
		Title: "从目录添加模型",
		Width: 600,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			in := kit.SearchInput(c, &cw.presetCatalogQuery, kit.SearchInputOptions{Placeholder: "搜索 Provider", Label: "搜索"})
			in.Element.FillWidth()
			q := strings.ToLower(strings.TrimSpace(cw.presetCatalogQuery))
			shown := 0
			for _, kind := range providerPresetKinds() {
				var items []providerPreset
				for _, e := range providerPresets() {
					if e.Kind != kind {
						continue
					}
					if q != "" && !strings.Contains(strings.ToLower(e.Name), q) &&
						!strings.Contains(strings.ToLower(e.Note), q) {
						continue
					}
					items = append(items, e)
				}
				if len(items) == 0 {
					continue
				}
				shown += len(items)
				mgo.Text(c, kind).FontSize(caption).FontWeight(600).TextColor(k.TextMuted)
				mgo.Scroll(c).MaxHeight(320).Children(func() {
					mgo.Column(c).Gap(u).Children(func() {
						for _, e := range items {
							e := e
							taken := cw.providerNameTaken(e.Name)
							mgo.Row(c).Key(e.Name).FillWidth().Gap(u*2).AlignItems(mgo.Center).
								Padding(u, u*2).Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border).
								Children(func() {
									mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
										mgo.Text(c, e.Name).FontWeight(600).SingleLine()
										if e.BaseURL != "" {
											mgo.Text(c, e.BaseURL).Font(mtheme.MonoFont).FontSize(caption).
												TextColor(k.TextMuted).SingleLine().Ellipsis("…").Tooltip(e.BaseURL)
										}
										if e.Note != "" {
											mgo.Text(c, e.Note).FontSize(caption).TextColor(k.TextMuted).MaxLines(1)
										}
									})
									if taken {
										mgo.Text(c, "已添加").FontSize(caption).TextColor(k.Success).Shrink(0)
									} else if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
										cw.addProviderFromPreset(e)
									}
								})
						}
					})
				})
			}
			if shown == 0 {
				mgo.Text(c, "没有匹配的 Provider").TextColor(k.TextMuted)
			}
		})
	})
}
