package waf

import (
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

func TestRemoteRuleManagerFetchCacheETagAndFailSafe(t *testing.T) {
	cacheDir := t.TempDir()
	var mode atomic.Int32 // 0=valid v1, 1=304, 2=malformed, 3=valid v2
	var updates atomic.Int32

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch mode.Load() {
			case 0:
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"ETag": []string{`"v1"`}},
					Body:       io.NopCloser(strings.NewReader("block path /remote-secret*\n")),
				}, nil
			case 1:
				if req.Header.Get("If-None-Match") == `"v1"` {
					return &http.Response{
						StatusCode: http.StatusNotModified,
						Header:     http.Header{},
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("block path /remote-secret*\n")),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("this is not valid rule syntax")),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"ETag": []string{`"v2"`}},
					Body:       io.NopCloser(strings.NewReader(`{"rules":[{"name":"block-v2","action":"block","paths":["/v2-blocked"]}]}`)),
				}, nil
			}
		}),
	}

	url := "https://rules.example.com/waf.rules"
	mgr := NewRemoteRuleManager(RemoteRuleManagerOptions{
		CacheDir:        cacheDir,
		RefreshInterval: time.Minute,
		URLs:            []string{url},
		HTTPClient:      client,
		OnUpdate:        func() { updates.Add(1) },
	})

	// 1. Initial fetch loads v1 rules and fires OnUpdate
	st := mgr.RefreshNow(context.Background())
	if st.TotalRules != 1 || len(st.Sources) != 1 || st.Sources[0].LastError != "" {
		t.Fatalf("unexpected initial status: %+v", st)
	}
	if updates.Load() != 1 {
		t.Fatalf("updates = %d, want 1", updates.Load())
	}
	rules := mgr.RulesForURLs([]string{url})
	if len(rules) != 1 || rules[0].Paths[0] != "/remote-secret*" {
		t.Fatalf("unexpected v1 rules: %+v", rules)
	}

	// 2. 304 Not Modified keeps rules and does NOT trigger redundant reload
	mode.Store(1)
	mgr.RefreshNow(context.Background())
	if updates.Load() != 1 {
		t.Fatalf("updates after 304 = %d, want 1", updates.Load())
	}

	// 3. Malformed response keeps last known good rules and records LastError
	mode.Store(2)
	stErr := mgr.RefreshNow(context.Background())
	if stErr.TotalRules != 1 || stErr.Sources[0].LastError == "" {
		t.Fatalf("expected fail-safe retention with LastError, got %+v", stErr)
	}
	if got := mgr.RulesForURLs([]string{url}); len(got) != 1 || got[0].Paths[0] != "/remote-secret*" {
		t.Fatalf("expected v1 rules retained on malformed payload, got %+v", got)
	}

	// 4. Valid JSON v2 updates rules and fires OnUpdate
	mode.Store(3)
	st2 := mgr.RefreshNow(context.Background())
	if st2.TotalRules != 1 || st2.Sources[0].LastError != "" || updates.Load() != 2 {
		t.Fatalf("expected v2 update, got status=%+v updates=%d", st2, updates.Load())
	}
	if got := mgr.RulesForURLs([]string{url}); len(got) != 1 || got[0].Name != "block-v2" {
		t.Fatalf("unexpected v2 rules: %+v", got)
	}

	// 5. Cold start warm-loads cached v2 rules from disk without network
	mgr2 := NewRemoteRuleManager(RemoteRuleManagerOptions{
		CacheDir: cacheDir,
		URLs:     []string{url},
	})
	warmRules := mgr2.RulesForURLs([]string{url})
	if len(warmRules) != 1 || warmRules[0].Name != "block-v2" {
		t.Fatalf("expected warm-loaded v2 rules from disk cache, got %+v", warmRules)
	}
	if warmSt := mgr2.Status(); len(warmSt.Sources) != 1 || !warmSt.Sources[0].FromCache {
		t.Fatalf("expected FromCache=true on warm load, got %+v", warmSt)
	}
}
