package waf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"gymbag/internal/config"
)

const maxMascotFileBytes = 16 * 1024 * 1024

var mascotAllowedExts = []string{".png", ".webp", ".gif", ".jpg", ".jpeg", ".svg", ".avif", ".ico"}

// AssetEntry holds the raw bytes, MIME Content-Type, and ETag of a served WAF asset.
type AssetEntry struct {
	Data        []byte
	ContentType string
	ETag        string
}

// AssetStore stores dynamic custom mascot assets alongside the built-in embedded mascot PNGs.
type AssetStore struct {
	custom atomic.Pointer[map[string]AssetEntry]
}

// NewAssetStore constructs an empty concurrency-safe AssetStore.
func NewAssetStore() *AssetStore {
	s := &AssetStore{}
	empty := make(map[string]AssetEntry)
	s.custom.Store(&empty)
	return s
}

// Replace atomically swaps the custom mascot asset map served by this store.
func (s *AssetStore) Replace(custom map[string]AssetEntry) {
	if s == nil {
		return
	}
	cloned := make(map[string]AssetEntry, len(custom))
	for k, v := range custom {
		cloned[k] = v
	}
	s.custom.Store(&cloned)
}

// Lookup retrieves an asset by filename from custom assets first, falling back to embedded assets.
func (s *AssetStore) Lookup(name string) (data []byte, contentType, etag string, ok bool) {
	if s != nil {
		if m := s.custom.Load(); m != nil {
			if entry, exists := (*m)[name]; exists {
				return entry.Data, entry.ContentType, entry.ETag, true
			}
		}
	}
	return AssetDataWithType(name)
}

// MascotSources holds resolved browser image URLs for the three mascot states.
type MascotSources struct {
	PensiveURL string
	HappyURL   string
	RejectURL  string
}

// DefaultMascotSources returns the built-in embedded mascot asset URLs.
func DefaultMascotSources() MascotSources {
	return MascotSources{
		PensiveURL: "/.waf/assets/" + PensiveAsset,
		HappyURL:   "/.waf/assets/" + HappyAsset,
		RejectURL:  "/.waf/assets/" + RejectAsset,
	}
}

func (m MascotSources) withDefaults() MascotSources {
	def := DefaultMascotSources()
	if strings.TrimSpace(m.PensiveURL) == "" {
		m.PensiveURL = def.PensiveURL
	}
	if strings.TrimSpace(m.HappyURL) == "" {
		m.HappyURL = def.HappyURL
	}
	if strings.TrimSpace(m.RejectURL) == "" {
		m.RejectURL = def.RejectURL
	}
	return m
}

// BuildPageCSP returns a Content-Security-Policy header allowing local assets ('self') and,
// when remote custom mascot URLs are configured, https: (and http: if needed).
func BuildPageCSP(m MascotSources) string {
	allowHTTPS := false
	allowHTTP := false
	for _, u := range []string{m.PensiveURL, m.HappyURL, m.RejectURL} {
		lower := strings.ToLower(strings.TrimSpace(u))
		if strings.HasPrefix(lower, "http://") {
			allowHTTP = true
			allowHTTPS = true
		} else if strings.HasPrefix(lower, "https://") {
			allowHTTPS = true
		}
	}
	imgSrc := "'self'"
	switch {
	case allowHTTP:
		imgSrc = "'self' https: http:"
	case allowHTTPS:
		imgSrc = "'self' https:"
	}
	return "default-src 'none'; img-src " + imgSrc + "; style-src 'unsafe-inline'; " +
		"script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'"
}

// ResolveMascot resolves effective Pensive, Happy, and Reject mascot URLs from per-site and global configs.
// Local files and directories are validated, loaded into memory as custom Assets, and mapped to
// deterministic /.waf/assets/custom-mascot-<hash><ext> URLs; remote http(s) URLs are kept directly.
func ResolveMascot(siteCfg, globalCfg *config.MascotConfig, baseDir string) (MascotSources, map[string]AssetEntry, error) {
	def := DefaultMascotSources()
	custom := make(map[string]AssetEntry)

	type slotSpec struct {
		slot       string
		siteSpec   string
		siteDef    string
		globalSpec string
		globalDef  string
		fallback   string
		dst        *string
	}
	out := MascotSources{}
	var siteSpec, siteDef, globalSpec, globalDef [3]string
	if siteCfg != nil {
		siteSpec = [3]string{siteCfg.Pensive, siteCfg.Happy, siteCfg.Reject}
		siteDef = [3]string{siteCfg.Default, siteCfg.Default, siteCfg.Default}
	}
	if globalCfg != nil {
		globalSpec = [3]string{globalCfg.Pensive, globalCfg.Happy, globalCfg.Reject}
		globalDef = [3]string{globalCfg.Default, globalCfg.Default, globalCfg.Default}
	}

	slots := []slotSpec{
		{"pensive", siteSpec[0], siteDef[0], globalSpec[0], globalDef[0], def.PensiveURL, &out.PensiveURL},
		{"happy", siteSpec[1], siteDef[1], globalSpec[1], globalDef[1], def.HappyURL, &out.HappyURL},
		{"reject", siteSpec[2], siteDef[2], globalSpec[2], globalDef[2], def.RejectURL, &out.RejectURL},
	}

	for _, sc := range slots {
		raw := firstNonEmptyMascot(sc.siteSpec, sc.siteDef, sc.globalSpec, sc.globalDef)
		if raw == "" {
			*sc.dst = sc.fallback
			continue
		}
		if isRemoteHTTPURL(raw) {
			*sc.dst = raw
			continue
		}
		assetURL, name, entry, err := loadLocalMascotAsset(raw, sc.slot, baseDir)
		if err != nil {
			return MascotSources{}, nil, err
		}
		custom[name] = entry
		*sc.dst = assetURL
	}
	return out, custom, nil
}

func firstNonEmptyMascot(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func isRemoteHTTPURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func expandMascotPath(rawPath, baseDir string) string {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if p == "~" {
				p = home
			} else {
				p = filepath.Join(home, p[2:])
			}
		}
	}
	if !filepath.IsAbs(p) && baseDir != "" {
		if _, err := os.Stat(p); err != nil {
			p = filepath.Join(baseDir, p)
		}
	}
	return p
}

func loadLocalMascotAsset(rawPath, slot, baseDir string) (assetURL, assetName string, entry AssetEntry, err error) {
	resolved := expandMascotPath(rawPath, baseDir)
	info, statErr := os.Stat(resolved)
	if statErr != nil {
		return "", "", AssetEntry{}, fmt.Errorf("failed to load custom mascot %q: %w", rawPath, statErr)
	}
	filePath := resolved
	if info.IsDir() {
		found, findErr := findMascotFileInDir(resolved, slot)
		if findErr != nil {
			return "", "", AssetEntry{}, findErr
		}
		filePath = found
		info, statErr = os.Stat(filePath)
		if statErr != nil {
			return "", "", AssetEntry{}, fmt.Errorf("failed to stat custom mascot %q: %w", filePath, statErr)
		}
	}
	if info.Size() > maxMascotFileBytes {
		return "", "", AssetEntry{}, fmt.Errorf("custom mascot %q exceeds max size (%d bytes)", rawPath, maxMascotFileBytes)
	}
	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return "", "", AssetEntry{}, fmt.Errorf("failed to read custom mascot %q: %w", filePath, readErr)
	}
	if len(data) == 0 {
		return "", "", AssetEntry{}, fmt.Errorf("custom mascot %q is empty", filePath)
	}
	if int64(len(data)) > maxMascotFileBytes {
		return "", "", AssetEntry{}, fmt.Errorf("custom mascot %q exceeds max size (%d bytes)", rawPath, maxMascotFileBytes)
	}

	ext, contentType := detectMascotType(filePath, data)
	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:12])
	assetName = "custom-mascot-" + hashHex + ext
	assetURL = "/.waf/assets/" + assetName
	entry = AssetEntry{
		Data:        data,
		ContentType: contentType,
		ETag:        `"` + hashHex + `"`,
	}
	return assetURL, assetName, entry, nil
}

func findMascotFileInDir(dir, slot string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read custom mascot directory %q: %w", dir, err)
	}
	byLower := make(map[string]string, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		byLower[strings.ToLower(ent.Name())] = filepath.Join(dir, ent.Name())
	}
	for _, prefix := range []string{slot, "default", "mascot"} {
		for _, ext := range mascotAllowedExts {
			if full, ok := byLower[prefix+ext]; ok {
				return full, nil
			}
		}
	}
	return "", fmt.Errorf("no mascot image found for %s in directory %s", slot, dir)
}

func detectMascotType(filePath string, data []byte) (ext, contentType string) {
	rawExt := strings.ToLower(filepath.Ext(filePath))
	switch rawExt {
	case ".png":
		return ".png", "image/png"
	case ".jpg", ".jpeg":
		return rawExt, "image/jpeg"
	case ".gif":
		return ".gif", "image/gif"
	case ".webp":
		return ".webp", "image/webp"
	case ".svg":
		return ".svg", "image/svg+xml"
	case ".avif":
		return ".avif", "image/avif"
	case ".ico":
		return ".ico", "image/x-icon"
	}
	detected := http.DetectContentType(data)
	switch {
	case strings.HasPrefix(detected, "image/png"):
		return ".png", "image/png"
	case strings.HasPrefix(detected, "image/jpeg"):
		return ".jpg", "image/jpeg"
	case strings.HasPrefix(detected, "image/gif"):
		return ".gif", "image/gif"
	case strings.HasPrefix(detected, "image/webp"):
		return ".webp", "image/webp"
	case strings.HasPrefix(detected, "image/x-icon"), strings.HasPrefix(detected, "image/vnd.microsoft.icon"):
		return ".ico", "image/x-icon"
	case strings.Contains(strings.ToLower(string(data[:min(len(data), 512)])), "<svg"):
		return ".svg", "image/svg+xml"
	default:
		if rawExt != "" {
			return rawExt, "application/octet-stream"
		}
		return ".bin", detected
	}
}
