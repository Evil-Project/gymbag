package host

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Normalize canonicalizes a host or domain name by trimming whitespace,
// folding ASCII case to lower, stripping trailing dots, and unwrapping IPv6 brackets.
func Normalize(raw string) string {
	val := strings.TrimSpace(raw)
	val = strings.ToLower(val)
	val = strings.TrimSuffix(val, ".")
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		val = val[1 : len(val)-1]
	}
	return val
}

// FromHeader extracts the normalized hostname from an HTTP Host header value
// (supporting IPv4, domain names, host:port, [ipv6], and [ipv6]:port).
func FromHeader(value string) string {
	header := strings.TrimSpace(value)
	if header == "" {
		return ""
	}
	if strings.HasPrefix(header, "[") {
		if end := strings.IndexByte(header, ']'); end != -1 {
			return Normalize(header[1:end])
		}
	}
	if strings.Count(header, ":") == 1 {
		hostPart, _, _ := strings.Cut(header, ":")
		return Normalize(hostPart)
	}
	return Normalize(header)
}

// ParsePort parses and validates a TCP port in [1, 65535].
func ParsePort(raw, fieldName string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", fieldName)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", fieldName)
	}
	return port, nil
}

// ParseSiteAddress parses a site address token from a Gymbagfile/Caddyfile header
// (e.g., "example.com", "https://example.com:8443", "[::1]:8443", "localhost:8443").
func ParseSiteAddress(token string) (hostname string, port int, hasPort bool, err error) {
	raw := strings.TrimRight(strings.TrimSpace(token), ",")
	if raw == "" {
		return "", 0, false, fmt.Errorf("site address must not be empty")
	}
	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return "", 0, false, fmt.Errorf("invalid site address: %s", token)
		}
		if p := parsed.Port(); p != "" {
			portVal, portErr := ParsePort(p, "site port")
			if portErr != nil {
				return "", 0, false, portErr
			}
			return Normalize(parsed.Hostname()), portVal, true, nil
		}
		return Normalize(parsed.Hostname()), 0, false, nil
	}

	hostname = FromHeader(raw)
	if strings.HasPrefix(raw, "[") && strings.Contains(raw, "]:") {
		idx := strings.LastIndex(raw, "]:")
		portVal, portErr := ParsePort(raw[idx+2:], "site port")
		if portErr != nil {
			return "", 0, false, portErr
		}
		port = portVal
		hasPort = true
	} else if strings.Count(raw, ":") == 1 {
		_, portStr, _ := strings.Cut(raw, ":")
		portVal, portErr := ParsePort(portStr, "site port")
		if portErr != nil {
			return "", 0, false, portErr
		}
		port = portVal
		hasPort = true
	}
	if hostname == "" {
		return "", 0, false, fmt.Errorf("invalid site address: %s", token)
	}
	return hostname, port, hasPort, nil
}

// IsPublic reports whether host is a publicly routable DNS hostname eligible
// for HTTP-01 ACME issuance (not localhost, .local, wildcard, or an IP literal).
func IsPublic(raw string) bool {
	name := Normalize(raw)
	if name == "" || strings.HasPrefix(name, "*.") {
		return false
	}
	if name == "localhost" || name == "localhost.localdomain" ||
		strings.HasSuffix(name, ".local") || strings.HasSuffix(name, ".localhost") {
		return false
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return false
	}
	return true
}

// Covers reports whether a certificate/site pattern (exact or single-label wildcard "*.example.com")
// covers the given domain.
func Covers(pattern, domain string) bool {
	candidate := Normalize(pattern)
	target := Normalize(domain)
	if candidate == "" || target == "" {
		return false
	}
	if candidate == target {
		return true
	}
	if !strings.HasPrefix(candidate, "*.") {
		return false
	}
	suffix := candidate[1:] // ".example.com"
	if !strings.HasSuffix(target, suffix) {
		return false
	}
	label := target[:len(target)-len(suffix)]
	return label != "" && !strings.Contains(label, ".")
}

// FormatForURL wraps IPv6 hostnames in brackets when constructing URLs.
func FormatForURL(hostname string) string {
	h := Normalize(hostname)
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}
