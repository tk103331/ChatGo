package plugin

// The installable catalog: the shipped plugins (provider presets, ACP
// agents, importers) ship as per-plugin zips on the GitHub release, and the
// app embeds only the index — the metadata the plugins page lists and the
// release URL each zip downloads from. Installing downloads the zip through
// the ordinary Install path, so a catalog plugin and a community one are
// the same thing from the manager's side.

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// catalogIndexJSON is generated from plugins/catalog by
// tools/gen-plugin-catalog; it carries every shipped plugin's metadata.
//
//go:embed catalog-index.json
var catalogIndexJSON []byte

// CatalogEntry is one installable shipped plugin.
type CatalogEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// Kind is the plugins page's filter category: catalog-providers,
	// catalog-acp, importers.
	Kind string `json:"kind"`
	// SHA256 pins the zip the release serves, so a swapped asset fails the
	// install instead of running unverified code.
	SHA256 string `json:"sha256"`
}

// Catalog lists every shipped plugin the release offers.
func Catalog() []CatalogEntry {
	var entries []CatalogEntry
	if err := json.Unmarshal(catalogIndexJSON, &entries); err != nil {
		return nil
	}
	return entries
}

// CatalogBase is the release the per-plugin zips download from. Overridable
// for tests.
var CatalogBase = "https://github.com/tk103331/ChatGo/releases/download/plugins"

// CatalogDownloadURL is where one entry's zip lives on the release.
func CatalogDownloadURL(entry CatalogEntry) string {
	return CatalogBase + "/plugin-" + entry.ID + ".zip"
}

// catalogDownloadTimeout bounds one zip download. Generous: the release CDN
// can be slow on some networks, and a zip is small enough that waiting
// beats failing.
const catalogDownloadTimeout = 60 * time.Second

// catalogURLFor resolves one entry's download URL. A var so a test can
// point it at a local server.
var catalogURLFor = CatalogDownloadURL

// InstallCatalog downloads one catalog entry's zip from the release and
// installs it through the ordinary path. The zip's hash is checked against
// the index, so a replaced asset never runs.
func (m *Manager) InstallCatalog(ctx context.Context, entry CatalogEntry, enabled bool) (Manifest, error) {
	if !catalogIDSafe(entry.ID) {
		return Manifest{}, fmt.Errorf("非法插件标识: %s", entry.ID)
	}
	data, err := catalogFetch(ctx, catalogURLFor(entry))
	if err != nil {
		return Manifest{}, err
	}
	if sum := fmt.Sprintf("%x", sha256.Sum256(data)); entry.SHA256 != "" && sum != entry.SHA256 {
		return Manifest{}, fmt.Errorf("插件包校验失败（SHA-256 不符），请更新应用后重试")
	}
	tmp, err := os.CreateTemp("", "chatgo-plugin-*.zip")
	if err != nil {
		return Manifest{}, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return Manifest{}, err
	}
	if err := tmp.Close(); err != nil {
		return Manifest{}, err
	}
	return m.Install(ctx, InstallOptions{
		Kind: "zip", Path: tmp.Name(),
		URL: catalogURLFor(entry), Ref: entry.Version,
		Permissions: DefaultPermissions(), Enabled: enabled,
	})
}

// catalogIDSafe rejects path escapes in an entry id before it is built into
// a URL or a file name.
func catalogIDSafe(id string) bool {
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return false
	}
	return path.Base(id) == id
}

// catalogFetch downloads a release asset with a bounded size.
func catalogFetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载插件包失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载插件包失败（HTTP %d）", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPackageBytes))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// PackCatalog zips every plugin of plugins/catalog into the per-plugin
// zips the release publishes, writing them to out and returning the index
// entries with their hashes. It is the release step's entry point; the
// shipped zips and the embedded index come from the same run, so the
// hashes always match.
func PackCatalog(catalogDir, out string) ([]CatalogEntry, error) {
	entries, err := os.ReadDir(catalogDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	var index []CatalogEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join(catalogDir, e.Name())
		manifest, err := LoadManifest(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		buf, sum, err := zipPluginDir(dir)
		if err != nil {
			return nil, err
		}
		zipName := "plugin-" + manifest.ID + ".zip"
		if err := os.WriteFile(path.Join(out, zipName), buf.Bytes(), 0o644); err != nil {
			return nil, err
		}
		index = append(index, CatalogEntry{
			ID: manifest.ID, Name: manifest.Name, Version: manifest.Version,
			Description: manifest.Description,
			Kind:        catalogKindOf(manifest), SHA256: sum,
		})
	}
	return index, nil
}

// catalogKindOf names the plugins page's filter category of a manifest.
func catalogKindOf(manifest *Manifest) string {
	switch {
	case len(manifest.Importers) > 0:
		return "importers"
	case len(manifest.Catalog) > 0:
		for _, e := range manifest.Catalog {
			if e.Kind == CatalogACP {
				return "catalog-acp"
			}
		}
		return "catalog-providers"
	}
	return ""
}

// zipPluginDir zips a plugin directory in memory, the entry names relative
// to the plugin root so the zip unpacks straight into the install target.
func zipPluginDir(dir string) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	h := sha256.New()
	zw := zip.NewWriter(io.MultiWriter(&buf, h))
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		zw.Close()
		return nil, "", err
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, fmt.Sprintf("%x", h.Sum(nil)), nil
}
