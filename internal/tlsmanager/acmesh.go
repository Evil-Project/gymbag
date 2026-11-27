package tlsmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gymbag/internal/host"
)

// ExpandUserPath expands a leading "~/" using the current user's home directory and resolves
// non-existent relative paths against baseDir when provided.
func ExpandUserPath(rawPath, baseDir string) string {
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

// ResolveAcmeShPair locates and validates a non-expired certificate & key pair managed by acme.sh
// inside basePath (either an acme.sh home directory containing <domain>_ecc/ / <domain>/ dirs
// or a direct single-domain cert directory such as /root/.acme.sh/example.com_ecc).
func ResolveAcmeShPair(basePath, baseDir, domain, password string) (certPath, keyPath string, pair tls.Certificate, leaf *x509.Certificate, ok bool) {
	root := ExpandUserPath(basePath, baseDir)
	if root == "" {
		return "", "", tls.Certificate{}, nil, false
	}

	d := host.Normalize(domain)
	parent := wildcardParentDomain(d)

	var candidateDirs []string
	seenDirs := make(map[string]struct{})
	addDir := func(dir string) {
		if dir == "" {
			return
		}
		clean := filepath.Clean(dir)
		if _, exists := seenDirs[clean]; !exists {
			seenDirs[clean] = struct{}{}
			candidateDirs = append(candidateDirs, clean)
		}
	}

	// Prefer acme.sh domain subdirectories (ECC first, then RSA) if present, as well as basePath itself
	// (to support single-domain dirs or --install-cert target directories).
	if d != "" {
		addDir(root)
		addDir(filepath.Join(root, d+"_ecc"))
		addDir(filepath.Join(root, d))
		if parent != "" {
			addDir(filepath.Join(root, "*."+parent+"_ecc"))
			addDir(filepath.Join(root, "*."+parent))
			addDir(filepath.Join(root, parent+"_ecc"))
			addDir(filepath.Join(root, parent))
		}
	} else {
		addDir(root)
	}

	if password == "" {
		password = os.Getenv("GYMBAG_KEY_PASSWORD")
	}

	now := time.Now().UTC()
	for _, dir := range candidateDirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		certNames := candidateCertNames(d, parent)
		keyNames := candidateKeyNames(dir, d, parent)
		for _, cn := range certNames {
			cp := filepath.Join(dir, cn)
			if !isRegularFile(cp) {
				continue
			}
			for _, kn := range keyNames {
				kp := filepath.Join(dir, kn)
				if !isRegularFile(kp) {
					continue
				}
				loadedPair, loadedLeaf, err := LoadKeyPairWithPassword(cp, kp, password)
				if err != nil || loadedLeaf == nil {
					continue
				}
				if loadedLeaf.NotBefore.Add(-5 * time.Minute).After(now) {
					continue
				}
				if !loadedLeaf.NotAfter.After(now) {
					continue
				}
				if d != "" && !CertificateCovers(loadedLeaf, []string{d}) {
					continue
				}
				return cp, kp, loadedPair, loadedLeaf, true
			}
		}
	}
	return "", "", tls.Certificate{}, nil, false
}

func wildcardParentDomain(d string) string {
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "*.") {
		return strings.TrimPrefix(d, "*.")
	}
	idx := strings.IndexByte(d, '.')
	if idx > 0 && idx+1 < len(d) && strings.Contains(d[idx+1:], ".") {
		return d[idx+1:]
	}
	return ""
}

func candidateCertNames(domain, parent string) []string {
	var out []string
	seen := make(map[string]struct{})
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	add("fullchain.cer")
	add("fullchain.pem")
	if domain != "" {
		add(domain + ".cer")
		add(domain + ".pem")
	}
	if parent != "" {
		add("*." + parent + ".cer")
		add("*." + parent + ".pem")
		add(parent + ".cer")
		add(parent + ".pem")
	}
	add("cert.pem")
	add("cert.cer")
	return out
}

func candidateKeyNames(dir, domain, parent string) []string {
	var out []string
	seen := make(map[string]struct{})
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	if domain != "" {
		add(domain + ".key")
	}
	add("privkey.pem")
	add("privkey.key")
	add("key.pem")
	add("key.key")
	if parent != "" {
		add("*." + parent + ".key")
		add(parent + ".key")
	}
	if entries, err := os.ReadDir(dir); err == nil {
		var extra []string
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			n := ent.Name()
			lower := strings.ToLower(n)
			if strings.HasSuffix(lower, ".key") && lower != "ca.key" && !strings.HasSuffix(lower, "_ca.key") {
				extra = append(extra, n)
			}
		}
		sort.Strings(extra)
		for _, n := range extra {
			add(n)
		}
	}
	return out
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// AcmeShProvider loads user-managed TLS certificates issued by acme.sh from Path.
type AcmeShProvider struct {
	Path        string
	BaseDir     string
	Domains     []string
	KeyPassword string
	Required    bool
}

func (p *AcmeShProvider) effectivePassword() string {
	if p.KeyPassword != "" {
		return p.KeyPassword
	}
	return os.Getenv("GYMBAG_KEY_PASSWORD")
}

// Issue resolves valid non-expired certificates from acme.sh for all configured domains.
func (p *AcmeShProvider) Issue(_ context.Context) (*Material, error) {
	if strings.TrimSpace(p.Path) == "" {
		if p.Required {
			return nil, certErrorf("acme.sh certificate path must not be empty")
		}
		return nil, nil
	}
	pwd := p.effectivePassword()
	if len(p.Domains) == 0 {
		cp, kp, pair, _, ok := ResolveAcmeShPair(p.Path, p.BaseDir, "", pwd)
		if !ok {
			if p.Required {
				return nil, certErrorf("acme.sh certificate missing, expired, or invalid in %s", p.Path)
			}
			return nil, nil
		}
		return &Material{
			CertPath: cp,
			KeyPath:  kp,
			Source:   "acme.sh",
			Provider: "acme.sh",
			Cert:     pair,
		}, nil
	}

	var first *Material
	for i, d := range p.Domains {
		cp, kp, pair, leaf, ok := ResolveAcmeShPair(p.Path, p.BaseDir, d, pwd)
		if !ok {
			if p.Required {
				return nil, certErrorf("acme.sh certificate missing, expired, or invalid for %s in %s", d, p.Path)
			}
			return nil, nil
		}
		if i == 0 {
			first = &Material{
				CertPath: cp,
				KeyPath:  kp,
				Source:   "acme.sh",
				Provider: "acme.sh",
				Cert:     pair,
			}
			if CertificateCovers(leaf, p.Domains) {
				return first, nil
			}
		}
	}
	return first, nil
}
