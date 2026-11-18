package intel

import (
	"cmp"
	"net/netip"
	"slices"
	"strings"
)

// IPSet is an immutable, zero-allocation lookup table for IPv4/IPv6 addresses and CIDR prefixes.
// Single-host addresses are stored in an O(1) hash map, while network prefixes are bucketed by
// prefix bit-length so Contains performs at most one hash lookup per active prefix length.
type IPSet struct {
	addrs        map[netip.Addr]struct{}
	v4ByBits     [33]map[netip.Prefix]struct{}
	activeV4Bits []int
	v6ByBits     [129]map[netip.Prefix]struct{}
	activeV6Bits []int
	totalEntries int
}

// NewIPSet compiles a slice of netip.Prefix entries into an immutable IPSet.
func NewIPSet(prefixes []netip.Prefix) *IPSet {
	s := &IPSet{
		addrs: make(map[netip.Addr]struct{}),
	}
	v4SeenBits := make(map[int]struct{})
	v6SeenBits := make(map[int]struct{})

	for _, p := range prefixes {
		if !p.IsValid() {
			continue
		}
		masked := p.Masked()
		addr := masked.Addr().Unmap()
		if addr.Is4() {
			bits := masked.Bits()
			if p.Addr().Is4In6() {
				bits -= 96
			}
			if bits >= 32 {
				if _, exists := s.addrs[addr]; !exists {
					s.addrs[addr] = struct{}{}
					s.totalEntries++
				}
				continue
			}
			if bits < 0 {
				continue
			}
			v4Pref := netip.PrefixFrom(addr, bits).Masked()
			if s.v4ByBits[bits] == nil {
				s.v4ByBits[bits] = make(map[netip.Prefix]struct{})
				v4SeenBits[bits] = struct{}{}
			}
			if _, exists := s.v4ByBits[bits][v4Pref]; !exists {
				s.v4ByBits[bits][v4Pref] = struct{}{}
				s.totalEntries++
			}
		} else {
			bits := masked.Bits()
			if bits >= 128 {
				if _, exists := s.addrs[addr]; !exists {
					s.addrs[addr] = struct{}{}
					s.totalEntries++
				}
				continue
			}
			if s.v6ByBits[bits] == nil {
				s.v6ByBits[bits] = make(map[netip.Prefix]struct{})
				v6SeenBits[bits] = struct{}{}
			}
			if _, exists := s.v6ByBits[bits][masked]; !exists {
				s.v6ByBits[bits][masked] = struct{}{}
				s.totalEntries++
			}
		}
	}

	for b := range v4SeenBits {
		s.activeV4Bits = append(s.activeV4Bits, b)
	}
	slices.SortFunc(s.activeV4Bits, func(a, b int) int { return cmp.Compare(b, a) }) // most specific first

	for b := range v6SeenBits {
		s.activeV6Bits = append(s.activeV6Bits, b)
	}
	slices.SortFunc(s.activeV6Bits, func(a, b int) int { return cmp.Compare(b, a) })

	return s
}

// Len returns the number of unique IP/CIDR entries in the set.
func (s *IPSet) Len() int {
	if s == nil {
		return 0
	}
	return s.totalEntries
}

// ContainsString parses ipStr and reports whether it belongs to the IPSet.
func (s *IPSet) ContainsString(ipStr string) bool {
	if s == nil || s.totalEntries == 0 {
		return false
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil {
		return false
	}
	return s.Contains(addr)
}

// Contains reports whether addr matches any exact IP or CIDR block in the IPSet.
func (s *IPSet) Contains(addr netip.Addr) bool {
	if s == nil || s.totalEntries == 0 || !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if _, ok := s.addrs[addr]; ok {
		return true
	}
	if addr.Is4() {
		for _, bits := range s.activeV4Bits {
			pref, _ := addr.Prefix(bits)
			if _, ok := s.v4ByBits[bits][pref]; ok {
				return true
			}
		}
		return false
	}
	for _, bits := range s.activeV6Bits {
		pref, _ := addr.Prefix(bits)
		if _, ok := s.v6ByBits[bits][pref]; ok {
			return true
		}
	}
	return false
}

// ParseIPOrCIDR parses either a single IP literal ("1.2.3.4", "2001:db8::1") or a CIDR ("1.2.3.0/24")
// into a normalized netip.Prefix.
func ParseIPOrCIDR(raw string) (netip.Prefix, error) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		return p.Masked(), nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
