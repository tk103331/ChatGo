// Binary distribution support: download the platform archive, verify its
// sha256, extract it safely and resolve the command the entry names. Used
// by the settings dialog's 安装 action for agents without an npx/uvx
// distribution.
package acpregistry

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"chatgo/internal/netx"
)

// maxBinaryBytes bounds one archive download. A CLI binary runs large — a
// few hundred megabytes at the extreme — so the cap sits far above the
// packages of the plugin catalog.
const maxBinaryBytes = 500 << 20

// BinaryTarget picks the entry's archive for the platform: the platform's
// own key first, then, on Linux, the musl build of the same architecture —
// a glibc host may still prefer a statically linked musl build, and the
// unsuffixed key is what PlatformKey already names.
func (a Agent) BinaryTarget(goos, goarch string) (BinaryDist, string, bool) {
	key := PlatformKey(goos, goarch)
	if d, ok := a.Distribution.Binary[key]; ok {
		return d, key, true
	}
	if goos == "linux" {
		arch := "x86_64"
		if goarch == "arm64" {
			arch = "aarch64"
		}
		musl := "linux-" + arch + "-musl"
		if d, ok := a.Distribution.Binary[musl]; ok {
			return d, musl, true
		}
	}
	return BinaryDist{}, "", false
}

// InstallBinary installs the entry's binary archive for the platform into
// dest — one directory per registry id and version — and returns the
// absolute path of the executable its manifest names. An existing dest is
// replaced wholesale, so reinstalling an entry can never mix two versions.
func InstallBinary(ctx context.Context, a Agent, goos, goarch, dest string) (string, error) {
	dist, _, ok := a.BinaryTarget(goos, goarch)
	if !ok {
		return "", fmt.Errorf("「%s」没有 %s/%s 的二进制发行", a.Name, goos, goarch)
	}
	if strings.TrimSpace(dist.Archive) == "" {
		return "", fmt.Errorf("「%s」的二进制发行缺少下载地址", a.Name)
	}
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	// The archive lands in a temp file beside dest, so a huge download is
	// never held in memory and never half-extracts into the destination.
	// The file keeps the archive's extension, because the extractor picks
	// the format by it.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*"+archiveExt(dist.Archive))
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := downloadArchive(ctx, dist.Archive, tmp); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := verifyArchive(tmpName, dist.SHA256); err != nil {
		return "", err
	}
	if err := extractArchive(tmpName, dest); err != nil {
		return "", err
	}
	bin := resolveCommandPath(dest, dist.Cmd)
	if bin == "" {
		return "", fmt.Errorf("「%s」的二进制发行没有可执行命令", a.Name)
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		return "", err
	}
	return bin, nil
}

// archiveExt is the extension the URL's own name ends with, lowercased —
// ".zip", ".tar.gz", "" for a raw executable. The registry's archive names
// carry the format.
func archiveExt(url string) string {
	name := url
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.IndexByte(name, '?'); i >= 0 {
		name = name[:i]
	}
	low := strings.ToLower(name)
	for _, ext := range []string{".tar.bz2", ".tar.gz", ".tbz2", ".tgz", ".zip"} {
		if strings.HasSuffix(low, ext) {
			return ext
		}
	}
	return ""
}

// downloadArchive streams the archive to w, bounding its size.
func downloadArchive(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("下载地址无效：%v", err)
	}
	resp, err := netx.Default().Do(req)
	if err != nil {
		return fmt.Errorf("下载失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	if _, err := io.Copy(w, io.LimitReader(resp.Body, maxBinaryBytes)); err != nil {
		return fmt.Errorf("下载中断：%v", err)
	}
	return nil
}

// verifyArchive checks the downloaded archive against the manifest's digest.
// An entry without a digest is installed as-is; the registry's own CI checks
// what it publishes, and the download runs over TLS.
func verifyArchive(path, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxBinaryBytes)); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("校验失败：期望 sha256 %s，实际 %s", want, got)
	}
	return nil
}

// extractArchive unpacks the archive by its extension: zip, tar.gz, tgz,
// tar.bz2 and tbz2, or a raw executable when it matches none. Installer
// formats the registry does not accept never appear in the index.
func extractArchive(path, dest string) error {
	low := strings.ToLower(path)
	switch {
	case strings.HasSuffix(low, ".zip"):
		return extractZipFile(path, dest)
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"),
		strings.HasSuffix(low, ".tar.bz2"), strings.HasSuffix(low, ".tbz2"):
		return extractTarFile(path, dest)
	default:
		return extractRawFile(path, dest)
	}
}

// extractZipFile unpacks a zip. Entry names stay inside dest: an absolute
// path or one that climbs out is dropped, so a hostile archive cannot write
// outside the install directory.
func extractZipFile(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("解压失败：%v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		name, ok := safeEntryName(f.Name)
		if !ok || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeArchiveFile(filepath.Join(dest, name), rc, f.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// extractTarFile unpacks a gzipped or bzip2-compressed tar. Regular files
// and directories are restored; symlinks and other special entries are
// skipped rather than trusted.
func extractTarFile(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	low := strings.ToLower(path)
	var tr *tar.Reader
	switch {
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("解压失败：%v", err)
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	default:
		tr = tar.NewReader(bzip2.NewReader(f))
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("解压失败：%v", err)
		}
		name, ok := safeEntryName(hdr.Name)
		if !ok {
			continue
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			mode := os.FileMode(hdr.Mode)
			if mode.Perm() == 0 {
				mode = 0o644
			}
			if err := writeArchiveFile(target, tr, mode); err != nil {
				return err
			}
		}
	}
}

// extractRawFile places a downloaded executable under its own name.
func extractRawFile(path, dest string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	return writeArchiveFile(filepath.Join(dest, filepath.Base(path)), src, 0o755)
}

// safeEntryName reports the name as a clean relative path inside the
// archive, false when it escapes: absolute, or climbing with "..".
func safeEntryName(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") {
		return "", false
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", false
	}
	return filepath.FromSlash(clean), true
}

// writeArchiveFile writes one extracted entry with its mode, creating
// parent directories as needed.
func writeArchiveFile(path string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(r, maxBinaryBytes))
	return err
}

// resolveCommandPath maps the manifest's cmd onto the extracted tree:
// "./dist-package\cursor-agent.cmd" and "harn.exe" both resolve inside
// dest. Empty when cmd names nothing.
func resolveCommandPath(dest, cmd string) string {
	c := strings.ReplaceAll(strings.TrimSpace(cmd), "\\", "/")
	c = strings.TrimPrefix(c, "./")
	c = strings.TrimPrefix(c, "/")
	c = strings.Trim(c, "/")
	if c == "" {
		return ""
	}
	return filepath.Join(dest, filepath.FromSlash(c))
}
