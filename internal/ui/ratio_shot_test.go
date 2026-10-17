package ui

import (
	"image/png"
	"os"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

func TestGitRatio(t *testing.T) {
	cw := testWindow()
	cm, _ := models.NewConversationManagerAt(t.TempDir())
	cw.convManager = cm
	cw.workspace = t.TempDir()
	cw.section = sectionApps
	cw.openApp("git")
	img := mgo.Render(cw.view, 1600, 950, 1)
	f, _ := os.Create("/tmp/git-ratio.png")
	png.Encode(f, img)
	f.Close()
	t.Logf("appLeftW=%.0f", cw.appLeftW)
}
