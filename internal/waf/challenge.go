package waf

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"gymbag/internal/host"
	"gymbag/internal/store"
)

// MaxChallengeAttempts is the maximum failed verification attempts before a challenge is burned.
const MaxChallengeAttempts = 8

// ChallengeService issues single-use proof-of-work challenges and verifies browser solutions.
type ChallengeService struct {
	store             store.ChallengeStore
	defaultDifficulty int
	defaultTTL        int
}

// NewChallengeService creates a ChallengeService backed by ChallengeStore.
func NewChallengeService(st store.ChallengeStore, defaultDifficulty, defaultTTL int) *ChallengeService {
	return &ChallengeService{
		store:             st,
		defaultDifficulty: defaultDifficulty,
		defaultTTL:        defaultTTL,
	}
}

// Issue creates and persists a single-use challenge bound to (hostName, clientIP).
func (s *ChallengeService) Issue(ctx context.Context, hostName, clientIP string, difficulty, ttlSeconds int) (string, int, error) {
	if difficulty <= 0 {
		difficulty = s.defaultDifficulty
	}
	if ttlSeconds <= 0 {
		ttlSeconds = s.defaultTTL
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", 0, err
	}
	challengeID := base64.RawURLEncoding.EncodeToString(buf)
	rec := store.ChallengeRecord{
		ClientIP:   clientIP,
		Host:       host.Normalize(hostName),
		Difficulty: difficulty,
		Attempts:   0,
	}
	if err := s.store.SaveChallenge(ctx, challengeID, rec, ttlSeconds); err != nil {
		return "", 0, err
	}
	return challengeID, difficulty, nil
}

// Verify checks a submitted nonce against the stored challenge record and enforces single-use + attempt limits.
func (s *ChallengeService) Verify(ctx context.Context, hostName, clientIP string, sub Submission) (bool, error) {
	rec, err := s.store.GetChallenge(ctx, sub.ChallengeID)
	if err != nil || rec == nil {
		return false, err
	}
	normHost := host.Normalize(hostName)
	if rec.ClientIP != clientIP || (rec.Host != "" && normHost != "" && rec.Host != normHost) {
		return false, nil
	}
	if sub.Verified && Accepts(sub.ChallengeID, sub.Nonce, rec.Difficulty) {
		_ = s.store.DeleteChallenge(ctx, sub.ChallengeID)
		return true, nil
	}
	updated := rec.Failed()
	if updated.Attempts >= MaxChallengeAttempts {
		_ = s.store.DeleteChallenge(ctx, sub.ChallengeID)
	} else {
		_ = s.store.SaveChallenge(ctx, sub.ChallengeID, updated, 0)
	}
	return false, nil
}
