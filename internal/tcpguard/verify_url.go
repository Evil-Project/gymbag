package tcpguard

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/host"
)

var nonPublicIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isPublicIPAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, p := range nonPublicIPPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// DetectServerIPs inspects local network interfaces and (when httpClient != nil and no public
// interface IP is found) optionally queries https://api.ipify.org with a 1.5s fail-open timeout.
func DetectServerIPs(ctx context.Context, httpClient *http.Client) (publicIP string, allIPs []string) {
	seen := make(map[string]struct{})
	add := func(s string) {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			allIPs = append(allIPs, s)
		}
	}

	var pubV4, pubV6 string
	if ifAddrs, err := net.InterfaceAddrs(); err == nil {
		for _, ia := range ifAddrs {
			ipNet, ok := ia.(*net.IPNet)
			if !ok {
				continue
			}
			addr, ok := netip.AddrFromSlice(ipNet.IP)
			if !ok || !addr.IsValid() {
				continue
			}
			addr = addr.Unmap()
			if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsUnspecified() {
				continue
			}
			add(addr.String())
			if isPublicIPAddr(addr) {
				if addr.Is4() && pubV4 == "" {
					pubV4 = addr.String()
				} else if addr.Is6() && pubV6 == "" {
					pubV6 = addr.String()
				}
			}
		}
	}

	if pubV4 != "" {
		publicIP = pubV4
	} else if pubV6 != "" {
		publicIP = pubV6
	}

	if publicIP == "" && httpClient != nil {
		probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		if req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "https://api.ipify.org", nil); err == nil {
			req.Header.Set("User-Agent", "gymbag/0.1.0")
			if resp, err := httpClient.Do(req); err == nil {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					if addr, pErr := netip.ParseAddr(strings.TrimSpace(string(body))); pErr == nil {
						addr = addr.Unmap()
						if isPublicIPAddr(addr) {
							publicIP = addr.String()
							add(publicIP)
						}
					}
				}
			}
		}
	}
	return publicIP, allIPs
}

// VerifyCertTargets returns the SAN list (hostname or IP addresses) for the verify portal TLS certificate.
func VerifyCertTargets(cfg config.VerifyServerConfig, detectedPublicIP string, localIPs []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		norm := host.Normalize(s)
		if norm == "" {
			return
		}
		if _, ok := seen[norm]; !ok {
			seen[norm] = struct{}{}
			out = append(out, norm)
		}
	}

	if customHost, _, _, err := cfg.ParsedHost(); err == nil && customHost != "" {
		add(customHost)
		if addr, pErr := netip.ParseAddr(customHost); pErr == nil && addr.Unmap().IsLoopback() {
			add("127.0.0.1")
			add("::1")
		}
		return out
	}

	if detectedPublicIP != "" {
		add(detectedPublicIP)
	}
	for _, ip := range localIPs {
		add(ip)
	}
	add("127.0.0.1")
	add("::1")
	return out
}

// ResolveVerifyBaseURL constructs the HTTPS base URL (e.g. "https://verify.example.com:8443" or "https://203.0.113.5:8443")
// for SSH and Minecraft verification prompts.
func ResolveVerifyBaseURL(
	cfg config.VerifyServerConfig,
	svcURL string,
	svcPort int,
	localAddr net.Addr,
	remoteClientIP, detectedPublicIP string,
) string {
	effURL := strings.TrimSpace(svcURL)
	if effURL == "" {
		effURL = strings.TrimSpace(cfg.URL)
	}
	effPort := svcPort
	if effPort <= 0 {
		effPort = cfg.Port
	}

	if effURL != "" {
		parsedCfg := config.VerifyServerConfig{URL: effURL, Port: effPort}
		h, expPort, pathPrefix, err := parsedCfg.ParsedHost()
		if err == nil && h != "" {
			port := effPort
			if port <= 0 {
				port = expPort
			}
			if port <= 0 {
				port = config.DefaultVerifyPort
			}
			return formatHTTPSBase(h, port) + pathPrefix
		}
	}

	if effPort <= 0 {
		effPort = cfg.EffectivePort()
	}

	chosenIP := pickVerifyIP(localAddr, remoteClientIP, detectedPublicIP)
	return formatHTTPSBase(chosenIP, effPort)
}

func pickVerifyIP(localAddr net.Addr, remoteClientIP, detectedPublicIP string) string {
	var localIP netip.Addr
	if ta, ok := localAddr.(*net.TCPAddr); ok && ta != nil {
		if a, ok := netip.AddrFromSlice(ta.IP); ok {
			localIP = a.Unmap()
		}
	}
	remoteIP, _ := netip.ParseAddr(strings.TrimSpace(remoteClientIP))
	remoteIP = remoteIP.Unmap()

	if remoteIP.IsValid() && remoteIP.IsLoopback() {
		if localIP.IsValid() && !localIP.IsUnspecified() {
			return localIP.String()
		}
		if remoteIP.Is6() {
			return "::1"
		}
		return "127.0.0.1"
	}
	if localIP.IsValid() && isPublicIPAddr(localIP) {
		return localIP.String()
	}
	if remoteIP.IsValid() && remoteIP.IsPrivate() && localIP.IsValid() && !localIP.IsUnspecified() {
		return localIP.String()
	}
	if detectedPublicIP != "" {
		return detectedPublicIP
	}
	if localIP.IsValid() && !localIP.IsUnspecified() {
		return localIP.String()
	}
	return "127.0.0.1"
}

func formatHTTPSBase(hostname string, port int) string {
	formatted := host.FormatForURL(hostname)
	if port == 443 || port == 0 {
		return "https://" + formatted
	}
	return fmt.Sprintf("https://%s:%d", formatted, port)
}

func extractRemoteIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	if ta, ok := addr.(*net.TCPAddr); ok && ta != nil && ta.IP != nil {
		if a, ok := netip.AddrFromSlice(ta.IP); ok {
			return a.Unmap().String()
		}
	}
	raw := addr.String()
	if h, _, err := net.SplitHostPort(raw); err == nil {
		if a, pErr := netip.ParseAddr(strings.Trim(h, "[]")); pErr == nil {
			return a.Unmap().String()
		}
		return strings.Trim(h, "[]")
	}
	return strings.Trim(raw, "[]")
}
