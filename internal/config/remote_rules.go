package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxRuleFileBytes = 4 << 20 // 4 MiB cap per rule file

// ParseRemoteRules parses a ruleset payload encoded as either:
//   - JSON array of CustomRule objects: [ { "action": "block", "paths": [...] }, ... ]
//   - JSON object with a "rules" field (optional "name" and "description"): { "rules": [ ... ] }
//   - Plain-text Gymbagfile rule directives (shorthand allow/verify/block lines and `rule [name] { ... }` blocks)
//
// Every parsed rule is normalized and validated via CustomRule.Validate().
func ParseRemoteRules(raw []byte) ([]CustomRule, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}

	// JSON format detection: either a JSON array or a JSON object containing "rules".
	if trimmed[0] == '[' || (trimmed[0] == '{' && json.Valid(trimmed)) {
		return parseRemoteRulesJSON(trimmed)
	}

	return parseRemoteRulesText(string(trimmed))
}

func parseRemoteRulesJSON(raw []byte) ([]CustomRule, error) {
	var rules []CustomRule
	if raw[0] == '[' {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rules); err != nil {
			return nil, fmt.Errorf("invalid remote rules JSON array: %w", err)
		}
	} else {
		var wrapper struct {
			Name        string       `json:"name,omitempty"`
			Description string       `json:"description,omitempty"`
			Rules       []CustomRule `json:"rules"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&wrapper); err != nil {
			return nil, fmt.Errorf("invalid remote rules JSON object: %w", err)
		}
		rules = wrapper.Rules
	}

	for i := range rules {
		if err := rules[i].Validate(); err != nil {
			return nil, fmt.Errorf("remote rule #%d: %w", i+1, err)
		}
	}
	return rules, nil
}

func parseRemoteRulesText(text string) ([]CustomRule, error) {
	tokens, err := tokenizeCaddyfile(text)
	if err != nil {
		return nil, err
	}
	items, err := parseItems(tokens)
	if err != nil {
		return nil, err
	}

	var rules []CustomRule
	for _, item := range items {
		if len(item.words) == 0 {
			continue
		}
		if item.sub != nil {
			if !strings.EqualFold(item.words[0], "rule") {
				return nil, fmt.Errorf("unsupported block %q in remote rules (expected 'rule [name] { ... }')", item.words[0])
			}
			r, err := parseRuleBlock(item.sub)
			if err != nil {
				return nil, err
			}
			rules = append(rules, r)
			continue
		}

		actionWord := strings.ToLower(item.words[0])
		r, err := parseShorthandRule(actionWord, item.words[1:])
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// FormatRules serializes rules into standalone Gymbagfile rule syntax (column 0).
func FormatRules(rules []CustomRule) string {
	if len(rules) == 0 {
		return ""
	}
	var raw strings.Builder
	for _, r := range rules {
		formatCustomRule(&raw, r)
	}
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(raw.String(), "\n"), "\n") {
		out.WriteString(strings.TrimPrefix(line, "    "))
		out.WriteByte('\n')
	}
	return out.String()
}

// FormatRulesJSON serializes rules into an indented JSON ruleset object with a trailing newline.
func FormatRulesJSON(rules []CustomRule) ([]byte, error) {
	if rules == nil {
		rules = []CustomRule{}
	}
	payload := struct {
		Rules []CustomRule `json:"rules"`
	}{Rules: rules}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// ResolveRuleSourcePath resolves a local rule file or directory path relative to baseDir when relative.
func ResolveRuleSourcePath(source, baseDir string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}
	rs := RuleSource{Source: trimmed}
	if rs.IsURL() {
		return trimmed
	}
	if filepath.IsAbs(trimmed) || strings.TrimSpace(baseDir) == "" {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

func isIgnoredRuleFilename(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tmp") ||
		strings.HasSuffix(lower, ".swp") ||
		strings.HasSuffix(lower, ".swo") ||
		strings.HasSuffix(lower, ".bak") ||
		strings.HasSuffix(name, "~")
}

func readLimitedRuleFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRuleFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRuleFileBytes {
		return nil, fmt.Errorf("rule file %s exceeds %d bytes limit", path, maxRuleFileBytes)
	}
	return data, nil
}

// LoadRuleSourceFromDisk loads and validates a dedicated local rule file or a directory of rule files.
// Directories load all non-hidden regular files sorted lexicographically by filename.
func LoadRuleSourceFromDisk(source, baseDir string) (rules []CustomRule, files []string, kind string, fingerprint [32]byte, err error) {
	resolved := ResolveRuleSourcePath(source, baseDir)
	if resolved == "" {
		return nil, nil, "", fingerprint, fmt.Errorf("rule source path must not be empty")
	}

	info, statErr := os.Stat(resolved)
	if statErr != nil {
		return nil, nil, "", fingerprint, fmt.Errorf("failed to stat rule source %s: %w", resolved, statErr)
	}

	if !info.IsDir() {
		raw, rErr := readLimitedRuleFile(resolved)
		if rErr != nil {
			return nil, nil, "file", fingerprint, fmt.Errorf("failed to read rule file %s: %w", resolved, rErr)
		}
		parsed, pErr := ParseRemoteRules(raw)
		if pErr != nil {
			return nil, nil, "file", fingerprint, fmt.Errorf("rule file %s: %w", resolved, pErr)
		}
		h := sha256.New()
		h.Write([]byte(filepath.Base(resolved)))
		h.Write([]byte{0})
		h.Write(raw)
		copy(fingerprint[:], h.Sum(nil))
		return parsed, []string{resolved}, "file", fingerprint, nil
	}

	entries, rErr := os.ReadDir(resolved)
	if rErr != nil {
		return nil, nil, "dir", fingerprint, fmt.Errorf("failed to read rules directory %s: %w", resolved, rErr)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || isIgnoredRuleFilename(name) {
			continue
		}
		full := filepath.Join(resolved, name)
		st, stErr := os.Stat(full)
		if stErr != nil || st.IsDir() || !st.Mode().IsRegular() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		full := filepath.Join(resolved, name)
		raw, fErr := readLimitedRuleFile(full)
		if fErr != nil {
			return nil, nil, "dir", fingerprint, fmt.Errorf("failed to read rule file %s: %w", full, fErr)
		}
		parsed, pErr := ParseRemoteRules(raw)
		if pErr != nil {
			return nil, nil, "dir", fingerprint, fmt.Errorf("rule file %s: %w", full, pErr)
		}
		rules = append(rules, parsed...)
		files = append(files, full)
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(raw)
		h.Write([]byte{0})
	}
	copy(fingerprint[:], h.Sum(nil))
	return rules, files, "dir", fingerprint, nil
}
