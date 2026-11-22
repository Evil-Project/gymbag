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

// GateTicket stores state for a pending browser verification link issued to an SSH or Minecraft client.
type GateTicket struct {
	Service    string `json:"service"`
	ClientIP   string `json:"ip"`
	Username   string `json:"username,omitempty"`
	Difficulty int    `json:"difficulty"`
	ClearTTL   int    `json:"clear_ttl"`
}

// Marshal serializes GateTicket to compact JSON.
func (t GateTicket) Marshal() (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalGateTicket deserializes GateTicket from JSON.
func UnmarshalGateTicket(payload string) (GateTicket, error) {
	var t GateTicket
	err := json.Unmarshal([]byte(payload), &t)
	return t, err
}

// GateStore manages verification tickets and IP clearances for non-HTTP protocol gates (SSH, Minecraft).
type GateStore interface {
	SaveGateTicket(ctx context.Context, token string, ticket GateTicket, ttlSeconds int) error
	GetGateTicket(ctx context.Context, token string) (*GateTicket, error)
	DeleteGateTicket(ctx context.Context, token string) error
	SaveClearance(ctx context.Context, service, clientIP string, ttlSeconds int) error
	IsCleared(ctx context.Context, service, clientIP string) (expiresAt int64, cleared bool, err error)
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
