package store

import (
	"context"
	"sync"
	"time"
)

type challengeEntry struct {
	record    ChallengeRecord
	expiresAt time.Time
}

type windowEntry struct {
	startedAt time.Time
	count     int
	window    time.Duration
}

const rateShardCount = 64

type rateShard struct {
	mu      sync.Mutex
	windows map[string]windowEntry
	ops     uint64
	_       [40]byte // prevent false sharing across CPU cache lines
}

// MemoryStore is a concurrency-safe, sharded in-memory implementation of Store.
type MemoryStore struct {
	chalMu     sync.Mutex
	challenges map[string]challengeEntry
	chalOps    uint64

	shards [rateShardCount]rateShard
	now    func() time.Time
}

// NewMemoryStore creates a new in-memory Store using the system clock.
func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithClock(time.Now)
}

// NewMemoryStoreWithClock creates a MemoryStore with a custom clock function for testing.
func NewMemoryStoreWithClock(clock func() time.Time) *MemoryStore {
	m := &MemoryStore{
		challenges: make(map[string]challengeEntry),
		now:        clock,
	}
	for i := range m.shards {
		m.shards[i].windows = make(map[string]windowEntry)
	}
	return m
}

func (m *MemoryStore) SaveChallenge(_ context.Context, challengeID string, record ChallengeRecord, ttlSeconds int) error {
	m.chalMu.Lock()
	defer m.chalMu.Unlock()

	now := m.now()
	m.maybeSweepChallengesLocked(now)

	var expiresAt time.Time
	if ttlSeconds <= 0 {
		if cur, ok := m.challenges[challengeID]; ok && cur.expiresAt.After(now) {
			expiresAt = cur.expiresAt
		} else {
			expiresAt = now.Add(time.Second)
		}
	} else {
		expiresAt = now.Add(time.Duration(ttlSeconds) * time.Second)
	}
	m.challenges[challengeID] = challengeEntry{
		record:    record,
		expiresAt: expiresAt,
	}
	return nil
}

func (m *MemoryStore) GetChallenge(_ context.Context, challengeID string) (*ChallengeRecord, error) {
	m.chalMu.Lock()
	defer m.chalMu.Unlock()

	cur, ok := m.challenges[challengeID]
	if !ok {
		return nil, nil
	}
	if !cur.expiresAt.After(m.now()) {
		delete(m.challenges, challengeID)
		return nil, nil
	}
	rec := cur.record
	return &rec, nil
}

func (m *MemoryStore) DeleteChallenge(_ context.Context, challengeID string) error {
	m.chalMu.Lock()
	defer m.chalMu.Unlock()
	delete(m.challenges, challengeID)
	return nil
}

func (m *MemoryStore) Increment(_ context.Context, key string, windowSeconds int) (int, error) {
	shard := &m.shards[fnv1a64(key)&(rateShardCount-1)]
	now := m.now()
	win := time.Duration(windowSeconds) * time.Second

	shard.mu.Lock()
	shard.ops++
	if shard.ops&255 == 0 {
		for k, v := range shard.windows {
			if now.Sub(v.startedAt) >= v.window {
				delete(shard.windows, k)
			}
		}
	}

	cur, ok := shard.windows[key]
	if !ok || now.Sub(cur.startedAt) >= win {
		cur = windowEntry{startedAt: now, count: 1, window: win}
	} else {
		cur.count++
		cur.window = win
	}
	shard.windows[key] = cur
	count := cur.count
	shard.mu.Unlock()
	return count, nil
}

func (m *MemoryStore) Close() error {
	return nil
}

func (m *MemoryStore) maybeSweepChallengesLocked(now time.Time) {
	m.chalOps++
	if m.chalOps&255 != 0 {
		return
	}
	for k, v := range m.challenges {
		if !v.expiresAt.After(now) {
			delete(m.challenges, k)
		}
	}
}

func fnv1a64(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}
