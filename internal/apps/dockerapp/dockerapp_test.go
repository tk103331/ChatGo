package dockerapp

import (
	"strings"
	"testing"
	"time"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"plain info line", 1},
		{"ERROR connection refused", 3},
		{"[ERR] timeout", 3},
		{"FATAL: out of memory", 3},
		{"panic: runtime error", 3},
		{"WARN disk almost full", 2},
		{"warning: deprecated", 2},
		{"DEBUG verbose detail", 0},
		{"TRACE entry", 0},
	}
	for _, tc := range cases {
		if got := int(parseLevel(tc.line)); got != tc.want {
			t.Errorf("parseLevel(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.n); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestShortID(t *testing.T) {
	long := "abcdef1234567890abcdef1234567890abcdef12"
	if got := shortID(long); got != "abcdef123456" {
		t.Errorf("shortID = %q, want abcdef123456", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID = %q, want abc", got)
	}
}

func TestContainerName(t *testing.T) {
	c := Container{ID: "abcdef1234567890", Names: []string{"/my-app"}}
	if got := c.Name(); got != "my-app" {
		t.Errorf("Name = %q, want my-app", got)
	}
	if got := c.ShortID(); got != "abcdef123456" {
		t.Errorf("ShortID = %q, want abcdef123456", got)
	}
	empty := Container{ID: "abcdef1234567890"}
	if got := empty.Name(); got != "abcdef123456" {
		t.Errorf("Name = %q, want abcdef123456", got)
	}
}

func TestImageTag(t *testing.T) {
	im := Image{ID: "sha256:abcdef1234567890abcdef1234567890abcdef12", RepoTags: []string{"nginx:latest"}}
	if got := im.Tag(); got != "nginx:latest" {
		t.Errorf("Tag = %q, want nginx:latest", got)
	}
	if got := im.ShortID(); got != "abcdef123456" {
		t.Errorf("ShortID = %q, want abcdef123456", got)
	}
	none := Image{ID: "sha256:abcdef1234567890"}
	if got := none.Tag(); got != "<none>" {
		t.Errorf("Tag = %q, want <none>", got)
	}
}

func TestParseLS(t *testing.T) {
	out := `total 8
drwxr-xr-x 1 root root 4096 Jan  1 00:00 .
drwxr-xr-x 1 root root 4096 Jan  1 00:00 ..
-rw-r--r-- 1 root root   15 Jan  1 00:00 hello.txt
drwxr-xr-x 1 root root 4096 Jan  1 00:00 subdir
lrwxrwxrwx 1 root root   11 Jan  1 00:00 link -> /etc/hostname`
	entries := parseLS(out)
	if len(entries) != 3 {
		t.Fatalf("parseLS = %d entries, want 3", len(entries))
	}
	if entries[0].Name != "hello.txt" || entries[0].IsDir || entries[0].Size != 15 {
		t.Errorf("entry 0 = %+v, want hello.txt file size 15", entries[0])
	}
	if !entries[1].IsDir || entries[1].Name != "subdir" {
		t.Errorf("entry 1 = %+v, want subdir dir", entries[1])
	}
	if entries[2].Name != "link" {
		t.Errorf("entry 2 = %+v, want link", entries[2])
	}
}

func TestJoinPath(t *testing.T) {
	cases := []struct{ dir, name, want string }{
		{"/", "etc", "/etc"},
		{"", "var", "/var"},
		{"/etc", "nginx.conf", "/etc/nginx.conf"},
		{"/etc/", "nginx.conf", "/etc/nginx.conf"},
	}
	for _, tc := range cases {
		if got := joinPath(tc.dir, tc.name); got != tc.want {
			t.Errorf("joinPath(%q,%q) = %q, want %q", tc.dir, tc.name, got, tc.want)
		}
	}
}

func TestShortErr(t *testing.T) {
	if got := shortErr(multiLineErr{}); got != "line1" {
		t.Errorf("shortErr = %q, want line1", got)
	}
}

type multiLineErr struct{}

func (multiLineErr) Error() string { return "line1\nline2\n" }

type fakeHost struct{}

func (fakeHost) Root() string                              { return "" }
func (fakeHost) Workspace() string                         { return "" }
func (fakeHost) Post(fn func())                            { fn() }
func (fakeHost) Invalidate()                               {}
func (fakeHost) Toast(msg string, sev mcore.Severity)                  { _ = msg; _ = sev }
func (fakeHost) Confirm(_, _ string, a func())             { if a != nil { a() } }
func (fakeHost) Prompt(_, _, _ string, a func(string))     { if a != nil { a("") } }
func (fakeHost) OpenFile(string)                           {}
func (fakeHost) OpenFileDialog(string) string              { return "" }
func (fakeHost) SaveFileDialog(string) string              { return "" }
func (fakeHost) Language(string) string                    { return "" }
func (fakeHost) NewID() string                             { return "" }
func (fakeHost) NativeWindow() *mygo.Window               { return nil }

func TestTermPaneNilTerm(t *testing.T) {
	cl := New(fakeHost{})
	cl.mu.Lock()
	cl.st.containers = []Container{{ID: "abc", Names: []string{"/t"}, Image: "img", State: "running"}}
	cl.st.detail = "abc"
	cl.st.detailTab = tabTerm
	cl.st.term = nil
	cl.st.termBusy = true
	cl.mu.Unlock()
	img := mgo.Render(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Mode: mcore.System})
		cl.View(c)
	}, 800, 600, 1)
	if img == nil {
		t.Fatal("render returned nil image")
	}
}

func TestDetailPaneTabBar(t *testing.T) {
	cl := New(fakeHost{})
	cl.mu.Lock()
	cl.st.containers = []Container{{ID: "abc", Names: []string{"/t"}, Image: "img", State: "running"}}
	cl.st.detail = "abc"
	cl.st.detailTab = tabInfo
	cl.mu.Unlock()
	img := mgo.Render(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Mode: mcore.System})
		cl.View(c)
	}, 800, 600, 1)
	if img == nil {
		t.Fatal("render returned nil image")
	}
}

func TestFilesPaneRender(t *testing.T) {
	cl := New(fakeHost{})
	cl.mu.Lock()
	cl.st.containers = []Container{{ID: "abc", Names: []string{"/t"}, Image: "img", State: "running"}}
	cl.st.detail = "abc"
	cl.st.detailTab = tabFiles
	cl.st.fileViewPath = "/etc/hosts"
	cl.st.fileContent = "127.0.0.1 localhost"
	cl.mu.Unlock()
	img := mgo.Render(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Mode: mcore.System})
		cl.View(c)
	}, 800, 600, 1)
	if img == nil {
		t.Fatal("render returned nil image")
	}
}

func TestFileTreeHelpers(t *testing.T) {
	cl := New(fakeHost{})
	cl.mu.Lock()
	cl.st.files["/"] = []fileEntry{
		{Name: "etc", IsDir: true},
		{Name: "app.jar", Size: 1024},
	}
	cl.st.filesLoaded["/"] = true
	cl.st.filesDir["/etc"] = true
	cl.mu.Unlock()
	if !cl.fileIsDir("/etc") {
		t.Error("/etc should be a dir")
	}
	if cl.fileIsDir("/app.jar") {
		t.Error("/app.jar should not be a dir")
	}
	if got := cl.fileLabel("/etc"); got != "etc/" {
		t.Errorf("fileLabel(/etc) = %q, want etc/", got)
	}
	if got := cl.fileLabel("/app.jar"); got != "app.jar" {
		t.Errorf("fileLabel(/app.jar) = %q, want app.jar", got)
	}
	if got := cl.fileSize("/app.jar"); got != 1024 {
		t.Errorf("fileSize(/app.jar) = %d, want 1024", got)
	}
	if !cl.fileLoaded("/") {
		t.Error("/ should be loaded")
	}
	if cl.fileLoaded("/etc") {
		t.Error("/etc should not be loaded yet")
	}
}

func TestDockerTTY(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available")
	}
	rows, err := listContainers()
	if err != nil {
		t.Skipf("list containers: %v", err)
	}
	var id string
	for _, c := range rows {
		if c.State == "running" {
			id = c.ID
			break
		}
	}
	if id == "" {
		t.Skip("no running container")
	}
	tm, err := openDockerTerminal(id, "", func() {})
	if err != nil {
		t.Fatalf("openDockerTerminal: %v", err)
	}
	defer tm.Close()
	time.Sleep(500 * time.Millisecond)
	tm.Send([]byte("echo TTY_MARKER_\x24\x24\r"))
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if strings.Contains(tm.Text(), "TTY_MARKER_") {
			return
		}
	}
	t.Fatalf("marker not seen; screen: %q", tm.Text())
}
