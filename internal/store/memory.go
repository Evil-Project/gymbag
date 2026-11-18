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

// MemoryStore is a concurrency-safe in-memory implementation of Store.
type MemoryStore struct {
	mu         sync.Mutex
	challenges map[string]challengeEntry
	windows    map[string]windowEntry
	now        func() time.Time
	ops        uint64
}

// NewMemoryStore creates a new in-memory Store using the system clock.
func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithClock(time.Now)
}

// NewMemoryStoreWithClock creates a MemoryStore with a custom clock function for testing.
func NewMemoryStoreWithClock(clock func() time.Time) *MemoryStore {
	return &MemoryStore{
		challenges: make(map[string]challengeEntry),
		windows:    make(map[string]windowEntry),
		now:        clock,
	}
}

func (m *MemoryStore) SaveChallenge(_ context.Context, challengeID string, record ChallengeRecord, ttlSeconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	m.maybeSweepLocked(now)

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
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.challenges, challengeID)
	return nil
}

func (m *MemoryStore) Increment(_ context.Context, key string, windowSeconds int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	m.maybeSweepLocked(now)

	win := time.Duration(windowSeconds) * time.Second
	cur, ok := m.windows[key]
	if !ok || now.Sub(cur.startedAt) >= win {
		cur = windowEntry{startedAt: now, count: 0, window: win}
	}
	cur.count++
	cur.window = win
	m.windows[key] = cur
	return cur.count, nil
}

func (m *MemoryStore) Close() error {
	return nil
}

func (m *MemoryStore) maybeSweepLocked(now time.Time) {
	m.ops++
	if m.ops%256 != 0 {
		return
	}
	for k, v := range m.challenges {
		if !v.expiresAt.After(now) {
			delete(m.challenges, k)
		}
	}
	for k, v := range m.windows {
		if now.Sub(v.startedAt) >= v.window {
			delete(m.windows, k)
		}
	}
}
