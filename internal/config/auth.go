package config

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	// DefaultAutheliaURI is Authelia's standard ForwardAuth verification endpoint path.
	DefaultAutheliaURI = "/api/authz/forward-auth"
	// DefaultAuthTimeout is the default timeout (in seconds) for forward-auth subrequests.
	DefaultAuthTimeout = 5
)

// DefaultAutheliaCopyHeaders lists the standard identity headers returned by Authelia and copied to upstreams.
var DefaultAutheliaCopyHeaders = []string{
	"Remote-User",
	"Remote-Groups",
	"Remote-Email",
	"Remote-Name",
}

// ForwardAuthConfig configures external forward authentication (such as Authelia or Caddy forward_auth).
type ForwardAuthConfig struct {
	Provider        string            `json:"provider,omitempty"`          // "authelia" or "forward_auth"
	URL             string            `json:"url"`                         // Base URL (or full verify URL) of the auth service
	URI             string            `json:"uri,omitempty"`               // Auth verify URI path (default "/api/authz/forward-auth" for authelia)
	CopyHeaders     []string          `json:"copy_headers,omitempty"`      // Auth response headers copied to upstream ("Src" or "Src>Dst")
	Headers         map[string]string `json:"headers,omitempty"`           // Extra headers sent to the auth endpoint
	Paths           []string          `json:"paths,omitempty"`             // Optional path matchers requiring auth (empty = all paths)
	ExceptPaths     []string          `json:"except_paths,omitempty"`      // Optional path matchers bypassing auth
	PortalURL       string            `json:"portal_url,omitempty"`        // Optional Authelia portal URL for rd= / 401 browser redirect
	Timeout         int               `json:"timeout,omitempty"`           // Subrequest timeout in seconds (default 5)
	InsecureSkipTLS bool              `json:"insecure_skip_tls,omitempty"` // Skip TLS verification for internal self-signed auth endpoints
}

// Clone returns a deep copy of ForwardAuthConfig.
func (a *ForwardAuthConfig) Clone() *ForwardAuthConfig {
	if a == nil {
		return nil
	}
	cp := *a
	cp.CopyHeaders = slices.Clone(a.CopyHeaders)
	cp.Headers = cloneHeaderMap(a.Headers)
	cp.Paths = slices.Clone(a.Paths)
	cp.ExceptPaths = slices.Clone(a.ExceptPaths)
	return &cp
}

// ParseCopyHeaderMapping parses a copy_headers entry ("Remote-User" or "Remote-User>X-Auth-User")
// into canonical HTTP header names (src, dst, err).
func ParseCopyHeaderMapping(spec string) (src, dst string, err error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return "", "", fmt.Errorf("copy_headers entry must not be empty")
	}
	if s, d, hasArrow := strings.Cut(trimmed, ">"); hasArrow {
		src = strings.TrimSpace(s)
		dst = strings.TrimSpace(d)
	} else {
		src = trimmed
		dst = trimmed
	}
	if src == "" || dst == "" {
		return "", "", fmt.Errorf("invalid copy_headers mapping %q", spec)
	}
	if strings.ContainsAny(src, " \t\r\n:") || strings.ContainsAny(dst, " \t\r\n:") {
		return "", "", fmt.Errorf("invalid header name in copy_headers mapping %q", spec)
	}
	return http.CanonicalHeaderKey(src), http.CanonicalHeaderKey(dst), nil
}

// Validate normalizes and checks ForwardAuthConfig.
func (a *ForwardAuthConfig) Validate() error {
	if a == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(a.Provider)) {
	case "":
		a.Provider = ""
	case "authelia":
		a.Provider = "authelia"
	case "forward_auth", "forward-auth", "forwardauth":
		a.Provider = "forward_auth"
	default:
		return fmt.Errorf("unsupported auth provider %q (expected authelia or forward_auth)", a.Provider)
	}

	normURL, err := NormalizeUpstream(a.URL, "auth url")
	if err != nil {
		return err
	}
	a.URL = normURL

	parsedBase, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("invalid auth url %q: %w", a.URL, err)
	}

	a.URI = strings.TrimSpace(a.URI)
	if a.URI != "" && !strings.HasPrefix(a.URI, "/") {
		a.URI = "/" + a.URI
	}

	baseHasPath := parsedBase.Path != "" && parsedBase.Path != "/"
	if a.URI == "" && !baseHasPath && (a.Provider == "" || a.Provider == "authelia") {
		a.Provider = "authelia"
		a.URI = DefaultAutheliaURI
	}
	if a.Provider == "" {
		a.Provider = "forward_auth"
	}

	isAutheliaLike := a.Provider == "authelia" ||
		strings.HasPrefix(a.URI, "/api/authz/") ||
		strings.HasPrefix(a.URI, "/api/verify") ||
		strings.HasPrefix(parsedBase.Path, "/api/authz/") ||
		strings.HasPrefix(parsedBase.Path, "/api/verify")

	if len(a.CopyHeaders) == 0 && isAutheliaLike {
		a.CopyHeaders = slices.Clone(DefaultAutheliaCopyHeaders)
	} else if len(a.CopyHeaders) > 0 {
		cleaned := make([]string, 0, len(a.CopyHeaders))
		for _, entry := range a.CopyHeaders {
			src, dst, mErr := ParseCopyHeaderMapping(entry)
			if mErr != nil {
				return mErr
			}
			if src == dst {
				cleaned = append(cleaned, src)
			} else {
				cleaned = append(cleaned, src+">"+dst)
			}
		}
		a.CopyHeaders = cleaned
	}

	if err := validateHeaderMap(a.Headers); err != nil {
		return fmt.Errorf("auth headers: %w", err)
	}

	normPaths := make([]string, 0, len(a.Paths))
	for _, p := range a.Paths {
		t := strings.TrimSpace(p)
		if t == "" || t == "*" || t == "/*" {
			continue
		}
		if _, err := CompilePathMatcher(t); err != nil {
			return fmt.Errorf("auth path %q: %w", t, err)
		}
		normPaths = append(normPaths, t)
	}
	a.Paths = normPaths

	normExcept := make([]string, 0, len(a.ExceptPaths))
	for _, p := range a.ExceptPaths {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if _, err := CompilePathMatcher(t); err != nil {
			return fmt.Errorf("auth except_paths %q: %w", t, err)
		}
		normExcept = append(normExcept, t)
	}
	a.ExceptPaths = normExcept

	a.PortalURL = strings.TrimSpace(a.PortalURL)
	if a.PortalURL != "" {
		if err := RequireHTTPURL(a.PortalURL, "auth portal_url"); err != nil {
			return err
		}
	}

	if a.Timeout < 0 {
		return fmt.Errorf("auth timeout must be greater than zero")
	}
	if a.Timeout == 0 {
		a.Timeout = DefaultAuthTimeout
	}
	return nil
}

// EndpointURL resolves the complete HTTP/HTTPS verification URL combining URL, URI, and optional PortalURL.
func (a *ForwardAuthConfig) EndpointURL() (string, error) {
	if a == nil {
		return "", fmt.Errorf("auth config is nil")
	}
	base, err := url.Parse(a.URL)
	if err != nil {
		return "", err
	}
	if a.URI != "" {
		uriRef, uErr := url.Parse(a.URI)
		if uErr != nil {
			return "", fmt.Errorf("invalid auth uri %q: %w", a.URI, uErr)
		}
		basePath := strings.TrimRight(base.Path, "/")
		uriPath := uriRef.Path
		if uriPath != "" && !strings.HasPrefix(uriPath, "/") {
			uriPath = "/" + uriPath
		}
		base.Path = basePath + uriPath
		if uriRef.RawQuery != "" {
			if base.RawQuery != "" {
				base.RawQuery = base.RawQuery + "&" + uriRef.RawQuery
			} else {
				base.RawQuery = uriRef.RawQuery
			}
		}
	}
	if base.Path == "" {
		base.Path = "/"
	}
	// Legacy Authelia /api/verify uses ?rd=<portal_url> when not already present.
	if a.PortalURL != "" && strings.HasSuffix(base.Path, "/api/verify") {
		q := base.Query()
		if q.Get("rd") == "" {
			q.Set("rd", a.PortalURL)
			base.RawQuery = q.Encode()
		}
	}
	return base.String(), nil
}
