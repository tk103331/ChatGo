package ui

import (
	"testing"

	"chatgo/internal/plugin"
)

func TestPluginChannelKeyRoundTrip(t *testing.T) {
	key := pluginChannelKey("weather", "myim")
	if key != "plugin:weather:myim" {
		t.Fatalf("key = %q", key)
	}
	pid, typ, ok := parsePluginChannelKey(key)
	if !ok || pid != "weather" || typ != "myim" {
		t.Fatalf("parse = %q %q %v", pid, typ, ok)
	}
	for _, bad := range []string{"myim", "plugin:", "plugin:x", "plugin::x"} {
		if _, _, ok := parsePluginChannelKey(bad); ok {
			t.Fatalf("%q parsed as a plugin channel", bad)
		}
	}
}

func TestSplitTriggerType(t *testing.T) {
	pid, typ, ok := splitTriggerType("plugin:weather:interval")
	if !ok || pid != "weather" || typ != "interval" {
		t.Fatalf("split = %q %q %v", pid, typ, ok)
	}
	if _, _, ok := splitTriggerType("daily"); ok {
		t.Fatal("builtin schedule treated as a plugin trigger")
	}
}

func TestChannelFieldsForPlugin(t *testing.T) {
	key := pluginChannelKey("weather", "myim")
	pluginChannelMu.Lock()
	pluginChannelSpecs[key] = plugin.ChannelContribution{
		PluginID: "weather",
		Spec: plugin.ChannelSpec{
			Type: "myim", Name: "私有 IM",
			Fields: []plugin.SettingField{
				{Key: "token", Label: "Bot Token", Type: "secret"},
				{Key: "signing", Label: "签名密钥", Type: "secret"},
			},
		},
	}
	pluginChannelMu.Unlock()
	defer func() {
		pluginChannelMu.Lock()
		delete(pluginChannelSpecs, key)
		pluginChannelMu.Unlock()
	}()

	fields := channelFields(key)
	if len(fields) != 2 || fields[0].Label != "Bot Token" || !fields[1].Secret {
		t.Fatalf("fields = %+v", fields)
	}
	if channelTypeName(key) != "私有 IM" {
		t.Fatalf("name = %q", channelTypeName(key))
	}
}
