package intel

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIPSetAndMultiFormatFeedParser(t *testing.T) {
	// 1. OpenAI / Anthropic / Google style JSON
	aiJSON := []byte(`{
		"creationTime": "2026-11-18T00:00:00Z",
		"prefixes": [
			{"ipv4Prefix": "20.15.240.64/28"},
			{"ipv6Prefix": "2603:1030::/32"}
		]
	}`)
	aiPrefs := ParseFeedPayload(aiJSON)
	aiSet := NewIPSet(aiPrefs)
	if !aiSet.ContainsString("20.15.240.70") || !aiSet.ContainsString("2603:1030::abcd") {
		t.Fatal("expected AI CIDR match")
	}
	if aiSet.ContainsString("20.15.240.90") || aiSet.ContainsString("1.1.1.1") {
		t.Fatal("unexpected AI CIDR false positive")
	}

	// 2. Spamhaus DROP JSONL
	dropJSONL := []byte("{\"cidr\":\"1.10.16.0/20\",\"sblid\":\"SBL256894\"}\n{\"type\":\"metadata\",\"records\":1}\n")
	dropSet := NewIPSet(ParseFeedPayload(dropJSONL))
	if !dropSet.ContainsString("1.10.20.5") || dropSet.ContainsString("1.11.0.1") {
		t.Fatal("Spamhaus DROP JSONL parsing mismatch")
	}

	// 3. IPsum + Spamhaus text comments
	textFeed := []byte("# IPsum feed\n45.33.32.156\t6\n185.220.101.0/24 ; SBL999\n")
	txtSet := NewIPSet(ParseFeedPayload(textFeed))
	if !txtSet.ContainsString("45.33.32.156") || !txtSet.ContainsString("185.220.101.42") {
		t.Fatal("plain-text feed parsing mismatch")
	}
}

func TestManagerAutoUpdateDiskCacheAndLiveReputation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var generation atomic.Int32
	generation.Store(1)

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "feeds.test" && req.URL.Path == "/ai.json":
			body := `{"prefixes":[{"ipv4Prefix":"20.15.240.64/28"}]}`
			if generation.Load() >= 2 {
				body = `{"prefixes":[{"ipv4Prefix":"20.15.240.64/28"},{"ipv4Prefix":"34.100.10.0/24"}]}`
			}
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"ETag": []string{`"v` + string(rune('0'+generation.Load())) + `"`}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil

		case req.URL.Host == "feeds.test" && req.URL.Path == "/threat.txt":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("91.240.118.172\t8\n185.220.101.0/24 ; tor\n")),
			}, nil

		case req.URL.Host == "internetdb.test":
			if strings.HasSuffix(req.URL.Path, "/8.8.4.4") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"ip":"8.8.4.4","tags":["dns"]}`))),
				}, nil
			}
			if strings.HasSuffix(req.URL.Path, "/45.155.205.233") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"ip":"45.155.205.233","tags":["scanner","compromised"]}`))),
				}, nil
			}
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil

		case req.URL.Host == "sfs.test":
			if strings.Contains(req.URL.RawQuery, "194.26.29.10") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"success":1,"ip":{"appears":1,"frequency":12,"confidence":98.5}}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"success":1,"ip":{"appears":0,"frequency":0,"confidence":0}}`)),
			}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	rep := NewReputationChecker(httpClient, time.Hour)
	rep.internetDBBaseURL = "https://internetdb.test"
	rep.stopForumSpamBaseURL = "https://sfs.test/api"

	cacheDir := t.TempDir()
	mgr := NewManager(ManagerOptions{
		CacheDir:              cacheDir,
		IPSecurityEnabled:     true,
		LiveReputationEnabled: true,
		AIIPListEnabled:       true,
		RefreshInterval:       40 * time.Millisecond,
		ReplaceThreatFeeds:    []FeedSource{{Name: "test-threat", URL: "https://feeds.test/threat.txt"}},
		ReplaceAIFeeds:        []FeedSource{{Name: "test-ai", URL: "https://feeds.test/ai.json"}},
		HTTPClient:            httpClient,
		Reputation:            rep,
	})

	// Initial refresh
	st := mgr.RefreshNow(ctx)
	if st.ThreatEntries != 2 || st.AIEntries != 1 {
		t.Fatalf("unexpected counts: %+v", st)
	}
	if !mgr.IsAIIP("20.15.240.66") {
		t.Fatal("expected 20.15.240.66 to match AI IP list")
	}
	if !mgr.IsThreatIP(ctx, "91.240.118.172") || !mgr.IsThreatIP(ctx, "185.220.101.9") {
		t.Fatal("expected feed threat IP match")
	}
	// Live Shodan InternetDB & StopForumSpam checks
	if !mgr.IsThreatIP(ctx, "45.155.205.233") {
		t.Fatal("expected InternetDB scanner IP to be flagged")
	}
	if !mgr.IsThreatIP(ctx, "194.26.29.10") {
		t.Fatal("expected StopForumSpam IP to be flagged")
	}
	if mgr.IsThreatIP(ctx, "8.8.4.4") {
		t.Fatal("expected 8.8.4.4 to be clean")
	}

	// Verify automatic background periodic update picks up generation 2 AI CIDR!
	generation.Store(2)
	mgr.Start(ctx)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if mgr.IsAIIP("34.100.10.55") {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !mgr.IsAIIP("34.100.10.55") {
		t.Fatal("expected background auto-updater to load new 34.100.10.0/24 AI range")
	}

	// Verify warm start from disk cache even when network is offline
	offlineClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("down"))}, nil
		}),
	}
	mgrWarm := NewManager(ManagerOptions{
		CacheDir:           cacheDir,
		IPSecurityEnabled:  true,
		AIIPListEnabled:    true,
		ReplaceThreatFeeds: []FeedSource{{Name: "test-threat", URL: "https://feeds.test/threat.txt"}},
		ReplaceAIFeeds:     []FeedSource{{Name: "test-ai", URL: "https://feeds.test/ai.json"}},
		HTTPClient:         offlineClient,
	})
	if !mgrWarm.IsAIIP("34.100.10.55") || !mgrWarm.IsThreatFeedIP("91.240.118.172") {
		t.Fatal("expected warm disk cache to populate IPSets immediately")
	}
}

func TestVPNDetectionFeedsLiveCheckersCacheAndFailOpen(t *testing.T) {
	ctx := context.Background()
	var liveHits atomic.Int32

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "feeds.test" && req.URL.Path == "/vpn.txt":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("# VPN CIDRs\n185.156.72.0/24\n2a07:7800::/32\n")),
			}, nil

		case req.URL.Host == "ipquery.test":
			liveHits.Add(1)
			if strings.HasSuffix(req.URL.Path, "/1.2.3.4") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"ip":"1.2.3.4","risk":{"is_vpn":true,"is_proxy":false,"is_tor":false}}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"ip":"other","risk":{"is_vpn":false,"is_proxy":false,"is_tor":false}}`)),
			}, nil

		case req.URL.Host == "ipapi.test":
			liveHits.Add(1)
			if strings.Contains(req.URL.RawQuery, "q=5.6.7.8") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"ip":"5.6.7.8","is_vpn":false,"is_proxy":true,"is_tor":false}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"is_vpn":false,"is_proxy":false,"is_tor":false}`)),
			}, nil
		}
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("error"))}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}
	vpnChk := NewVPNChecker(httpClient, time.Hour)
	vpnChk.ipQueryBaseURL = "https://ipquery.test"
	vpnChk.ipAPIIsBaseURL = "https://ipapi.test"

	cacheDir := t.TempDir()
	mgr := NewManager(ManagerOptions{
		CacheDir:              cacheDir,
		LiveReputationEnabled: true,
		VPNDetectionEnabled:   true,
		ReplaceVPNFeeds:       []FeedSource{{Name: "test-vpn", URL: "https://feeds.test/vpn.txt"}},
		HTTPClient:            httpClient,
		VPNChecker:            vpnChk,
	})
	st := mgr.RefreshNow(ctx)
	if !st.VPNDetectionEnabled || st.VPNEntries != 2 {
		t.Fatalf("unexpected VPN status: %+v", st)
	}
	if !mgr.IsVPNOrProxyIP(ctx, "185.156.72.42") || !mgr.IsVPNOrProxyIP(ctx, "2a07:7800::1") {
		t.Fatal("expected VPN feed match")
	}
	if !mgr.IsVPNOrProxyIP(ctx, "1.2.3.4") {
		t.Fatal("expected ipquery VPN IP match")
	}
	if !mgr.IsVPNOrProxyIP(ctx, "5.6.7.8") {
		t.Fatal("expected ipapi.is proxy IP match")
	}
	if mgr.IsVPNOrProxyIP(ctx, "8.8.8.8") {
		t.Fatal("expected 8.8.8.8 to not be flagged as VPN/proxy")
	}

	// Verify cache avoids duplicate live queries
	beforeHits := liveHits.Load()
	if !mgr.IsVPNOrProxyIP(ctx, "1.2.3.4") || mgr.IsVPNOrProxyIP(ctx, "8.8.8.8") {
		t.Fatal("cached VPN lookup mismatch")
	}
	if liveHits.Load() != beforeHits {
		t.Fatalf("expected cached lookups without extra HTTP hits, got %d -> %d", beforeHits, liveHits.Load())
	}

	// Verify warm disk cache restores VPN feed without network
	offlineClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("down"))}, nil
		}),
	}
	mgrWarm := NewManager(ManagerOptions{
		CacheDir:            cacheDir,
		VPNDetectionEnabled: true,
		ReplaceVPNFeeds:     []FeedSource{{Name: "test-vpn", URL: "https://feeds.test/vpn.txt"}},
		HTTPClient:          offlineClient,
	})
	if !mgrWarm.IsVPNFeedIP("185.156.72.42") {
		t.Fatal("expected warm VPN disk cache match")
	}
	// Fail-open when live VPN endpoints fail
	failVPN := NewVPNChecker(offlineClient, time.Hour)
	failVPN.ipQueryBaseURL = "https://ipquery.test"
	failVPN.ipAPIIsBaseURL = "https://ipapi.test"
	if flagged, _ := failVPN.Check(ctx, "9.9.9.9"); flagged {
		t.Fatal("expected fail-open false when upstream VPN APIs return 503")
	}
}
