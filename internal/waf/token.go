package waf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"strconv"
	"strings"
	"sync"
	"time"

	"gymbag/internal/host"
)

const (
	tokenCacheShards    = 64
	tokenCacheShardMax  = 2048
)

type tokenCacheKey struct {
	token    string
	host     string
	clientIP string
}

type tokenCacheShard struct {
	mu      sync.RWMutex
	entries map[tokenCacheKey]int64
	_       [32]byte
}

// ClearanceTokens issues and verifies HMAC-SHA256 clearance cookies bound to (host, clientIP).
type ClearanceTokens struct {
	secret     []byte
	defaultTTL int
	now        func() time.Time
	macPool    sync.Pool
	cache      [tokenCacheShards]tokenCacheShard
}

// NewClearanceTokens constructs a ClearanceTokens manager using secret and defaultTTL seconds.
func NewClearanceTokens(secret []byte, defaultTTL int) *ClearanceTokens {
	sec := append([]byte(nil), secret...)
	c := &ClearanceTokens{
		secret:     sec,
		defaultTTL: defaultTTL,
		now:        time.Now,
	}
	c.macPool.New = func() any {
		return hmac.New(sha256.New, c.secret)
	}
	for i := range c.cache {
		c.cache[i].entries = make(map[tokenCacheKey]int64)
	}
	return c
}

// NewClearanceTokensWithClock constructs a ClearanceTokens manager with an injectable clock.
func NewClearanceTokensWithClock(secret []byte, defaultTTL int, clock func() time.Time) *ClearanceTokens {
	t := NewClearanceTokens(secret, defaultTTL)
	t.now = clock
	return t
}

// Issue generates a signed clearance cookie value bound to hostName and clientIP.
func (c *ClearanceTokens) Issue(hostName, clientIP string, ttlSeconds int) string {
	ttl := ttlSeconds
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	expires := c.now().Unix() + int64(ttl)

	var plainBuf [96]byte
	p := strconv.AppendInt(plainBuf[:0], expires, 10)
	p = append(p, ':')
	var hexBuf [sha256.Size * 2]byte
	writeBindingHashHex(hexBuf[:], hostName, clientIP)
	p = append(p, hexBuf[:]...)

	body := base64.RawURLEncoding.EncodeToString(p)
	return body + "." + c.sign(body)
}

// Valid checks the HMAC signature, expiry timestamp, and (host, clientIP) binding in constant time.
func (c *ClearanceTokens) Valid(token, hostName, clientIP string) bool {
	_, ok := c.Parse(token, hostName, clientIP)
	return ok
}

// Parse validates token for (hostName, clientIP) and returns its Unix expiration timestamp if valid.
func (c *ClearanceTokens) Parse(token, hostName, clientIP string) (int64, bool) {
	if token == "" {
		return 0, false
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 || strings.IndexByte(token[dot+1:], '.') != -1 {
		return 0, false
	}

	normHost := host.Normalize(hostName)
	nowUnix := c.now().Unix()

	// Fast path: check sharded verified-token cache.
	key := tokenCacheKey{token: token, host: normHost, clientIP: clientIP}
	shard := &c.cache[tokenKeyHash(token, normHost, clientIP)&(tokenCacheShards-1)]
	shard.mu.RLock()
	if cachedExp, ok := shard.entries[key]; ok {
		shard.mu.RUnlock()
		if cachedExp >= nowUnix {
			return cachedExp, true
		}
		shard.mu.Lock()
		delete(shard.entries, key)
		shard.mu.Unlock()
		return 0, false
	}
	shard.mu.RUnlock()

	body := token[:dot]
	sig := token[dot+1:]

	var sigBuf [43]byte
	expectedSig := c.signInto(sigBuf[:0], body)
	if len(sig) != len(expectedSig) || !hmac.Equal([]byte(sig), expectedSig) {
		return 0, false
	}

	var decodedBuf [128]byte
	var decoded []byte
	if base64.RawURLEncoding.DecodedLen(len(body)) <= len(decodedBuf) {
		n, err := base64.RawURLEncoding.Decode(decodedBuf[:], []byte(body))
		if err != nil {
			return 0, false
		}
		decoded = decodedBuf[:n]
	} else {
		var err error
		decoded, err = base64.RawURLEncoding.DecodeString(body)
		if err != nil {
			return 0, false
		}
	}

	colon := -1
	for i := 0; i < len(decoded); i++ {
		if decoded[i] == ':' {
			colon = i
			break
		}
	}
	if colon <= 0 {
		return 0, false
	}
	expires, err := strconv.ParseInt(string(decoded[:colon]), 10, 64)
	if err != nil || expires < nowUnix {
		return 0, false
	}
	binding := decoded[colon+1:]

	var expectedBinding [sha256.Size * 2]byte
	writeBindingHashHex(expectedBinding[:], normHost, clientIP)
	if !hmac.Equal(binding, expectedBinding[:]) {
		return 0, false
	}

	// Cache verified token for fast subsequent requests.
	shard.mu.Lock()
	if len(shard.entries) >= tokenCacheShardMax {
		for k, exp := range shard.entries {
			if exp < nowUnix {
				delete(shard.entries, k)
			}
		}
		if len(shard.entries) >= tokenCacheShardMax {
			clear(shard.entries)
		}
	}
	shard.entries[key] = expires
	shard.mu.Unlock()

	return expires, true
}

func writeBindingHashHex(dst []byte, hostName, clientIP string) {
	normHost := host.Normalize(hostName)
	h := sha256.New()
	if normHost != "" {
		_, _ = h.Write([]byte(normHost))
		_, _ = h.Write([]byte{'|'})
	}
	_, _ = h.Write([]byte(clientIP))
	var sum [sha256.Size]byte
	h.Sum(sum[:0])
	hex.Encode(dst, sum[:])
}

func (c *ClearanceTokens) sign(body string) string {
	var buf [43]byte
	return string(c.signInto(buf[:0], body))
}

func (c *ClearanceTokens) signInto(dst []byte, body string) []byte {
	mac := c.macPool.Get().(hash.Hash)
	mac.Reset()
	_, _ = mac.Write([]byte(body))
	var sum [sha256.Size]byte
	mac.Sum(sum[:0])
	c.macPool.Put(mac)
	return base64.RawURLEncoding.AppendEncode(dst, sum[:])
}

func tokenKeyHash(token, hostName, clientIP string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(token); i++ {
		h ^= uint64(token[i])
		h *= prime64
	}
	for i := 0; i < len(hostName); i++ {
		h ^= uint64(hostName[i])
		h *= prime64
	}
	for i := 0; i < len(clientIP); i++ {
		h ^= uint64(clientIP[i])
		h *= prime64
	}
	return h
}
