package ui

import (
	"testing"

	"chatgo/internal/netx"

	mgo "github.com/egoist/mygo/ui"
)

// clickSwitch flips the named switch at its own box: the row's label text is
// drawn as text, so a named Click would hit the words instead of the toggle.
func clickSwitch(t *testing.T, tr *mgo.Tester, name string) {
	t.Helper()
	info, ok := tr.AccessByRole(mgo.RoleSwitch, name)
	if !ok {
		t.Fatalf("the %s switch is missing; texts: %v", name, tr.Texts())
	}
	tr.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
}

// The 系统 tab holds the proxy card, and the switch and the address field
// save through to the config and the applied proxy without a restart.
func TestSystemTabProxyCard(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsSystemTab
	tr := mgo.NewTester(cw.view, 1000, 800)
	for range 8 {
		tr.Frame()
	}
	for _, want := range []string{"网络代理", "使用网络代理", "保存"} {
		if !tr.HasText(want) {
			t.Fatalf("the 系统 tab lacks %q; texts: %v", want, tr.Texts())
		}
	}

	// Flipping the switch saves immediately; without an address the proxy
	// is on but not routed anywhere.
	clickSwitch(t, tr, "使用网络代理")
	tr.Frame()
	if !cw.cfg.Proxy.Enabled {
		t.Fatal("the switch did not turn the proxy on")
	}
	if got := netx.ProxyURL(); got != "" {
		t.Fatalf("an empty address should not route anything, got %q", got)
	}

	// A valid address saves and takes effect at once.
	cw.settings.proxy.url = "socks5://127.0.0.1:1080"
	tr.Frame()
	if err := tr.Click("保存"); err != nil {
		t.Fatalf("save: %v", err)
	}
	tr.Frame()
	if cw.cfg.Proxy.URL != "socks5://127.0.0.1:1080" {
		t.Fatalf("the address was not saved: %+v", cw.cfg.Proxy)
	}
	if got := netx.ProxyURL(); got != "socks5://127.0.0.1:1080" {
		t.Fatalf("saving should apply the proxy immediately, got %q", got)
	}
	if cw.settings.proxy.err != "" {
		t.Fatalf("a valid address should not be marked invalid: %q", cw.settings.proxy.err)
	}

	// An invalid address is rejected with an explanation, and keeps the
	// previously applied proxy untouched.
	cw.settings.proxy.url = "ftp://127.0.0.1:21"
	tr.Frame()
	if err := tr.Click("保存"); err != nil {
		t.Fatalf("save: %v", err)
	}
	tr.Frame()
	if cw.settings.proxy.err == "" {
		t.Fatal("an invalid address should show a validation error")
	}
	if !tr.HasText("不支持的协议") {
		t.Fatalf("the rejection should name the problem; texts: %v", tr.Texts())
	}
	if cw.cfg.Proxy.URL != "socks5://127.0.0.1:1080" {
		t.Fatalf("a rejected draft must not overwrite the config: %+v", cw.cfg.Proxy)
	}
	if got := netx.ProxyURL(); got != "socks5://127.0.0.1:1080" {
		t.Fatalf("a rejected draft must not change the applied proxy, got %q", got)
	}

	// Turning the switch back off saves that too.
	clickSwitch(t, tr, "使用网络代理")
	tr.Frame()
	if cw.cfg.Proxy.Enabled {
		t.Fatal("the switch did not turn the proxy off")
	}
	if got := netx.ProxyURL(); got != "" {
		t.Fatalf("turning the proxy off should clear it, got %q", got)
	}
}
