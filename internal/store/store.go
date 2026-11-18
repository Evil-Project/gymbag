package store

import (
	"context"
	"encoding/json"
	"log/slog"
)

// ChallengeRecord stores state for an issued single-use proof-of-work challenge.
type ChallengeRecord struct {
	ClientIP   string `json:"ip"`
	Host       string `json:"host,omitempty"`
	Difficulty int    `json:"difficulty"`
	Attempts   int    `json:"attempts"`
}

// Failed returns an immutable copy with Attempts incremented by one.
func (r ChallengeRecord) Failed() ChallengeRecord {
	r.Attempts++
	return r
}

// Marshal serializes ChallengeRecord to compact JSON.
func (r ChallengeRecord) Marshal() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalChallengeRecord deserializes ChallengeRecord from JSON.
func UnmarshalChallengeRecord(payload string) (ChallengeRecord, error) {
	var r ChallengeRecord
	err := json.Unmarshal([]byte(payload), &r)
	return r, err
}

// ChallengeStore manages single-use PoW challenge records (ISP).
type ChallengeStore interface {
	// SaveChallenge stores record under challengeID. When ttlSeconds <= 0, the existing TTL is preserved.
	SaveChallenge(ctx context.Context, challengeID string, record ChallengeRecord, ttlSeconds int) error
	GetChallenge(ctx context.Context, challengeID string) (*ChallengeRecord, error)
	DeleteChallenge(ctx context.Context, challengeID string) error
}

// RateCounter tracks fixed-window request counters per key (ISP).
type RateCounter interface {
	Increment(ctx context.Context, key string, windowSeconds int) (int, error)
}

// Store composes ChallengeStore, RateCounter, and lifecycle cleanup.
type Store interface {
	ChallengeStore
	RateCounter
	Close() error
}

// Open opens a Redis-backed Store if redisURL is non-empty and reachable,
// otherwise falls back cleanly to an in-memory Store.
func Open(ctx context.Context, redisURL string, logger *slog.Logger) Store {
	if redisURL == "" {
		return NewMemoryStore()
	}
	rs, err := NewRedisStore(ctx, redisURL)
	if err != nil {
		if logger != nil {
			logger.Warn("Redis is unreachable; WAF state will stay in memory", "error", err)
		}
		return NewMemoryStore()
	}
	return rs
}
