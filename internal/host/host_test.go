package host

import "testing"

func TestNormalizeAndFromHeader(t *testing.T) {
	if got := Normalize(" Example.COM. "); got != "example.com" {
		t.Fatalf("Normalize = %q, want example.com", got)
	}
	if got := FromHeader("App.Example.COM:8443"); got != "app.example.com" {
		t.Fatalf("FromHeader = %q, want app.example.com", got)
	}
	if got := FromHeader("[::1]:8443"); got != "::1" {
		t.Fatalf("FromHeader IPv6 = %q, want ::1", got)
	}
	if got := FromHeader("[2001:db8::1]"); got != "2001:db8::1" {
		t.Fatalf("FromHeader bare IPv6 = %q, want 2001:db8::1", got)
	}
}

func TestIsPublic(t *testing.T) {
	publicCases := []string{"example.com", "sub.domain.org"}
	for _, tc := range publicCases {
		if !IsPublic(tc) {
			t.Errorf("IsPublic(%q) = false, want true", tc)
		}
	}
	privateCases := []string{"", "localhost", "api.localhost", "printer.local", "*.example.com", "127.0.0.1", "::1"}
	for _, tc := range privateCases {
		if IsPublic(tc) {
			t.Errorf("IsPublic(%q) = true, want false", tc)
		}
	}
}

func TestCovers(t *testing.T) {
	if !Covers("*.example.com", "www.example.com") {
		t.Fatal("expected wildcard to cover one label")
	}
	if Covers("*.example.com", "example.com") {
		t.Fatal("expected wildcard not to cover apex")
	}
	if Covers("*.example.com", "a.b.example.com") {
		t.Fatal("expected wildcard not to cover multiple labels")
	}
	if !Covers("Example.COM.", "example.com") {
		t.Fatal("expected exact match after normalization")
	}
}

func TestParseSiteAddress(t *testing.T) {
	h, port, ok, err := ParseSiteAddress("https://App.Example.com:8443,")
	if err != nil || !ok || h != "app.example.com" || port != 8443 {
		t.Fatalf("got (%q, %d, %v, %v)", h, port, ok, err)
	}
	h, port, ok, err = ParseSiteAddress("[::1]:9443")
	if err != nil || !ok || h != "::1" || port != 9443 {
		t.Fatalf("got (%q, %d, %v, %v)", h, port, ok, err)
	}
}
