package intel

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strings"
)

// ParseFeedPayload extracts IPv4 and IPv6 addresses/CIDR prefixes from any supported feed format:
//  1. Standardized crawler JSON (OpenAI, Anthropic, Perplexity, Google, Applebot: {"prefixes":[{"ipv4Prefix":...},{"ipv6Prefix":...}]})
//  2. JSON arrays or objects with "cidrs" / "ips" / "prefixes" string lists
//  3. JSON Lines (Spamhaus DROP v4/v6 JSONL: {"cidr":"1.10.16.0/20",...})
//  4. Plain-text blocklists (IPsum "1.2.3.4\t5", Spamhaus "1.10.16.0/20 ; SBL...", Feodo, FireHOL, Tor exit lists)
func ParseFeedPayload(data []byte) []netip.Prefix {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}

	// 1. Try single JSON document first if it starts with '{' or '['
	if trimmed[0] == '{' || trimmed[0] == '[' {
		if prefixes, ok := tryParseJSONDocument(trimmed); ok && len(prefixes) > 0 {
			return prefixes
		}
	}

	// 2. Line-by-line parser supporting both JSONL (Spamhaus) and plain-text feeds (IPsum, DROP, Feodo)
	var out []netip.Prefix
	for _, rawLine := range strings.Split(string(trimmed), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err == nil {
				for _, key := range []string{"cidr", "ip", "ipv4Prefix", "ipv6Prefix", "ip_prefix", "prefix"} {
					if s, ok := obj[key].(string); ok {
						if p, pErr := ParseIPOrCIDR(s); pErr == nil {
							out = append(out, p)
						}
					}
				}
			}
			continue
		}

		// Strip inline comments (';' or '#')
		if idx := strings.IndexAny(line, ";#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		// Take first whitespace/tab/comma-delimited token (handles IPsum "ip\tscore" and CSV lists)
		fields := strings.FieldsFunc(line, func(r rune) bool {
			return r == ' ' || r == '\t' || r == ',' || r == '\r'
		})
		if len(fields) == 0 {
			continue
		}
		if p, err := ParseIPOrCIDR(fields[0]); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func tryParseJSONDocument(raw []byte) ([]netip.Prefix, bool) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, false
	}
	var out []netip.Prefix
	extractPrefixesRecursive(root, &out)
	return out, true
}

func extractPrefixesRecursive(node any, out *[]netip.Prefix) {
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			extractPrefixesRecursive(item, out)
		}
	case string:
		if p, err := ParseIPOrCIDR(v); err == nil {
			*out = append(*out, p)
		}
	case map[string]any:
		for _, key := range []string{"ipv4Prefix", "ipv6Prefix", "ip_prefix", "cidr", "ip", "prefix"} {
			if s, ok := v[key].(string); ok {
				if p, err := ParseIPOrCIDR(s); err == nil {
					*out = append(*out, p)
				}
			}
		}
		for _, arrKey := range []string{"prefixes", "cidrs", "ips", "ranges", "items", "data"} {
			if child, ok := v[arrKey]; ok {
				extractPrefixesRecursive(child, out)
			}
		}
	}
}
