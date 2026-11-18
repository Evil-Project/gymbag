package localpki

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"gymbag/internal/host"
)

const (
	unixHostsFilePath    = "/etc/hosts"
	windowsHostsFilePath = `C:\Windows\System32\drivers\etc\hosts`
	managedTagPrefix     = "# gymbag-managed:"
)

// ErrPermissionDenied is returned when modifying the system hosts file fails
// due to insufficient OS permissions (e.g. EACCES / EPERM / os.ErrPermission).
var ErrPermissionDenied = errors.New("localpki: permission denied modifying hosts file")

// DefaultHostsFilePath returns the platform-standard hosts file path.
func DefaultHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return windowsHostsFilePath
	}
	return unixHostsFilePath
}

// HostsManager manages automatic loopback entries in the system hosts file for local domains.
type HostsManager struct {
	mu            sync.Mutex
	hostsFilePath string
	instanceID    string
	logger        *slog.Logger
}

// NewHostsManager creates a HostsManager for hostsFilePath tagged with instanceID.
// If hostsFilePath is empty, it defaults to /etc/hosts on Unix and
// C:\Windows\System32\drivers\etc\hosts on Windows.
// If instanceID is empty, a random 16-character hex identifier is generated.
func NewHostsManager(hostsFilePath string, instanceID string, logger *slog.Logger) *HostsManager {
	path := strings.TrimSpace(hostsFilePath)
	if path == "" {
		path = DefaultHostsFilePath()
	}
	id := sanitizeInstanceID(instanceID)
	if id == "" {
		id = randomHexID()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HostsManager{
		hostsFilePath: path,
		instanceID:    id,
		logger:        logger,
	}
}

// HostsFilePath returns the managed hosts file path.
func (m *HostsManager) HostsFilePath() string {
	return m.hostsFilePath
}

// InstanceID returns the instance identifier used in "# gymbag-managed:<instanceID>" tags.
func (m *HostsManager) InstanceID() string {
	return m.instanceID
}

// EligibleDomains filters domains down to concrete (non-wildcard, non-IP) local hostnames
// suitable for /etc/hosts loopback entries (e.g. "xxx.internal", "api.internal", "app.localhost", "localhost").
func (m *HostsManager) EligibleDomains(domains []string) []string {
	return EligibleDomains(domains)
}

// EligibleDomains filters domains down to concrete (non-wildcard, non-IP) local hostnames
// suitable for /etc/hosts loopback entries (e.g. "xxx.internal", "api.internal", "app.localhost", "localhost").
func EligibleDomains(domains []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range domains {
		d := normalizeDomainToken(raw)
		if d == "" || strings.Contains(d, "*") {
			continue
		}
		if _, err := netip.ParseAddr(d); err == nil {
			continue
		}
		if !IsLocalDomain(d) || !isSafeHostsHostname(d) {
			continue
		}
		if _, exists := seen[d]; !exists {
			seen[d] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}

// Setup ensures all eligible local domains in domains resolve in the hosts file.
// Any domain not already present on an existing uncommented hosts line gets both IPv4 (127.0.0.1)
// and IPv6 (::1) entries tagged with "# gymbag-managed:<instanceID>".
func (m *HostsManager) Setup(domains []string) (added []string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	eligible := EligibleDomains(domains)
	if len(eligible) == 0 {
		return nil, nil
	}

	rawBytes, filePerm, err := readHostsFile(m.hostsFilePath)
	if err != nil {
		if isPermissionError(err) {
			m.logPermissionWarning(eligible, err)
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("localpki: failed to read hosts file %s: %w", m.hostsFilePath, err)
	}

	existingContent := string(rawBytes)
	resolved := parseResolvedHosts(existingContent)

	var missing []string
	for _, domain := range eligible {
		if _, ok := resolved[domain]; !ok {
			missing = append(missing, domain)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	lineSep := detectLineSeparator(existingContent)
	tag := managedTagPrefix + m.instanceID

	var b strings.Builder
	b.Grow(len(existingContent) + len(missing)*96)
	b.WriteString(existingContent)
	if len(existingContent) > 0 && !strings.HasSuffix(existingContent, "\n") {
		b.WriteString(lineSep)
	}
	for _, domain := range missing {
		b.WriteString("127.0.0.1 ")
		b.WriteString(domain)
		b.WriteString(" ")
		b.WriteString(tag)
		b.WriteString(lineSep)

		b.WriteString("::1 ")
		b.WriteString(domain)
		b.WriteString(" ")
		b.WriteString(tag)
		b.WriteString(lineSep)
	}

	if err := os.WriteFile(m.hostsFilePath, []byte(b.String()), filePerm); err != nil {
		if isPermissionError(err) {
			m.logPermissionWarning(missing, err)
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("localpki: failed to write hosts file %s: %w", m.hostsFilePath, err)
	}

	return missing, nil
}

// Cleanup re-reads the hosts file and removes only lines containing "# gymbag-managed:<instanceID>".
// It is concurrency-safe and idempotent.
func (m *HostsManager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, err := os.Stat(m.hostsFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if isPermissionError(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("localpki: failed to stat hosts file %s: %w", m.hostsFilePath, err)
	}

	rawBytes, err := os.ReadFile(m.hostsFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if isPermissionError(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("localpki: failed to read hosts file %s: %w", m.hostsFilePath, err)
	}

	content := string(rawBytes)
	if content == "" {
		return nil
	}

	lineSep := detectLineSeparator(content)
	hadTrailingNewline := strings.HasSuffix(content, "\n")
	trimmed := content
	if hadTrailingNewline {
		trimmed = strings.TrimSuffix(trimmed, lineSep)
		if trimmed == content {
			trimmed = strings.TrimSuffix(trimmed, "\n")
		}
	}

	lines := strings.Split(trimmed, lineSep)
	kept := make([]string, 0, len(lines))
	removed := 0
	for _, line := range lines {
		if hasManagedInstanceTag(line, m.instanceID) {
			removed++
			continue
		}
		kept = append(kept, line)
	}

	if removed == 0 {
		return nil
	}

	var updated string
	if len(kept) > 0 {
		updated = strings.Join(kept, lineSep)
		if hadTrailingNewline {
			updated += lineSep
		}
	}

	if err := os.WriteFile(m.hostsFilePath, []byte(updated), info.Mode().Perm()); err != nil {
		if isPermissionError(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("localpki: failed to write cleaned hosts file %s: %w", m.hostsFilePath, err)
	}
	return nil
}

func (m *HostsManager) logPermissionWarning(domains []string, err error) {
	if m.logger == nil {
		return
	}
	m.logger.Warn(
		"insufficient permissions to update hosts file automatically; run with elevated privileges or add loopback entries manually",
		"hosts_file", m.hostsFilePath,
		"domains", strings.Join(domains, ", "),
		"hint", fmt.Sprintf("127.0.0.1 %s", strings.Join(domains, " ")),
		"error", err,
	)
}

func readHostsFile(path string) ([]byte, os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0o644, nil
		}
		return nil, 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	perm := info.Mode().Perm()
	if perm == 0 {
		perm = 0o644
	}
	return data, perm, nil
}

func parseResolvedHosts(content string) map[string]struct{} {
	resolved := make(map[string]struct{})
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if _, err := netip.ParseAddr(fields[0]); err != nil {
			continue
		}
		for _, hostname := range fields[1:] {
			norm := host.Normalize(hostname)
			if norm != "" {
				resolved[norm] = struct{}{}
			}
		}
	}
	return resolved
}

func hasManagedInstanceTag(line, instanceID string) bool {
	marker := managedTagPrefix + instanceID
	search := line
	for {
		idx := strings.Index(search, marker)
		if idx < 0 {
			return false
		}
		after := search[idx+len(marker):]
		if after == "" || after[0] == ' ' || after[0] == '\t' || after[0] == '\r' {
			return true
		}
		search = search[idx+len(managedTagPrefix):]
	}
}

func detectLineSeparator(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

func isSafeHostsHostname(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.Contains(name, "..") {
		return false
	}
	for i := 0; i < len(name); i++ {
		ch := name[i]
		isLowerAlpha := ch >= 'a' && ch <= 'z'
		isDigit := ch >= '0' && ch <= '9'
		if !isLowerAlpha && !isDigit && ch != '-' && ch != '.' && ch != '_' {
			return false
		}
	}
	return true
}

func sanitizeInstanceID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func randomHexID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "default"
	}
	return hex.EncodeToString(buf[:])
}
