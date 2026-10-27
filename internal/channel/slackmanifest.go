package channel

import (
	"encoding/json"
	"net/url"
)

// SlackManifestURL is the deep link that opens Slack's app creation form with
// every setting the channel needs already filled in: Socket Mode, the four
// scopes and the two event subscriptions. The user still has to click Create
// and install the app, which is what produces the tokens — Slack gives no way
// to skip that step — but no field has to be found in the console by hand.
//
// It is not a QR flow: Slack has no code to scan. The caller shows the link as
// a button instead.
func SlackManifestURL() (string, error) {
	manifest := map[string]any{
		"_metadata": map[string]any{"major_version": 1, "minor_version": 1},
		"display_information": map[string]any{
			"name":        "ChatGo",
			"description": "ChatGo 桌面客户端的 Slack 渠道",
		},
		"features": map[string]any{
			"bot_user": map[string]any{"display_name": "ChatGo", "always_online": true},
		},
		"oauth_config": map[string]any{
			"scopes": map[string]any{
				"bot": []string{
					"chat:write",
					"app_mentions:read",
					"im:history",
					"im:read",
				},
			},
		},
		"settings": map[string]any{
			// Socket Mode needs no request URL, which is what makes the app
			// usable from a desktop client without a public address.
			"socket_mode_enabled": true,
			"event_subscriptions": map[string]any{
				"bot_events": []string{"app_mention", "message.im"},
			},
			"org_deploy_enabled":     false,
			"token_rotation_enabled": false,
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return "https://api.slack.com/apps?new_app=1&manifest_json=" + url.QueryEscape(string(raw)), nil
}

// SlackAppTokenURL is the page where the app-level token is generated. It has
// no deep link of its own, so the settings card links to the app list and the
// hint names the exact section.
const SlackAppTokenURL = "https://api.slack.com/apps"
