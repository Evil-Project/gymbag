package waf

import "strings"

// asciiAC is a pre-completed case-insensitive ASCII Aho-Corasick DFA for O(len(input)),
// zero-allocation multi-pattern substring matching. Non-ASCII patterns (if any) fall back
// to standard lowercased substring checks.
type asciiAC struct {
	next     [][128]int32
	match    []bool
	nonASCII []string
}

func compileASCIIAC(patterns []string) *asciiAC {
	if len(patterns) == 0 {
		return nil
	}
	ac := &asciiAC{
		next:  make([][128]int32, 1),
		match: make([]bool, 1),
	}
	hasASCII := false
	for _, raw := range patterns {
		p := strings.ToLower(raw)
		if p == "" {
			continue
		}
		if !isPureASCII(p) {
			ac.nonASCII = append(ac.nonASCII, p)
			continue
		}
		hasASCII = true
		cur := int32(0)
		for i := 0; i < len(p); i++ {
			c := p[i]
			nxt := ac.next[cur][c]
			if nxt == 0 {
				nxt = int32(len(ac.next))
				ac.next = append(ac.next, [128]int32{})
				ac.match = append(ac.match, false)
				ac.next[cur][c] = nxt
			}
			cur = nxt
		}
		ac.match[cur] = true
	}
	if !hasASCII {
		ac.next = nil
		ac.match = nil
		if len(ac.nonASCII) == 0 {
			return nil
		}
		return ac
	}

	// BFS to compute failure links and complete the DFA transition table so matching
	// performs exactly one table lookup per input byte with zero branches for fail links.
	fail := make([]int32, len(ac.next))
	queue := make([]int32, 0, len(ac.next))
	for c := 0; c < 128; c++ {
		if nxt := ac.next[0][c]; nxt != 0 {
			queue = append(queue, nxt)
		}
	}
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		f := fail[u]
		if ac.match[f] {
			ac.match[u] = true
		}
		for c := 0; c < 128; c++ {
			v := ac.next[u][c]
			if v != 0 {
				fail[v] = ac.next[f][c]
				queue = append(queue, v)
			} else {
				ac.next[u][c] = ac.next[f][c]
			}
		}
	}
	return ac
}

// MatchAny reports whether s contains any compiled pattern (case-insensitive for ASCII)
// without allocating a lowercased copy of s.
func (ac *asciiAC) MatchAny(s string) bool {
	if ac == nil || s == "" {
		return false
	}
	if len(ac.next) > 0 {
		state := int32(0)
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c < 128 {
				state = ac.next[state][c]
				if ac.match[state] {
					return true
				}
			} else {
				state = 0
			}
		}
	}
	if len(ac.nonASCII) > 0 {
		lower := strings.ToLower(s)
		for _, p := range ac.nonASCII {
			if strings.Contains(lower, p) {
				return true
			}
		}
	}
	return false
}

func isPureASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 128 {
			return false
		}
	}
	return true
}
