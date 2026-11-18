package waf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gymbag/internal/host"
)

// ClearanceTokens issues and verifies HMAC-SHA256 clearance cookies bound to (host, clientIP).
type ClearanceTokens struct {
	secret     []byte
	defaultTTL int
	now        func() time.Time
}

// NewClearanceTokens constructs a ClearanceTokens manager using secret and defaultTTL seconds.
func NewClearanceTokens(secret []byte, defaultTTL int) *ClearanceTokens {
	return &ClearanceTokens{
		secret:     append([]byte(nil), secret...),
		defaultTTL: defaultTTL,
		now:        time.Now,
	}
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
	body := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%s", expires, bindingHash(hostName, clientIP))))
	return body + "." + c.sign(body)
}

// Valid checks the HMAC signature, expiry timestamp, and (host, clientIP) binding in constant time.
func (c *ClearanceTokens) Valid(token, hostName, clientIP string) bool {
	if token == "" || strings.Count(token, ".") != 1 {
		return false
	}
	body, sig, _ := strings.Cut(token, ".")
	expectedSig := c.sign(body)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return false
	}
	expiresStr, binding, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return false
	}
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || expires < c.now().Unix() {
		return false
	}
	expectedBinding := bindingHash(hostName, clientIP)
	return hmac.Equal([]byte(binding), []byte(expectedBinding))
}

func bindingHash(hostName, clientIP string) string {
	normHost := host.Normalize(hostName)
	var input string
	if normHost != "" {
		input = normHost + "|" + clientIP
	} else {
		input = clientIP
	}
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func (c *ClearanceTokens) sign(body string) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
