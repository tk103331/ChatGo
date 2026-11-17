package ui

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"
)

func TestWebTabShowsTokenCard(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.cfg.WebEnabled = true
	cw.cfg.WebToken = "saved-token"
	cw.section = sectionSettings
	cw.settings.tab = settingsWebTab
	tr := mgo.NewTester(cw.view, 1000, 900)
	for range 8 {
		tr.Frame()
	}
	for _, want := range []string{"访问令牌", "生成", "复制", "清除", "外部 Agent 端点"} {
		if !tr.HasText(want) {
			t.Fatalf("the Web tab lacks %q; texts: %v", want, tr.Texts())
		}
	}
	if cw.settings.webToken.value != "saved-token" {
		t.Fatalf("token draft = %q", cw.settings.webToken.value)
	}

	cw.settings.webToken.value = "draft-token"
	tr.Frame()
	if !tr.HasText("有未保存的修改") {
		t.Fatalf("an edited token should be marked unsaved; texts: %v", tr.Texts())
	}
	if cw.cfg.WebToken != "saved-token" {
		t.Fatalf("an unsaved draft changed the config: %q", cw.cfg.WebToken)
	}
}
