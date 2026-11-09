package plugin

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chatgo/internal/config"
)

// maxPackageBytes bounds an archive, and maxPackageFiles its entry count.
const (
	maxPackageBytes = 20 << 20
	maxPackageFiles = 2000
)

// InstallOptions describes one installation request.
type InstallOptions struct {
	// Kind is dir, zip, git or npm.
	Kind string
	// Path is the directory or zip path for local sources.
	Path string
	// URL is the git URL or npm package name.
	URL string
	// Ref selects a git branch/tag.
	Ref string
	// Permissions are the capabilities the user confirmed.
	Permissions []string
	// Settings are initial plugin settings.
	Settings map[string]string
	// Enabled loads the plugin after installing.
	Enabled bool
}

// Install copies, validates and registers a plugin. It stages everything
// first, so a failure never damages the installed version.
func (m *Manager) Install(ctx context.Context, opts InstallOptions) (Manifest, error) {
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return Manifest{}, err
	}
	stage, err := os.MkdirTemp(m.root, ".staging-*")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(stage)

	var integrity string
	switch opts.Kind {
	case "dir":
		if opts.Path == "" {
			return Manifest{}, fmt.Errorf("缺少目录路径")
		}
		if err := copyTree(opts.Path, stage); err != nil {
			return Manifest{}, err
		}
	case "zip":
		sum, err := extractZip(opts.Path, stage)
		if err != nil {
			return Manifest{}, err
		}
		integrity = sum
	case "git":
		commit, err := cloneGit(ctx, opts.URL, opts.Ref, stage)
		if err != nil {
			return Manifest{}, err
		}
		integrity = "git:" + commit
	case "npm":
		sum, err := downloadNPM(ctx, opts.URL, stage)
		if err != nil {
			return Manifest{}, err
		}
		integrity = sum
	default:
		return Manifest{}, fmt.Errorf("不支持的安装来源: %s", opts.Kind)
	}

	manifest, err := LoadManifest(stage)
	if err != nil {
		return Manifest{}, err
	}
	if opts.Enabled {
		if missing := MissingPermissions(manifest, opts.Permissions); len(missing) > 0 {
			return *manifest, fmt.Errorf("需要授权: %s", strings.Join(missing, ", "))
		}
	}

	target := filepath.Join(m.root, manifest.ID)
	backup := target + ".old"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return Manifest{}, fmt.Errorf("备份旧版本失败: %w", err)
		}
	}
	if err := os.Rename(stage, target); err != nil {
		_ = os.Rename(backup, target)
		return Manifest{}, fmt.Errorf("安装失败: %w", err)
	}
	_ = os.RemoveAll(backup)

	// A loaded instance keeps its module in memory and its contributions in the
	// config; revert them before the files are swapped.
	m.mu.RLock()
	old, hadOld := m.instances[manifest.ID]
	m.mu.RUnlock()
	if hadOld && old.status == StatusLoaded {
		m.unload(old)
	}

	m.mu.Lock()
	ref, exists := m.cfg.FindPlugin(manifest.ID)
	if !exists {
		ref = config.PluginRef{ID: manifest.ID}
	}
	ref.Enabled = opts.Enabled
	if opts.Permissions != nil {
		ref.Permissions = opts.Permissions
	}
	if opts.Settings != nil {
		ref.Settings = opts.Settings
	}
	ref.Source = config.PluginSource{
		Kind: opts.Kind, URL: opts.URL, Ref: opts.Ref, Integrity: integrity,
		Path: opts.Path, InstalledAt: time.Now(),
	}
	if exists {
		for i := range m.cfg.Plugins {
			if m.cfg.Plugins[i].ID == manifest.ID {
				m.cfg.Plugins[i] = ref
			}
		}
	} else {
		m.cfg.Plugins = append(m.cfg.Plugins, ref)
	}
	inst := &InstanceState{
		Manifest: manifest, Dir: target,
		DataDir: filepath.Join(m.dataRoot, manifest.ID),
		Ref:     ref, status: StatusDisabled,
	}
	m.instances[manifest.ID] = inst
	m.mu.Unlock()
	_ = config.SaveConfig(m.cfg)

	if opts.Enabled {
		if err := m.load(inst); err != nil {
			return *manifest, err
		}
	}
	return *manifest, nil
}

// Uninstall removes a plugin and optionally its data.
func (m *Manager) Uninstall(id string, removeData bool) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("插件不存在: %s", id)
	}
	if inst.status == StatusLoaded {
		m.unload(inst)
	}
	if err := os.RemoveAll(inst.Dir); err != nil {
		return err
	}
	if removeData {
		_ = os.RemoveAll(inst.DataDir)
	}
	m.mu.Lock()
	delete(m.instances, id)
	out := m.cfg.Plugins[:0]
	for _, p := range m.cfg.Plugins {
		if p.ID != id {
			out = append(out, p)
		}
	}
	m.cfg.Plugins = out
	m.mu.Unlock()
	return config.SaveConfig(m.cfg)
}

// copyTree copies src into dst, rejecting symlinks and oversized trees.
func copyTree(src, dst string) error {
	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	count := 0
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == src {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "..") {
			return fmt.Errorf("路径越界: %s", rel)
		}
		count++
		if count > maxPackageFiles {
			return fmt.Errorf("文件数量超过上限")
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxPackageBytes {
			return fmt.Errorf("文件过大: %s", rel)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(in, maxPackageBytes))
	return err
}

// extractZip unpacks a plugin zip safely and returns its sha256.
func extractZip(path, dst string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > maxPackageBytes {
		return "", fmt.Errorf("压缩包超过 20 MiB")
	}
	sum := sha256.Sum256(data)
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("打开 zip 失败: %w", err)
	}
	if len(r.File) > maxPackageFiles {
		return "", fmt.Errorf("压缩包文件数量超过上限")
	}
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		clean := filepath.Clean(filepath.FromSlash(name))
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return "", fmt.Errorf("压缩包路径越界: %s", f.Name)
		}
		target := filepath.Join(dst, clean)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, maxPackageBytes))
		rc.Close()
		out.Close()
		if copyErr != nil {
			return "", copyErr
		}
	}
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// cloneGit clones a repository into dst and returns its commit.
func cloneGit(ctx context.Context, url, ref, dst string) (string, error) {
	if strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("缺少 git 地址")
	}
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, dst)
	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone 失败: %s", strings.TrimSpace(string(out)))
	}
	rev := exec.CommandContext(ctx, "git", "-C", dst, "rev-parse", "HEAD")
	out, err := rev.Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// npmRegistry is the package registry npm installs read from.
var npmRegistry = "https://registry.npmjs.org"

// downloadNPM fetches a package tarball from the npm registry, verifies its
// integrity and extracts the package/ tree. It needs no npm binary.
func downloadNPM(ctx context.Context, pkg, dst string) (string, error) {
	name, version := splitNPMSpec(pkg)
	if name == "" {
		return "", fmt.Errorf("缺少 npm 包名")
	}
	metaURL := npmRegistry + "/" + name
	if version != "" {
		metaURL += "/" + version
	}
	meta, err := httpGetJSON(ctx, metaURL)
	if err != nil {
		return "", err
	}
	dist, _ := meta["dist"].(map[string]any)
	tarball, _ := dist["tarball"].(string)
	integrity, _ := dist["integrity"].(string)
	if tarball == "" {
		return "", fmt.Errorf("npm 包没有 tarball 地址")
	}
	data, err := httpGet(ctx, tarball, maxPackageBytes)
	if err != nil {
		return "", err
	}
	if err := verifyNPMIntegrity(data, integrity); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if err := extractTarGz(data, dst, "package/"); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func splitNPMSpec(pkg string) (string, string) {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return "", ""
	}
	if i := strings.LastIndex(pkg, "@"); i > 0 {
		return pkg[:i], pkg[i+1:]
	}
	return pkg, ""
}

func verifyNPMIntegrity(data []byte, integrity string) error {
	if integrity == "" {
		return nil
	}
	parts := strings.SplitN(integrity, "-", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "sha512") {
		return nil
	}
	want, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("npm integrity 无法解析: %w", err)
	}
	sum := sha512.Sum512(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), hex.EncodeToString(want)) {
		return fmt.Errorf("npm 包完整性校验失败")
	}
	return nil
}

// extractTarGz unpacks a gzipped tarball, keeping entries under prefix.
func extractTarGz(data []byte, dst, prefix string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("解压 npm 包失败: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		count++
		if count > maxPackageFiles {
			return fmt.Errorf("npm 包文件数量超过上限")
		}
		name := strings.ReplaceAll(hdr.Name, "\\", "/")
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		clean := filepath.Clean(filepath.FromSlash(rel))
		if rel == "" || clean == "." {
			continue
		}
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("npm 包路径越界: %s", hdr.Name)
		}
		target := filepath.Join(dst, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(tr, maxPackageBytes))
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}

func httpGetJSON(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry 返回 HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPackageBytes)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
