package waf

import (
	"context"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

// reqView provides a zero-allocation, lazy-cached view of either a live *http.Request
// or a pre-built IncomingRequest across the entire WAF rule chain.
type reqView struct {
	httpReq *http.Request
	inc     *IncomingRequest

	method    string
	host      string
	path      string
	rawTarget string
	query     string
	clientIP  string

	uaLoaded        bool
	ua              string
	lowerUALoaded   bool
	lowerUA         string
	lowerPathLoaded bool
	lowerPath       string
	decodedQLoaded  bool
	decodedQuery    string
	lowerQLoaded    bool
	lowerDecodedQ   string
	threatChecked   bool
	isThreat        bool
	vpnIPChecked    bool
	isVPNIP         bool
	aiIPChecked     bool
	isAIIP          bool
	aiMatchChecked  bool
	isAIMatch       bool
}

func newHTTPReqView(r *http.Request, hostName, clientIP string) reqView {
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	method := r.Method
	if !isUpperASCII(method) {
		method = strings.ToUpper(method)
	}
	return reqView{
		httpReq:   r,
		method:    method,
		host:      hostName,
		path:      path,
		rawTarget: r.RequestURI,
		query:     r.URL.RawQuery,
		clientIP:  clientIP,
	}
}

func newIncomingReqView(req *IncomingRequest) reqView {
	method := req.Method
	if !isUpperASCII(method) {
		method = strings.ToUpper(method)
	}
	return reqView{
		inc:       req,
		method:    method,
		host:      req.Host,
		path:      req.Path,
		rawTarget: req.RawTarget,
		query:     req.Query,
		clientIP:  req.ClientIP,
	}
}

func (v *reqView) toIncoming() IncomingRequest {
	if v.inc != nil {
		return *v.inc
	}
	inc := FromHTTPRequest(v.httpReq, v.clientIP)
	v.inc = &inc
	return inc
}

func (v *reqView) getRawTarget() string {
	if v.rawTarget != "" {
		return v.rawTarget
	}
	if v.httpReq != nil {
		if ep := v.httpReq.URL.EscapedPath(); ep != "" {
			if v.query != "" {
				v.rawTarget = ep + "?" + v.query
			} else {
				v.rawTarget = ep
			}
			return v.rawTarget
		}
	}
	if v.query != "" {
		v.rawTarget = v.path + "?" + v.query
	} else {
		v.rawTarget = v.path
	}
	return v.rawTarget
}

func (v *reqView) getHeader(canonical, lower string) string {
	if v.inc != nil {
		return v.inc.Headers[lower]
	}
	if lower == "host" && v.httpReq.Host != "" {
		return v.httpReq.Host
	}
	vals := v.httpReq.Header[canonical]
	if len(vals) == 0 {
		return ""
	}
	if len(vals) == 1 {
		return vals[0]
	}
	return strings.Join(vals, ", ")
}

func (v *reqView) getCookie(name string) string {
	if v.inc != nil {
		return v.inc.Cookies[name]
	}
	cookieHeaders := v.httpReq.Header["Cookie"]
	if len(cookieHeaders) == 0 {
		return ""
	}
	var lastVal string
	for _, raw := range cookieHeaders {
		s := raw
		for len(s) > 0 {
			var part string
			if idx := strings.IndexByte(s, ';'); idx >= 0 {
				part = s[:idx]
				s = s[idx+1:]
			} else {
				part = s
				s = ""
			}
			eq := strings.IndexByte(part, '=')
			if eq < 0 {
				continue
			}
			k := trimASCIISpace(part[:eq])
			if k == name {
				lastVal = trimASCIISpace(part[eq+1:])
			}
		}
	}
	return lastVal
}

func (v *reqView) getUserAgent() string {
	if !v.uaLoaded {
		v.ua = v.getHeader("User-Agent", "user-agent")
		v.uaLoaded = true
	}
	return v.ua
}

func (v *reqView) getLowerUA() string {
	if !v.lowerUALoaded {
		v.lowerUA = asciiLowerIfNeeded(trimASCIISpace(v.getUserAgent()))
		v.lowerUALoaded = true
	}
	return v.lowerUA
}

func (v *reqView) getLowerPath() string {
	if !v.lowerPathLoaded {
		v.lowerPath = asciiLowerIfNeeded(v.path)
		v.lowerPathLoaded = true
	}
	return v.lowerPath
}

func (v *reqView) getDecodedQuery() string {
	if !v.decodedQLoaded {
		v.decodedQLoaded = true
		if v.query == "" {
			v.decodedQuery = ""
		} else if !strings.ContainsAny(v.query, "%+") {
			v.decodedQuery = v.query
		} else if decoded, err := url.QueryUnescape(v.query); err == nil {
			v.decodedQuery = decoded
		} else {
			v.decodedQuery = v.query
		}
	}
	return v.decodedQuery
}

func (v *reqView) getLowerDecodedQuery() string {
	if !v.lowerQLoaded {
		v.lowerQLoaded = true
		v.lowerDecodedQ = asciiLowerIfNeeded(v.getDecodedQuery())
	}
	return v.lowerDecodedQ
}

func (v *reqView) checkThreatIP(ctx context.Context, ip IntelProvider) bool {
	if ip == nil {
		return false
	}
	if !v.threatChecked {
		v.isThreat = ip.IsThreatIP(ctx, v.clientIP)
		v.threatChecked = true
	}
	return v.isThreat
}

func (v *reqView) checkVPNOrProxyIP(ctx context.Context, ip IntelProvider) bool {
	if ip == nil {
		return false
	}
	if !v.vpnIPChecked {
		if vp, ok := ip.(VPNIntelProvider); ok {
			v.isVPNIP = vp.IsVPNOrProxyIP(ctx, v.clientIP)
		}
		v.vpnIPChecked = true
	}
	return v.isVPNIP
}

func (v *reqView) checkAIIP(ip IntelProvider) bool {
	if ip == nil {
		return false
	}
	if !v.aiIPChecked {
		v.isAIIP = ip.IsAIIP(v.clientIP)
		v.aiIPChecked = true
	}
	return v.isAIIP
}

// fastRule is implemented by built-in WAF rules to evaluate directly on reqView without allocating maps.
type fastRule interface {
	evaluateView(ctx context.Context, v *reqView) *Decision
}

// fastDetector is implemented by built-in Detectors to evaluate directly on reqView.
type fastDetector interface {
	matchView(ctx context.Context, v *reqView) bool
}

func canonicalHeaderName(name string) string {
	return textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
}

func isUpperASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			return false
		}
	}
	return true
}

func asciiLowerIfNeeded(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || c >= 0x80 {
			return strings.ToLower(s)
		}
	}
	return s
}

func trimASCIISpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}
