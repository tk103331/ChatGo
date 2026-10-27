package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

// QRLogin is what a platform's quick setup returns: the credentials a user
// would otherwise copy out of a console by hand.
type QRLogin struct {
	// URL is the page the user opens, rendered as a QR code or a link.
	URL string
	// Tip is the instruction shown beside the code.
	Tip string
	// Notice is an optional warning the user must read before scanning, used
	// where the platform's page is branded for another product.
	Notice string
	// ExpiresIn is how long the code stays valid, zero when unknown.
	ExpiresIn time.Duration
	// Wait blocks until the user finishes, returning the credentials. It must
	// honour ctx and return ErrQRLoginExpired when the code goes stale.
	Wait func(ctx context.Context) (QuickCredentials, error)
}

// QuickCredentials is a quick setup's result.
type QuickCredentials struct {
	AppID     string
	AppSecret string
	// Options carries any extra field the platform returned.
	Options map[string]string
}

// ErrQRLoginExpired reports that the code was not scanned in time.
var ErrQRLoginExpired = fmt.Errorf("channel: 二维码已过期")

// QRLoginer is implemented by platforms that can create or bind a bot from a
// scanned code, which is the only setup a desktop app can offer without asking
// the user to navigate a developer console.
type QRLoginer interface {
	// BeginQRLogin starts the flow and returns the code to show.
	BeginQRLogin(ctx context.Context) (*QRLogin, error)
}

// wecomQRGenerateURL and wecomQRQueryURL are the intelligent robot's own
// quick-creation endpoints. They are not part of the published API reference
// but are what the platform's CLI and several clients use.
var (
	wecomQRGenerateURL = "https://work.weixin.qq.com/ai/qc/generate"
	wecomQRQueryURL    = "https://work.weixin.qq.com/ai/qc/query_result"
)

const (
	// wecomQRPageURL renders the code as a page, which is what a desktop user
	// opens when the image cannot be shown.
	wecomQRPageURL = "https://work.weixin.qq.com/ai/qc/gen"
	// wecomQRSource identifies this client to the platform. The parameter is
	// a free-form tag, not a registered identifier.
	wecomQRSource = "chatgo"
	// wecomQRPollInterval is how often the result is checked; the platform
	// asks for no faster than every three seconds.
	wecomQRPollInterval = 3 * time.Second
	// wecomQRTimeout bounds the whole flow.
	wecomQRTimeout = 5 * time.Minute
)

// wecomPlatformCode is the platform tag the generate endpoint expects.
func wecomPlatformCode() int {
	switch {
	case isDarwin():
		return 1
	case isWindows():
		return 2
	default:
		return 3
	}
}

// BeginQRLogin implements QRLoginer for WeCom: the user scans with the WeCom
// app, taps "create a robot", and the platform hands back its BotID and secret.
func (w *WeCom) BeginQRLogin(ctx context.Context) (*QRLogin, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	generate := fmt.Sprintf("%s?source=%s&plat=%d", wecomQRGenerateURL, wecomQRSource, wecomPlatformCode())
	var gen struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Data    struct {
			SCode   string `json:"scode"`
			AuthURL string `json:"auth_url"`
		} `json:"data"`
	}
	if err := getJSON(ctx, client, generate, &gen); err != nil {
		return nil, fmt.Errorf("channel: 获取企业微信二维码失败: %w", err)
	}
	if gen.ErrCode != 0 {
		return nil, fmt.Errorf("channel: 获取企业微信二维码失败: %d %s", gen.ErrCode, gen.ErrMsg)
	}
	if gen.Data.SCode == "" || gen.Data.AuthURL == "" {
		return nil, fmt.Errorf("channel: 企业微信二维码响应缺少字段")
	}
	scode := gen.Data.SCode

	return &QRLogin{
		URL:       gen.Data.AuthURL,
		Tip:       "请用企业微信 App 扫码，在手机上点「一键创建智能机器人」（5 分钟内有效）",
		ExpiresIn: wecomQRTimeout,
		Wait: func(ctx context.Context) (QuickCredentials, error) {
			deadline := time.Now().Add(wecomQRTimeout)
			for {
				if time.Now().After(deadline) {
					return QuickCredentials{}, ErrQRLoginExpired
				}
				select {
				case <-ctx.Done():
					return QuickCredentials{}, ctx.Err()
				case <-time.After(wecomQRPollInterval):
				}
				var res struct {
					ErrCode int    `json:"errcode"`
					ErrMsg  string `json:"errmsg"`
					Data    struct {
						Status  string `json:"status"`
						BotInfo struct {
							BotID  string `json:"botid"`
							Secret string `json:"secret"`
						} `json:"bot_info"`
					} `json:"data"`
				}
				query := wecomQRQueryURL + "?scode=" + url.QueryEscape(scode)
				if err := getJSON(ctx, client, query, &res); err != nil {
					// A transient failure must not abort a flow the user is
					// halfway through; the next tick tries again.
					continue
				}
				if res.ErrCode != 0 {
					continue
				}
				if res.Data.Status != "success" {
					continue
				}
				if res.Data.BotInfo.BotID == "" || res.Data.BotInfo.Secret == "" {
					return QuickCredentials{}, fmt.Errorf("channel: 扫码成功但没有返回 BotID 或 Secret")
				}
				return QuickCredentials{
					AppID:     res.Data.BotInfo.BotID,
					AppSecret: res.Data.BotInfo.Secret,
				}, nil
			}
		},
	}, nil
}

// dingtalkRegistrationBase is the device-flow registration endpoint. The flow
// is what the platform's own onboarding clients use: init hands out a nonce,
// begin turns it into a device code and a verification page, and poll reports
// the credentials once the user authorizes.
var dingtalkRegistrationBase = "https://oapi.dingtalk.com/app/registration"

// dingtalkRegistrationSource identifies this client. The platform accepts any
// value here, but the page it sends the user to is branded for OpenClaw
// regardless, which is why the flow carries a notice.
const dingtalkRegistrationSource = "openClaw"

// dingtalkPollInterval is how often the authorization is checked.
const dingtalkPollInterval = 3 * time.Second

// dingtalkTimeout bounds the whole flow.
const dingtalkTimeout = 5 * time.Minute

// BeginQRLogin implements QRLoginer for DingTalk.
//
// The page the platform opens is branded "OpenClaw", not ChatGo: the
// verification address is fixed server-side and cannot be changed from here.
// The caller must show Notice so the user knows why another product's name
// appears before they authorize.
func (d *DingTalk) BeginQRLogin(ctx context.Context) (*QRLogin, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	var init struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Nonce   string `json:"nonce"`
	}
	if err := postJSON(ctx, client, dingtalkRegistrationBase+"/init",
		map[string]string{"source": dingtalkRegistrationSource}, &init); err != nil {
		return nil, fmt.Errorf("channel: 钉钉注册初始化失败: %w", err)
	}
	if init.ErrCode != 0 {
		return nil, fmt.Errorf("channel: 钉钉注册初始化失败: %d %s", init.ErrCode, init.ErrMsg)
	}
	if init.Nonce == "" {
		return nil, fmt.Errorf("channel: 钉钉注册初始化缺少 nonce")
	}

	var begin struct {
		ErrCode                 int    `json:"errcode"`
		ErrMsg                  string `json:"errmsg"`
		DeviceCode              string `json:"device_code"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := postJSON(ctx, client, dingtalkRegistrationBase+"/begin",
		map[string]string{"nonce": init.Nonce}, &begin); err != nil {
		return nil, fmt.Errorf("channel: 钉钉注册请求失败: %w", err)
	}
	if begin.ErrCode != 0 {
		return nil, fmt.Errorf("channel: 钉钉注册请求失败: %d %s", begin.ErrCode, begin.ErrMsg)
	}
	if begin.DeviceCode == "" || begin.VerificationURIComplete == "" {
		return nil, fmt.Errorf("channel: 钉钉注册响应缺少设备码或验证地址")
	}

	interval := time.Duration(begin.Interval) * time.Second
	if interval < 2*time.Second {
		interval = 3 * time.Second
	}
	expires := dingtalkTimeout
	if begin.ExpiresIn > 0 {
		expires = time.Duration(begin.ExpiresIn) * time.Second
	}
	deviceCode := begin.DeviceCode

	return &QRLogin{
		URL: begin.VerificationURIComplete,
		Tip: "请用钉钉 App 扫码并在手机上确认授权（5 分钟内有效）",
		// The scan page is DingTalk's own onboarding bridge and carries the
		// OpenClaw brand; saying so up front keeps the user from thinking they
		// landed somewhere wrong.
		Notice:    "注意：钉钉的授权页面由平台固定生成，页面上会显示「OpenClaw」字样，这是钉钉的接入通道，不是 ChatGo 之外的另一个应用。确认页面地址以 open-dev.dingtalk.com 开头后再扫码授权。",
		ExpiresIn: expires,
		Wait: func(ctx context.Context) (QuickCredentials, error) {
			deadline := time.Now().Add(expires)
			for {
				if time.Now().After(deadline) {
					return QuickCredentials{}, ErrQRLoginExpired
				}
				select {
				case <-ctx.Done():
					return QuickCredentials{}, ctx.Err()
				case <-time.After(interval):
				}
				var poll struct {
					ErrCode      int    `json:"errcode"`
					ErrMsg       string `json:"errmsg"`
					Status       string `json:"status"`
					ClientID     string `json:"client_id"`
					ClientSecret string `json:"client_secret"`
					FailReason   string `json:"fail_reason"`
				}
				if err := postJSON(ctx, client, dingtalkRegistrationBase+"/poll",
					map[string]string{"device_code": deviceCode}, &poll); err != nil {
					continue
				}
				if poll.ErrCode != 0 {
					continue
				}
				switch strings.ToUpper(poll.Status) {
				case "SUCCESS":
					if poll.ClientID == "" || poll.ClientSecret == "" {
						return QuickCredentials{}, fmt.Errorf("channel: 钉钉授权成功但没有返回凭证")
					}
					return QuickCredentials{AppID: poll.ClientID, AppSecret: poll.ClientSecret}, nil
				case "FAIL":
					reason := poll.FailReason
					if reason == "" {
						reason = "授权被拒绝"
					}
					return QuickCredentials{}, fmt.Errorf("channel: 钉钉授权失败: %s", reason)
				case "EXPIRED":
					return QuickCredentials{}, ErrQRLoginExpired
				}
			}
		},
	}, nil
}

// getJSON performs a GET and decodes the JSON body.
func getJSON(ctx context.Context, client *http.Client, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	return doJSON(client, req, out)
}

// postJSON performs a POST with a JSON body and decodes the response.
func postJSON(ctx context.Context, client *http.Client, target string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(client, req, out)
}

// doJSON runs a request and decodes its JSON body, keeping the read bounded.
func doJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// isDarwin and isWindows report the host platform for the generate endpoint's
// platform tag.
func isDarwin() bool  { return runtime.GOOS == "darwin" }
func isWindows() bool { return runtime.GOOS == "windows" }
