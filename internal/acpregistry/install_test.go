package acpregistry

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// zipBytes builds an in-memory zip holding the given files.
func zipBytes(files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, _ := w.Create(name)
		f.Write([]byte(body))
	}
	w.Close()
	return buf.Bytes()
}

// tgzBytes builds an in-memory gzipped tar holding the given files.
func tgzBytes(files map[string]string) []byte {
	var tarbuf bytes.Buffer
	tw := tar.NewWriter(&tarbuf)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}
		tw.WriteHeader(hdr)
		tw.Write([]byte(body))
	}
	tw.Close()
	var gzbuf bytes.Buffer
	gw := gzip.NewWriter(&gzbuf)
	gw.Write(tarbuf.Bytes())
	gw.Close()
	return gzbuf.Bytes()
}

// The platform picker prefers the platform's own key, and on Linux falls
// back to the musl build of the same architecture.
func TestBinaryTargetPrefersThenFallsBack(t *testing.T) {
	glibc := BinaryDist{Archive: "https://e/glibc"}
	musl := BinaryDist{Archive: "https://e/musl"}
	a := Agent{Distribution: Distribution{Binary: map[string]BinaryDist{
		"linux-x86_64": glibc, "linux-x86_64-musl": musl,
	}}}
	if d, key, ok := a.BinaryTarget("linux", "amd64"); !ok || key != "linux-x86_64" || d.Archive != glibc.Archive {
		t.Fatalf("the platform's own key should win, got %q %v", key, d)
	}

	onlyMusl := Agent{Distribution: Distribution{Binary: map[string]BinaryDist{
		"linux-x86_64-musl": musl,
	}}}
	if d, key, ok := onlyMusl.BinaryTarget("linux", "amd64"); !ok || key != "linux-x86_64-musl" || d.Archive != musl.Archive {
		t.Fatalf("Linux should fall back to musl, got %q %v", key, d)
	}
	if _, _, ok := onlyMusl.BinaryTarget("darwin", "arm64"); ok {
		t.Fatal("a missing platform should not resolve")
	}
}

// extractArchive restores each supported format under dest, rejects entries
// that escape it, and a raw download lands under its own name.
func TestExtractArchive(t *testing.T) {
	dest := t.TempDir()

	// zip: regular entries, a directory, and a traversal attempt.
	evil := zipBytes(map[string]string{
		"bin/run":      "zip-run",
		"../../escape": "no",
		"/abs/escape":  "no",
		"ok/../inside": "cleaned",
	})
	zpath := filepath.Join(dest, "..", "a.zip")
	if err := os.WriteFile(zpath, evil, 0o644); err != nil {
		t.Fatal(err)
	}
	zdest := filepath.Join(dest, "z")
	if err := extractArchive(zpath, zdest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(zdest, "bin", "run")); string(got) != "zip-run" {
		t.Fatalf("the zip entry did not extract: %q", got)
	}
	// "ok/../inside" cleans to "inside" — inside dest, so it extracts.
	if got, _ := os.ReadFile(filepath.Join(zdest, "inside")); string(got) != "cleaned" {
		t.Fatalf("a cleanable name should extract: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "escape")); !os.IsNotExist(err) {
		t.Fatal("a ../ escape must not extract outside dest")
	}
	if _, err := os.Stat("/abs/escape"); !os.IsNotExist(err) {
		t.Fatal("an absolute path must not extract")
	}

	// tar.gz.
	tgz := tgzBytes(map[string]string{"./bin/goose": "tar-run"})
	tgzPath := filepath.Join(dest, "..", "b.tar.gz")
	if err := os.WriteFile(tgzPath, tgz, 0o644); err != nil {
		t.Fatal(err)
	}
	tdest := filepath.Join(dest, "t")
	if err := extractArchive(tgzPath, tdest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(tdest, "bin", "goose")); string(got) != "tar-run" {
		t.Fatalf("the tar entry did not extract: %q", got)
	}

	// raw executable.
	rawPath := filepath.Join(dest, "..", "c.bin")
	if err := os.WriteFile(rawPath, []byte("raw"), 0o644); err != nil {
		t.Fatal(err)
	}
	rdest := filepath.Join(dest, "r")
	os.MkdirAll(rdest, 0o755)
	if err := extractArchive(rawPath, rdest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(rdest, "c.bin")); string(got) != "raw" {
		t.Fatalf("the raw file did not land: %q", got)
	}
}

// resolveCommandPath maps the manifest's cmd spellings onto the extracted
// tree, Windows separators included.
func TestResolveCommandPath(t *testing.T) {
	cases := map[string]string{
		"./amp-acp":                       "amp-acp",
		"./dist-package/cursor-agent":     "dist-package/cursor-agent",
		`./dist-package\cursor-agent.cmd`: "dist-package/cursor-agent.cmd",
		"harn.exe":                        "harn.exe",
		"./coco-1.0.73+v9/cortex":         "coco-1.0.73+v9/cortex",
	}
	for cmd, want := range cases {
		got := resolveCommandPath("/dest", cmd)
		if got != filepath.FromSlash(want) && got != "/dest/"+want {
			t.Fatalf("resolveCommandPath(%q) = %q, want %q", cmd, got, want)
		}
	}
	if resolveCommandPath("/dest", "") != "" || resolveCommandPath("/dest", "/") != "" {
		t.Fatal("an empty cmd should not resolve")
	}
}

// InstallBinary walks the whole chain against a local HTTP server: the
// archive downloads, its digest is checked, the tree extracts and the
// executable is resolved, chmodded and returned. A bad digest stops the
// install before anything lands in dest.
func TestInstallBinaryEndToEnd(t *testing.T) {
	// A zip whose digest the manifest pins.
	archive := zipBytes(map[string]string{"bin/amp-acp": "run me"})
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()

	agent := Agent{ID: "amp", Name: "Amp", Version: "0.13.0",
		Distribution: Distribution{Binary: map[string]BinaryDist{
			PlatformKey(runtime.GOOS, runtime.GOARCH): {
				Archive: srv.URL + "/amp.zip", SHA256: digest, Cmd: "./bin/amp-acp",
				Args: []string{"--acp"}, Env: map[string]string{"K": "V"},
			},
		}}}
	dest := filepath.Join(t.TempDir(), "agents", "amp", "0.13.0")
	bin, err := InstallBinary(context.Background(), agent, runtime.GOOS, runtime.GOARCH, dest)
	if err != nil {
		t.Fatal(err)
	}
	if bin != filepath.Join(dest, "bin", "amp-acp") {
		t.Fatalf("the executable resolved to %q", bin)
	}
	if got, _ := os.ReadFile(bin); string(got) != "run me" {
		t.Fatalf("the binary did not extract: %q", got)
	}
	if info, err := os.Stat(bin); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("the binary should be executable, got %v %v", info.Mode(), err)
	}

	// A tampered digest refuses to install.
	agent.Distribution.Binary[PlatformKey(runtime.GOOS, runtime.GOARCH)] = BinaryDist{
		Archive: srv.URL + "/amp.zip", SHA256: strings.Repeat("0", 64), Cmd: "./bin/amp-acp",
	}
	if _, err := InstallBinary(context.Background(), agent, runtime.GOOS, runtime.GOARCH, dest); err == nil {
		t.Fatal("a bad digest should fail the install")
	} else if !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("the failure should name the digest check: %v", err)
	}

	// A platform the entry does not ship for refuses too.
	if _, err := InstallBinary(context.Background(), agent, "plan9", "arm", dest); err == nil {
		t.Fatal("a missing platform should fail the install")
	}
}
