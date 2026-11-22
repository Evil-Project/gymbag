package waf

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/store"
)

func TestProofAndTokens(t *testing.T) {
	digest := ProofHex("gymbag", 0)
	if len(digest) != 64 {
		t.Fatalf("len(digest) = %d, want 64", len(digest))
	}
	var nonce int64
	for !Accepts("gymbag", nonce, 1) {
		nonce++
	}
	if !strings.HasPrefix(ProofHex("gymbag", nonce), "0") {
		t.Fatal("expected leading zero")
	}
	if Accepts("gymbag", -1, 1) || Accepts("gymbag", nonce, 0) {
		t.Fatal("expected invalid difficulty/nonce rejection")
	}

	tokens := NewClearanceTokens([]byte("secret"), 60)
	tok := tokens.Issue("app.example.com", "203.0.113.5", 60)
	if !tokens.Valid(tok, "app.example.com", "203.0.113.5") {
		t.Fatal("expected valid token for matching host & IP")
	}
	if tokens.Valid(tok, "203.0.113.6", "203.0.113.6") || tokens.Valid(tok, "other.example.com", "203.0.113.5") {
		t.Fatal("expected token rejection on different IP or different Host")
	}
	if tokens.Valid("not-a-token", "app.example.com", "203.0.113.5") || tokens.Valid("", "app.example.com", "203.0.113.5") {
		t.Fatal("expected invalid token rejection")
	}
}

func TestChallengeServiceSingleUseAndAttemptLimit(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	svc := NewChallengeService(st, 1, 60)

	chalID, diff, err := svc.Issue(ctx, "app.example.com", "203.0.113.9", 1, 60)
	if err != nil {
		t.Fatal(err)
	}
	var goodNonce int64
	for !Accepts(chalID, goodNonce, diff) {
		goodNonce++
	}
	sub := Submission{ChallengeID: chalID, Nonce: goodNonce, Verified: true}

	// Wrong IP or wrong Host must fail without burning valid attempt for owner
	if ok, _ := svc.Verify(ctx, "app.example.com", "198.51.100.1", sub); ok {
		t.Fatal("expected wrong IP to fail")
	}
	if ok, _ := svc.Verify(ctx, "other.example.com", "203.0.113.9", sub); ok {
		t.Fatal("expected wrong host to fail")
	}
	// Right IP + Host succeeds once, then replay fails
	if ok, _ := svc.Verify(ctx, "app.example.com", "203.0.113.9", sub); !ok {
		t.Fatal("expected valid proof to pass")
	}
	if ok, _ := svc.Verify(ctx, "app.example.com", "203.0.113.9", sub); ok {
		t.Fatal("expected replay to fail")
	}
}

func TestAllFourTriggerModesAndDetectors(t *testing.T) {
	ctx := context.Background()

	humanReq := IncomingRequest{
		Method:   "GET",
		Host:     "site.example.com",
		Path:     "/",
		ClientIP: "203.0.113.10",
		Headers: map[string]string{
			"user-agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"accept-language": "en-US,en;q=0.9",
			"sec-fetch-mode":  "navigate",
		},
	}

	suspiciousCurlReq := IncomingRequest{
		Method:   "GET",
		Host:     "site.example.com",
		Path:     "/",
		ClientIP: "203.0.113.11",
		Headers: map[string]string{
			"user-agent": "curl/8.7.1",
			"accept":     "*/*",
		},
	}

	spoofedBrowserReq := IncomingRequest{
		Method:   "GET",
		Host:     "site.example.com",
		Path:     "/",
		ClientIP: "203.0.113.12",
		Headers: map[string]string{
			"user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0 Safari/537.36",
		},
	}

	aiBotReq := IncomingRequest{
		Method:   "GET",
		Host:     "site.example.com",
		Path:     "/articles/1",
		ClientIP: "203.0.113.13",
		Headers: map[string]string{
			"user-agent": "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
			"accept":     "*/*",
		},
	}

	buildForLevel := func(level config.VerifyLevel, protect bool) *Engine {
		st := store.NewMemoryStore()
		tokens := NewClearanceTokens([]byte("secret"), 60)
		wafCfg := config.DefaultSiteWAFConfig()
		wafCfg.VerifyLevel = level
		site := config.SiteConfig{
			Domain:    "site.example.com",
			Upstreams: []string{"http://localhost:3000"},
			Protect:   protect,
			WAF:       wafCfg,
		}
		return BuildSiteEngine(site, st, tokens)
	}

	// Level 0: always -> challenges human, curl, spoofed, and AI
	alwaysEng := buildForLevel(config.VerifyLevelAlways, true)
	for name, r := range map[string]IncomingRequest{
		"human": humanReq, "curl": suspiciousCurlReq, "spoofed": spoofedBrowserReq, "ai": aiBotReq,
	} {
		if dec := alwaysEng.Decide(ctx, r); dec.Action != ActionChallenge {
			t.Fatalf("level 0 (always): %s got %s, want challenge", name, dec.Action)
		}
	}

	// Level 1: only suspicious -> allows human & AI, challenges curl & spoofed
	suspEng := buildForLevel(config.VerifyLevelOnlySuspicious, true)
	if dec := suspEng.Decide(ctx, humanReq); dec.Action != ActionAllow {
		t.Fatalf("level 1 (only suspicious): human got %s, want allow", dec.Action)
	}
	if dec := suspEng.Decide(ctx, aiBotReq); dec.Action != ActionAllow {
		t.Fatalf("level 1 (only suspicious): aiBot got %s, want allow", dec.Action)
	}
	if dec := suspEng.Decide(ctx, suspiciousCurlReq); dec.Action != ActionChallenge {
		t.Fatalf("level 1 (only suspicious): curl got %s, want challenge", dec.Action)
	}
	if dec := suspEng.Decide(ctx, spoofedBrowserReq); dec.Action != ActionChallenge {
		t.Fatalf("level 1 (only suspicious): spoofed got %s, want challenge", dec.Action)
	}

	// Level 2: only ai -> allows human & curl, challenges AI
	aiEng := buildForLevel(config.VerifyLevelOnlyAI, true)
	if dec := aiEng.Decide(ctx, humanReq); dec.Action != ActionAllow {
		t.Fatalf("level 2 (only ai): human got %s, want allow", dec.Action)
	}
	if dec := aiEng.Decide(ctx, suspiciousCurlReq); dec.Action != ActionAllow {
		t.Fatalf("level 2 (only ai): curl got %s, want allow", dec.Action)
	}
	if dec := aiEng.Decide(ctx, aiBotReq); dec.Action != ActionChallenge {
		t.Fatalf("level 2 (only ai): aiBot got %s, want challenge", dec.Action)
	}

	// Level 3: suspicious & ai -> allows human, challenges curl, spoofed, AND AI
	bothEng := buildForLevel(config.VerifyLevelSuspiciousAndAI, true)
	if dec := bothEng.Decide(ctx, humanReq); dec.Action != ActionAllow {
		t.Fatalf("level 3 (suspicious & ai): human got %s, want allow", dec.Action)
	}
	if dec := bothEng.Decide(ctx, suspiciousCurlReq); dec.Action != ActionChallenge {
		t.Fatalf("level 3 (suspicious & ai): curl got %s, want challenge", dec.Action)
	}
	if dec := bothEng.Decide(ctx, aiBotReq); dec.Action != ActionChallenge {
		t.Fatalf("level 3 (suspicious & ai): aiBot got %s, want challenge", dec.Action)
	}

	// Unprotected website (protect off) -> allows all normal/bot requests, still blocks traversal & TRACE
	openEng := buildForLevel(config.VerifyLevelAlways, false)
	if dec := openEng.Decide(ctx, aiBotReq); dec.Action != ActionAllow {
		t.Fatalf("unprotected: aiBot got %s, want allow", dec.Action)
	}
	if dec := openEng.Decide(ctx, IncomingRequest{Method: "TRACE", Path: "/"}); dec.Action != ActionDeny {
		t.Fatalf("unprotected: TRACE got %s, want deny", dec.Action)
	}
	if dec := openEng.Decide(ctx, IncomingRequest{Method: "GET", Path: "/../secret", RawTarget: "/../secret"}); dec.Action != ActionDeny {
		t.Fatalf("unprotected: traversal got %s, want deny", dec.Action)
	}
}

func TestPageRendererAndBrowserSolverParity(t *testing.T) {
	renderer := NewRenderer("")
	html := renderer.RenderChallengeHTML("challenge-1", 1)
	for _, want := range []string{PensiveAsset, HappyAsset, RejectAsset} {
		if !strings.Contains(html, want) {
			t.Fatalf("challenge HTML missing %s", want)
		}
	}
	specMatch := regexp.MustCompile(`<script type="application/json" id="gymbag-challenge">(.*?)</script>`).FindStringSubmatch(html)
	if len(specMatch) != 2 {
		t.Fatal("missing gymbag-challenge JSON tag")
	}
	var spec map[string]any
	if err := json.Unmarshal([]byte(specMatch[1]), &spec); err != nil {
		t.Fatal(err)
	}
	if spec["challenge_id"] != "challenge-1" {
		t.Fatalf("challenge_id = %v", spec["challenge_id"])
	}

	if _, err := exec.LookPath("node"); err == nil {
		scriptMatch := regexp.MustCompile(`(?s)<script>\n(.*)</script>`).FindStringSubmatch(html)
		if len(scriptMatch) != 2 {
			t.Fatal("missing inline solver script")
		}
		tmpJS := filepath.Join(t.TempDir(), "page.js")
		if err := os.WriteFile(tmpJS, []byte(scriptMatch[1]), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("node", "--check", tmpJS).CombinedOutput(); err != nil {
			t.Fatalf("node --check failed: %v\n%s", err, out)
		}

		// Verify JS sha256 matches Go ProofHex
		solverRaw, _ := embeddedAssets.ReadFile("assets/solver.js")
		solverPath := filepath.Join(t.TempDir(), "solver.js")
		_ = os.WriteFile(solverPath, solverRaw, 0o600)
		checkScript := `
const fs = require("node:fs");
const src = fs.readFileSync(process.argv[1], "utf8");
const { sha256 } = new Function(src + "; return { sha256 };")();
process.stdout.write(sha256("Gymbag:42"));
`
		out, err := exec.Command("node", "-e", checkScript, solverPath).CombinedOutput()
		if err != nil {
			t.Fatalf("node sha256 run failed: %v\n%s", err, out)
		}
		if string(out) != ProofHex("Gymbag", 42) {
			t.Fatalf("JS sha256 %q != Go ProofHex %q", string(out), ProofHex("Gymbag", 42))
		}
	}
}

type stubIntel struct {
	threatIPs map[string]bool
	aiIPs     map[string]bool
	vpnIPs    map[string]bool
}

func (s *stubIntel) IsThreatIP(_ context.Context, ip string) bool     { return s.threatIPs[ip] }
func (s *stubIntel) IsAIIP(ip string) bool                            { return s.aiIPs[ip] }
func (s *stubIntel) IsVPNOrProxyIP(_ context.Context, ip string) bool { return s.vpnIPs[ip] }

func TestCustomUserRulesAndIntelIntegration(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	tokens := NewClearanceTokens([]byte("secret"), 60)
	intelStub := &stubIntel{
		threatIPs: map[string]bool{"185.220.101.5": true},
		aiIPs:     map[string]bool{"20.15.240.66": true}, // OpenAI CIDR IP
	}

	wafCfg := config.DefaultSiteWAFConfig()
	wafCfg.VerifyLevel = config.VerifyLevelSuspiciousAndAI // level 3
	wafCfg.Rules = []config.CustomRule{
		{Action: config.RuleActionAllow, IPs: []string{"10.10.0.0/16"}},
		{Action: config.RuleActionVerify, IPs: []string{"198.51.100.50"}},
		{Action: config.RuleActionBlock, IPs: []string{"198.51.100.0/24"}},
		{Action: config.RuleActionBlock, UserAgents: []string{"EvilScraper", "~^Scrapy/"}},
		{Action: config.RuleActionVerify, Paths: []string{"/api/export*"}, Methods: []string{"POST"}},
		{Action: config.RuleActionBlock, Headers: []config.KeyValueMatch{{Name: "Referer", Pattern: "*spam.example*"}}},
		{Action: config.RuleActionAllow, Cookies: []config.KeyValueMatch{{Name: "vip_pass", Pattern: "gold"}}},
	}

	site := config.SiteConfig{
		Domain:    "app.example.com",
		Upstreams: []string{"http://localhost:3000"},
		Protect:   true,
		WAF:       wafCfg,
	}
	eng := BuildSiteEngineWithIntel(site, st, tokens, intelStub, "block")

	browserHeaders := func() map[string]string {
		return map[string]string{
			"user-agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/131.0.0.0 Safari/537.36",
			"accept":          "text/html,*/*",
			"accept-language": "en-US,en;q=0.9",
			"sec-fetch-mode":  "navigate",
		}
	}

	// 1. Whitelisted subnet (10.10.0.0/16) bypasses even with curl UA
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/", ClientIP: "10.10.1.5",
		Headers: map[string]string{"user-agent": "curl/8.7.1"},
	}); dec.Action != ActionAllow {
		t.Fatalf("whitelisted subnet got %s, want allow", dec.Action)
	}

	// 2. First-match ordering: 198.51.100.50 hits 'verify ip' before 'block ip 198.51.100.0/24',
	//    then passes once cleared! While 198.51.100.99 hits 'block ip 198.51.100.0/24'.
	verifyIPReq := IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/", ClientIP: "198.51.100.50",
		Headers: browserHeaders(),
	}
	if dec := eng.Decide(ctx, verifyIPReq); dec.Action != ActionChallenge {
		t.Fatalf("198.51.100.50 uncleared got %s, want challenge", dec.Action)
	}
	verifyIPReq.Cookies = map[string]string{"waf_clearance": tokens.Issue("app.example.com", "198.51.100.50", 60)}
	if dec := eng.Decide(ctx, verifyIPReq); dec.Action != ActionAllow {
		t.Fatalf("198.51.100.50 cleared got %s, want allow", dec.Action)
	}
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/", ClientIP: "198.51.100.99",
		Headers: browserHeaders(),
	}); dec.Action != ActionDeny {
		t.Fatalf("198.51.100.99 in blocked subnet got %s, want deny", dec.Action)
	}

	// 3. Custom User-Agent & Header block rules
	h := browserHeaders()
	h["user-agent"] = "Scrapy/2.11.0"
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/", ClientIP: "203.0.113.1", Headers: h,
	}); dec.Action != ActionDeny {
		t.Fatalf("Scrapy UA got %s, want deny", dec.Action)
	}

	h2 := browserHeaders()
	h2["referer"] = "https://bad.spam.example/link"
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/", ClientIP: "203.0.113.1", Headers: h2,
	}); dec.Action != ActionDeny {
		t.Fatalf("spam Referer got %s, want deny", dec.Action)
	}

	// 4. Compound custom rule (POST + /api/export*) challenges normal browser, while GET /api/export/csv passes
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "POST", Host: "app.example.com", Path: "/api/export/csv", ClientIP: "203.0.113.2",
		Headers: browserHeaders(),
	}); dec.Action != ActionChallenge {
		t.Fatalf("POST /api/export/csv got %s, want challenge", dec.Action)
	}
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/api/export/csv", ClientIP: "203.0.113.2",
		Headers: browserHeaders(),
	}); dec.Action != ActionAllow {
		t.Fatalf("GET /api/export/csv got %s, want allow", dec.Action)
	}

	// 5. Threat Intel IP (185.220.101.5) blocked when ip_security_action == "block"
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/", ClientIP: "185.220.101.5",
		Headers: browserHeaders(),
	}); dec.Action != ActionDeny || dec.Reason != "threat" {
		t.Fatalf("threat IP got %+v, want deny(threat)", dec)
	}

	// 6. AI Robot IP (20.15.240.66) spoofing full Chrome browser headers is STILL caught as AI!
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.example.com", Path: "/docs", ClientIP: "20.15.240.66",
		Headers: browserHeaders(),
	}); dec.Action != ActionChallenge {
		t.Fatalf("spoofed AI IP 20.15.240.66 got %s, want challenge", dec.Action)
	}
}

func TestOpenBoxDefaultSecurityRulesAndOverride(t *testing.T) {
	ctx := context.Background()

	// Validate all built-in preset rules pass CustomRule.Validate()
	blockOnly := DefaultSecurityRules(false)
	fullPreset := DefaultSecurityRules(true)
	if len(blockOnly) != 4 || len(fullPreset) != 6 {
		t.Fatalf("unexpected DefaultSecurityRules lengths: blockOnly=%d full=%d", len(blockOnly), len(fullPreset))
	}
	for i := range fullPreset {
		r := fullPreset[i]
		if err := r.Validate(); err != nil {
			t.Fatalf("DefaultSecurityRules[%d] (%s) failed Validate(): %v", i, r.Name, err)
		}
	}

	// 1. Open-box bare 'verify' config (zero custom rules)
	openBoxCfg, err := config.ParseCaddyfile(`
		app.internal {
			reverse_proxy :3000
			verify
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile bare verify failed: %v", err)
	}
	st := store.NewMemoryStore()
	tokens := NewClearanceTokens([]byte("secret"), 60)
	eng := BuildSiteEngine(openBoxCfg.Sites[0], st, tokens)

	browserHeaders := func() map[string]string {
		return map[string]string{
			"user-agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/131.0.0.0 Safari/537.36",
			"accept":          "text/html,*/*",
			"accept-language": "en-US,en;q=0.9",
			"sec-fetch-mode":  "navigate",
		}
	}
	clearedCookies := map[string]string{
		"waf_clearance": tokens.Issue("app.internal", "203.0.113.10", 60),
	}

	// Normal browser allowed
	if dec := eng.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.internal", Path: "/home", ClientIP: "203.0.113.10", Headers: browserHeaders(),
	}); dec.Action != ActionAllow {
		t.Fatalf("openbox browser got %s, want allow", dec.Action)
	}

	// Sensitive files, exploit probes, vuln scanners, and SQLi/XSS queries MUST be blocked (ActionDeny),
	// even if the client already holds a valid PoW clearance cookie!
	blockedCases := []struct {
		name string
		req  IncomingRequest
	}{
		{
			name: "/.env",
			req:  IncomingRequest{Method: "GET", Host: "app.internal", Path: "/.env", ClientIP: "203.0.113.10", Headers: browserHeaders(), Cookies: clearedCookies},
		},
		{
			name: "/.git/config",
			req:  IncomingRequest{Method: "GET", Host: "app.internal", Path: "/.git/config", ClientIP: "203.0.113.10", Headers: browserHeaders(), Cookies: clearedCookies},
		},
		{
			name: "/backup.sql",
			req:  IncomingRequest{Method: "GET", Host: "app.internal", Path: "/backup.sql", ClientIP: "203.0.113.10", Headers: browserHeaders(), Cookies: clearedCookies},
		},
		{
			name: "/wp-login.php",
			req:  IncomingRequest{Method: "GET", Host: "app.internal", Path: "/wp-login.php", ClientIP: "203.0.113.10", Headers: browserHeaders(), Cookies: clearedCookies},
		},
		{
			name: "sqlmap scanner",
			req:  IncomingRequest{Method: "GET", Host: "app.internal", Path: "/", ClientIP: "203.0.113.10", Headers: map[string]string{"user-agent": "sqlmap/1.8#stable"}, Cookies: clearedCookies},
		},
		{
			name: "SQLi UNION SELECT query",
			req:  IncomingRequest{Method: "GET", Host: "app.internal", Path: "/items", Query: "id=1+UNION+SELECT+username,password+FROM+users", ClientIP: "203.0.113.10", Headers: browserHeaders(), Cookies: clearedCookies},
		},
	}
	for _, bc := range blockedCases {
		if dec := eng.Decide(ctx, bc.req); dec.Action != ActionDeny {
			t.Fatalf("openbox %s got %s, want deny", bc.name, dec.Action)
		}
	}

	// Default verify rules challenge curl & missing User-Agent, and allow once cleared
	curlReq := IncomingRequest{
		Method: "GET", Host: "app.internal", Path: "/api/data", ClientIP: "203.0.113.10",
		Headers: map[string]string{"user-agent": "curl/8.7.1"},
	}
	if dec := eng.Decide(ctx, curlReq); dec.Action != ActionChallenge {
		t.Fatalf("openbox curl uncleared got %s, want challenge", dec.Action)
	}
	curlReq.Cookies = clearedCookies
	if dec := eng.Decide(ctx, curlReq); dec.Action != ActionAllow {
		t.Fatalf("openbox curl cleared got %s, want allow", dec.Action)
	}

	missingUAReq := IncomingRequest{
		Method: "GET", Host: "app.internal", Path: "/api/data", ClientIP: "203.0.113.10",
		Headers: map[string]string{},
	}
	if dec := eng.Decide(ctx, missingUAReq); dec.Action != ActionChallenge {
		t.Fatalf("openbox missing UA got %s, want challenge", dec.Action)
	}

	// 2. User custom rules run BEFORE baseline default block rules so user 'allow' takes priority,
	//    while other default block rules remain active!
	customPlusDefaultCfg, err := config.ParseCaddyfile(`
		app.internal {
			reverse_proxy :3000
			verify
			allow path /wp-login.php
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile custom + default failed: %v", err)
	}
	engCustom := BuildSiteEngine(customPlusDefaultCfg.Sites[0], st, tokens)
	if dec := engCustom.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.internal", Path: "/wp-login.php", ClientIP: "203.0.113.10", Headers: browserHeaders(),
	}); dec.Action != ActionAllow {
		t.Fatalf("explicitly allowed /wp-login.php got %s, want allow", dec.Action)
	}
	if dec := engCustom.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.internal", Path: "/.env", ClientIP: "203.0.113.10", Headers: browserHeaders(),
	}); dec.Action != ActionDeny {
		t.Fatalf("/.env should still be blocked by baseline default rules, got %s", dec.Action)
	}

	// 3. 'default_rules off' disables built-in default block rules
	disabledCfg, err := config.ParseCaddyfile(`
		app.internal {
			reverse_proxy :3000
			verify 2
			default_rules off
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile default_rules off failed: %v", err)
	}
	engDisabled := BuildSiteEngine(disabledCfg.Sites[0], st, tokens)
	for _, p := range []string{"/.env", "/wp-login.php"} {
		if dec := engDisabled.Decide(ctx, IncomingRequest{
			Method: "GET", Host: "app.internal", Path: p, ClientIP: "203.0.113.10", Headers: browserHeaders(),
		}); dec.Action != ActionAllow {
			t.Fatalf("default_rules off: %s got %s, want allow", p, dec.Action)
		}
	}
	if dec := engDisabled.Decide(ctx, IncomingRequest{
		Method: "GET", Host: "app.internal", Path: "/", Query: "id=1+UNION+SELECT+1", ClientIP: "203.0.113.10",
		Headers: map[string]string{"user-agent": "sqlmap/1.8"},
	}); dec.Action != ActionAllow {
		t.Fatalf("default_rules off: sqlmap/SQLi got %s, want allow on verify 2", dec.Action)
	}
}

func TestVPNProxyIPSuspicionClassification(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	tokens := NewClearanceTokens([]byte("secret"), 60)
	intelStub := &stubIntel{
		vpnIPs: map[string]bool{"198.51.100.77": true},
	}
	normalReq := IncomingRequest{
		Method:   "GET",
		Host:     "app.example.com",
		Path:     "/",
		ClientIP: "198.51.100.77",
		Headers: map[string]string{
			"user-agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36",
			"accept":          "text/html,*/*",
			"accept-language": "en-US,en;q=0.9",
			"sec-fetch-mode":  "navigate",
		},
	}

	makeEng := func(lvl config.VerifyLevel) *Engine {
		w := config.DefaultSiteWAFConfig()
		w.VerifyLevel = lvl
		site := config.SiteConfig{
			Domain:    "app.example.com",
			Upstreams: []string{"http://localhost:3000"},
			Protect:   true,
			WAF:       w,
		}
		return BuildSiteEngineWithIntel(site, st, tokens, intelStub, "block")
	}

	// Verify 1 (only suspicious) & 3 (suspicious & ai) challenge VPN/proxy IP (never hard-block even with ip_security_action=block)
	for _, lvl := range []config.VerifyLevel{config.VerifyLevelOnlySuspicious, config.VerifyLevelSuspiciousAndAI} {
		if dec := makeEng(lvl).Decide(ctx, normalReq); dec.Action != ActionChallenge {
			t.Fatalf("verify %d with VPN IP got %s, want challenge", lvl, dec.Action)
		}
	}
	// Verify 2 (only ai) allows normal browser on VPN/proxy IP
	if dec := makeEng(config.VerifyLevelOnlyAI).Decide(ctx, normalReq); dec.Action != ActionAllow {
		t.Fatalf("verify 2 with VPN IP got %s, want allow", dec.Action)
	}
}
