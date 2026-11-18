package waf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ProofHex computes the lowercase SHA-256 hex digest of "<challengeID>:<nonce>".
func ProofHex(challengeID string, nonce int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", challengeID, nonce)))
	return hex.EncodeToString(sum[:])
}

// Accepts reports whether "<challengeID>:<nonce>" satisfies the given leading-zero hex difficulty.
func Accepts(challengeID string, nonce int64, difficulty int) bool {
	if difficulty < 1 || nonce < 0 {
		return false
	}
	prefix := strings.Repeat("0", difficulty)
	return strings.HasPrefix(ProofHex(challengeID, nonce), prefix)
}
