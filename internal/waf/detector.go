package waf

import (
	"context"
	"net/url"
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
	signatures []string
	intel      IntelProvider
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
	return &AIDetector{signatures: sigs, intel: intel}
}

// Match reports whether req originates from a known AI crawler IP block or LLM agent signature.
func (d *AIDetector) Match(_ context.Context, req IncomingRequest) bool {
	if d.intel != nil && d.intel.IsAIIP(req.ClientIP) {
		return true
	}
	if req.Headers["x-ai-agent"] != "" || req.Headers["sec-agent"] != "" {
		return true
	}
	ua := strings.ToLower(strings.TrimSpace(req.Headers["user-agent"]))
	if ua == "" {
		return false
	}
	for _, sig := range d.signatures {
		if strings.Contains(ua, sig) {
			return true
		}
	}
	return false
}

// SuspicionDetector identifies malicious/threat IPs, automated scrapers, headless browsers,
// header spoofing, and reconnaissance/exploit probe traffic.
type SuspicionDetector struct {
	uaSignatures []string
	pathPrefixes []string
	aiDetector   Detector
	intel        IntelProvider
}

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
		uaSignatures: uas,
		pathPrefixes: paths,
		aiDetector:   aiDetector,
		intel:        intel,
	}
}

// Match reports whether req exhibits threat-intel, bot, scraper, spoofing, or exploit-probing signals.
func (d *SuspicionDetector) Match(ctx context.Context, req IncomingRequest) bool {
	// 1. Exploit / reconnaissance path & query probes always trigger suspicion.
	lowerPath := strings.ToLower(req.Path)
	for _, prefix := range d.pathPrefixes {
		if strings.HasPrefix(lowerPath, prefix) {
			return true
		}
	}
	if req.Query != "" {
		decodedQuery, err := url.QueryUnescape(req.Query)
		if err != nil {
			decodedQuery = req.Query
		}
		lowerQuery := strings.ToLower(decodedQuery)
		for _, tok := range defaultSuspiciousQueryTokens {
			if strings.Contains(lowerQuery, tok) {
				return true
			}
		}
	}

	// 2. IP security threat-feed & live reputation check (Spamhaus DROP, IPsum, Feodo, InternetDB, StopForumSpam).
	if d.intel != nil && d.intel.IsThreatIP(ctx, req.ClientIP) {
		return true
	}

	ua := strings.ToLower(strings.TrimSpace(req.Headers["user-agent"]))
	// 3. Missing or implausibly short User-Agent.
	if len(ua) < 8 {
		return true
	}

	// If recognized as a known AI crawler (without exploit probes or threat IP above), leave classification
	// to AIDetector so verify level 1 (only suspicious) and level 2 (only ai) stay cleanly orthogonal.
	if d.aiDetector != nil && d.aiDetector.Match(ctx, req) {
		return false
	}

	// 3. Known CLI tools, HTTP client libraries, scrapers, headless browsers, and vulnerability scanners.
	if slices.Contains(defaultExactSuspiciousUAs, ua) {
		return true
	}
	for _, sig := range d.uaSignatures {
		if strings.Contains(ua, sig) {
			return true
		}
	}

	// 4. Interactive browser impersonation without standard browser request headers.
	claimsInteractiveBrowser := strings.Contains(ua, "mozilla/5.0") &&
		!strings.Contains(ua, "compatible;") &&
		(strings.Contains(ua, "chrome/") ||
			strings.Contains(ua, "firefox/") ||
			strings.Contains(ua, "safari/") ||
			strings.Contains(ua, "edg/"))

	if claimsInteractiveBrowser {
		if strings.TrimSpace(req.Headers["accept"]) == "" {
			return true
		}
		if strings.TrimSpace(req.Headers["accept-language"]) == "" &&
			strings.TrimSpace(req.Headers["sec-fetch-mode"]) == "" {
			return true
		}
	}

	return false
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
	for _, d := range o.detectors {
		if d != nil && d.Match(ctx, req) {
			return true
		}
	}
	return false
}
