package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ParseRemoteRules parses a remote rules payload encoded as either:
//   - JSON array of CustomRule objects: [ { "action": "block", "paths": [...] }, ... ]
//   - JSON object with a "rules" field: { "rules": [ ... ] }
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
			Rules []CustomRule `json:"rules"`
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
