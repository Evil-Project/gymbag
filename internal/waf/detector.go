package waf

import (
	"context"
	"slices"
	"strings"
)

var defaultAIBotSignatures = []string{
	"gptbot",
	"chatgpt-user",
	"oai-searchbot",
	"claudebot",
	"claude-web",
	"claude-searchbot",
	"claude-user",
	"anthropic-ai",
	"google-extended",
	"googleother",
	"google-cloudvertexbot",
	"gemini-deep-research",
	"perplexitybot",
	"perplexity-user",
	"bytespider",
	"ccbot",
	"meta-externalagent",
	"meta-externalfetcher",
	"facebookbot",
	"applebot-extended",
	"amazonbot",
	"cohere-ai",
	"cohere-training-data-crawler",
	"deepseekbot",
	"duckassistbot",
	"mistralai-user",
	"youbot",
	"diffbot",
	"ai2bot",
	"omgili",
	"timpibot",
	"imagesiftbot",
	"webzio-extended",
	"kangaroo bot",
	"petalbot",
	"pangubot",
	"velenpublicwebcrawler",
	"friendlycrawler",
}

var defaultSuspiciousUASignatures = []string{
	"curl/",
	"wget/",
	"python-requests",
	"python-urllib",
	"python-httpx",
	"httpx/",
	"aiohttp/",
	"go-http-client",
	"java/",
	"okhttp/",
	"axios/",
	"node-fetch",
	"undici",
	"libwww-perl",
	"mechanize",
	"scrapy",
	"headlesschrome",
	"phantomjs",
	"playwright",
	"puppeteer",
	"selenium",
	"cypress",
	"httrack",
	"nikto",
	"sqlmap",
	"nmap",
	"masscan",
	"zgrab",
	"gobuster",
	"dirbuster",
	"ffuf",
	"nuclei",
	"wfuzz",
	"feroxbuster",
}

var defaultExactSuspiciousUAs = []string{
	"curl",
	"wget",
	"python",
	"go",
	"ruby",
	"php",
	"node",
}

var defaultSuspiciousPathPrefixes = []string{
	"/.env",
	"/.git",
	"/wp-admin",
	"/wp-login.php",
	"/xmlrpc.php",
	"/phpmyadmin",
	"/actuator",
	"/cgi-bin/",
	"/vendor/phpunit",
}

var defaultSuspiciousQueryTokens = []string{
	"union select",
	"<script",
	"javascript:",
	"onerror=",
	"onload=",
	"/etc/passwd",
	"cmd.exe",
	"information_schema",
}

// AIDetector identifies AI training crawlers, AI search crawlers, and LLM agent fetchers
// using both official AI crawler IP CIDR lists and User-Agent / agent header signatures.
type AIDetector struct {
	ac    *asciiAC
	intel IntelProvider
}

// NewAIDetector creates an AIDetector combining AI IP feeds, built-in signatures, and custom patterns.
func NewAIDetector(customPatterns []string, intel IntelProvider) *AIDetector {
	sigs := make([]string, 0, len(defaultAIBotSignatures)+len(customPatterns))
	sigs = append(sigs, defaultAIBotSignatures...)
	for _, p := range customPatterns {
		if trimmed := strings.ToLower(strings.TrimSpace(p)); trimmed != "" {
			sigs = append(sigs, trimmed)
		}
	}
	return &AIDetector{ac: compileASCIIAC(sigs), intel: intel}
}

// Match reports whether req originates from a known AI crawler IP block or LLM agent signature.
func (d *AIDetector) Match(ctx context.Context, req IncomingRequest) bool {
	v := newIncomingReqView(&req)
	return d.matchView(ctx, &v)
}

func (d *AIDetector) matchView(_ context.Context, v *reqView) bool {
	if v.aiMatchChecked {
		return v.isAIMatch
	}
	matched := d.evalAI(v)
	v.aiMatchChecked = true
	v.isAIMatch = matched
	return matched
}

func (d *AIDetector) evalAI(v *reqView) bool {
	if d.intel != nil && v.checkAIIP(d.intel) {
		return true
	}
	if v.getHeader("X-Ai-Agent", "x-ai-agent") != "" || v.getHeader("Sec-Agent", "sec-agent") != "" {
		return true
	}
	ua := trimASCIISpace(v.getUserAgent())
	if ua == "" {
		return false
	}
	return d.ac.MatchAny(ua)
}

// SuspicionDetector identifies malicious/threat IPs, automated scrapers, headless browsers,
// header spoofing, and reconnaissance/exploit probe traffic.
type SuspicionDetector struct {
	uaAC         *asciiAC
	queryAC      *asciiAC
	pathPrefixes []string
	aiDetector   Detector
	intel        IntelProvider
}

var defaultSuspiciousQueryAC = compileASCIIAC(defaultSuspiciousQueryTokens)

// NewSuspicionDetector creates a SuspicionDetector with optional threat intel and custom patterns.
func NewSuspicionDetector(customUAs, customPaths []string, aiDetector Detector, intel IntelProvider) *SuspicionDetector {
	uas := make([]string, 0, len(defaultSuspiciousUASignatures)+len(customUAs))
	uas = append(uas, defaultSuspiciousUASignatures...)
	for _, p := range customUAs {
		if trimmed := strings.ToLower(strings.TrimSpace(p)); trimmed != "" {
			uas = append(uas, trimmed)
		}
	}
	paths := make([]string, 0, len(defaultSuspiciousPathPrefixes)+len(customPaths))
	paths = append(paths, defaultSuspiciousPathPrefixes...)
	for _, p := range customPaths {
		if trimmed := strings.ToLower(strings.TrimSpace(p)); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return &SuspicionDetector{
		uaAC:         compileASCIIAC(uas),
		queryAC:      defaultSuspiciousQueryAC,
		pathPrefixes: paths,
		aiDetector:   aiDetector,
		intel:        intel,
	}
}

// Match reports whether req exhibits threat-intel, bot, scraper, spoofing, or exploit-probing signals.
func (d *SuspicionDetector) Match(ctx context.Context, req IncomingRequest) bool {
	v := newIncomingReqView(&req)
	return d.matchView(ctx, &v)
}

func (d *SuspicionDetector) matchView(ctx context.Context, v *reqView) bool {
	// 1. Exploit / reconnaissance path & query probes always trigger suspicion.
	lowerPath := v.getLowerPath()
	for _, prefix := range d.pathPrefixes {
		if strings.HasPrefix(lowerPath, prefix) {
			return true
		}
	}
	if v.query != "" {
		if d.queryAC.MatchAny(v.getDecodedQuery()) {
			return true
		}
	}

	// 2. IP security threat-feed & live reputation check (reuses cached result from ThreatIntelRule).
	if d.intel != nil && v.checkThreatIP(ctx, d.intel) {
		return true
	}

	rawUA := trimASCIISpace(v.getUserAgent())
	// 3. Missing or implausibly short User-Agent.
	if len(rawUA) < 8 {
		return true
	}

	// If recognized as a known AI crawler (without exploit probes or threat IP above), leave classification
	// to AIDetector so verify level 1 (only suspicious) and level 2 (only ai) stay cleanly orthogonal.
	if d.aiDetector != nil {
		if fd, ok := d.aiDetector.(fastDetector); ok {
			if fd.matchView(ctx, v) {
				return false
			}
		} else if d.aiDetector.Match(ctx, v.toIncoming()) {
			return false
		}
	}

	// 4. Known CLI tools, HTTP client libraries, scrapers, headless browsers, and vulnerability scanners.
	if len(rawUA) <= 6 && slices.Contains(defaultExactSuspiciousUAs, v.getLowerUA()) {
		return true
	}
	if d.uaAC.MatchAny(rawUA) {
		return true
	}

	// 5. Interactive browser impersonation without standard browser request headers.
	if claimsInteractiveBrowserASCII(rawUA) {
		if trimASCIISpace(v.getHeader("Accept", "accept")) == "" {
			return true
		}
		if trimASCIISpace(v.getHeader("Accept-Language", "accept-language")) == "" &&
			trimASCIISpace(v.getHeader("Sec-Fetch-Mode", "sec-fetch-mode")) == "" {
			return true
		}
	}

	return false
}

var (
	browserMozillaAC = compileASCIIAC([]string{"mozilla/5.0"})
	browserCompatAC  = compileASCIIAC([]string{"compatible;"})
	browserEngineAC  = compileASCIIAC([]string{"chrome/", "firefox/", "safari/", "edg/"})
)

func claimsInteractiveBrowserASCII(ua string) bool {
	return browserMozillaAC.MatchAny(ua) &&
		!browserCompatAC.MatchAny(ua) &&
		browserEngineAC.MatchAny(ua)
}

// OrDetector matches when any child Detector matches (Composite pattern).
type OrDetector struct {
	detectors []Detector
}

// NewOrDetector combines multiple Detectors via logical OR.
func NewOrDetector(detectors ...Detector) *OrDetector {
	return &OrDetector{detectors: detectors}
}

func (o *OrDetector) Match(ctx context.Context, req IncomingRequest) bool {
	v := newIncomingReqView(&req)
	return o.matchView(ctx, &v)
}

func (o *OrDetector) matchView(ctx context.Context, v *reqView) bool {
	for _, d := range o.detectors {
		if d == nil {
			continue
		}
		if fd, ok := d.(fastDetector); ok {
			if fd.matchView(ctx, v) {
				return true
			}
		} else if d.Match(ctx, v.toIncoming()) {
			return true
		}
	}
	return false
}
