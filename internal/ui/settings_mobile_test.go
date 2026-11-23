package ui

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/relayclient"
)

func TestMobileTabShowsRelayCard(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.cfg.Relay.Enabled = true
	cw.cfg.Relay.URL = "wss://relay.example.com"
	cw.cfg.Relay.DesktopID = "d_test"
	cw.cfg.Relay.AllowPairing = true
	cw.section = sectionSettings
	cw.settings.tab = settingsMobileTab
	tr := mgo.NewTester(cw.view, 1000, 900)
	for range 8 {
		tr.Frame()
	}
	for _, want := range []string{"手机连接", "启用中继", "中继地址", "配对手机", "桌面 ID", "已配对设备"} {
		if !tr.HasText(want) {
			t.Fatalf("the mobile tab lacks %q; texts: %v", want, tr.Texts())
		}
	}
}

func TestMobileTabListsDisabledDevice(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.cfg.Relay.Enabled = true
	cw.cfg.Relay.URL = "wss://relay.example.com"
	cw.cfg.Relay.DesktopID = "d_test"
	cw.cfg.Relay.AllowPairing = true
	conn := relayclient.New(relayclient.Config{DesktopID: "d_test"})
	conn.SetDevices([]relayclient.Device{
		{ID: "dev_1", Name: "iPhone", Platform: "ios", Disabled: true},
	})
	cw.relay.conn = conn
	cw.section = sectionSettings
	cw.settings.tab = settingsMobileTab
	tr := mgo.NewTester(cw.view, 1000, 900)
	for range 8 {
		tr.Frame()
	}
	for _, want := range []string{"iPhone", "已停用", "启用", "吊销"} {
		if !tr.HasText(want) {
			t.Fatalf("the disabled device row lacks %q; texts: %v", want, tr.Texts())
		}
	}
}
