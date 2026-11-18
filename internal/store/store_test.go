package store

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMemoryStoreChallengeAndRateWindow(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	ms := NewMemoryStoreWithClock(func() time.Time { return now })
	ctx := context.Background()

	rec := ChallengeRecord{ClientIP: "203.0.113.1", Host: "example.com", Difficulty: 4, Attempts: 0}
	if err := ms.SaveChallenge(ctx, "c1", rec, 60); err != nil {
		t.Fatal(err)
	}
	got, err := ms.GetChallenge(ctx, "c1")
	if err != nil || got == nil || got.ClientIP != "203.0.113.1" || got.Attempts != 0 {
		t.Fatalf("got = %+v, err = %v", got, err)
	}

	// Update attempts while preserving original TTL (ttlSeconds = 0)
	now = now.Add(20 * time.Second)
	if err := ms.SaveChallenge(ctx, "c1", rec.Failed(), 0); err != nil {
		t.Fatal(err)
	}
	got, _ = ms.GetChallenge(ctx, "c1")
	if got == nil || got.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %+v", got)
	}

	// Advance past original 60s TTL
	now = now.Add(41 * time.Second)
	got, _ = ms.GetChallenge(ctx, "c1")
	if got != nil {
		t.Fatalf("expected expired nil challenge, got %+v", got)
	}

	// Rate window check
	c1, _ := ms.Increment(ctx, "ip:example.com:1.2.3.4", 60)
	c2, _ := ms.Increment(ctx, "ip:example.com:1.2.3.4", 60)
	if c1 != 1 || c2 != 2 {
		t.Fatalf("counts = (%d, %d), want (1, 2)", c1, c2)
	}
	now = now.Add(61 * time.Second)
	c3, _ := ms.Increment(ctx, "ip:example.com:1.2.3.4", 60)
	if c3 != 1 {
		t.Fatalf("after window expiry count = %d, want 1", c3)
	}
}

// miniRedisServer is a lightweight in-process RESP2 server over net.Pipe for testing RedisStore.
type miniRedisServer struct {
	mu   sync.Mutex
	kv   map[string]string
	ttls map[string]int
}

func startMiniRedis(t *testing.T) *miniRedisServer {
	t.Helper()
	return &miniRedisServer{
		kv:   make(map[string]string),
		ttls: make(map[string]int),
	}
}

func (s *miniRedisServer) dial(_ context.Context, _, _ string) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	go s.handleConn(serverConn)
	return clientConn, nil
}

func (s *miniRedisServer) handleConn(conn net.Conn) {
	defer conn.Close()
	rd := bufio.NewReader(conn)
	for {
		raw, err := readRESP(rd)
		if err != nil {
			return
		}
		arr, ok := raw.([]any)
		if !ok || len(arr) == 0 {
			return
		}
		args := make([]string, len(arr))
		for i, v := range arr {
			args[i], _ = v.(string)
		}
		cmd := strings.ToUpper(args[0])
		s.mu.Lock()
		switch cmd {
		case "PING":
			_, _ = io.WriteString(conn, "+PONG\r\n")
		case "SET":
			key, val := args[1], args[2]
			s.kv[key] = val
			if len(args) >= 5 && strings.EqualFold(args[3], "EX") {
				ttl, _ := strconv.Atoi(args[4])
				s.ttls[key] = ttl
			}
			_, _ = io.WriteString(conn, "+OK\r\n")
		case "GET":
			val, exists := s.kv[args[1]]
			if !exists {
				_, _ = io.WriteString(conn, "$-1\r\n")
			} else {
				_, _ = io.WriteString(conn, fmt.Sprintf("$%d\r\n%s\r\n", len(val), val))
			}
		case "TTL":
			ttl, exists := s.ttls[args[1]]
			if !exists {
				_, _ = io.WriteString(conn, ":-2\r\n")
			} else {
				_, _ = io.WriteString(conn, fmt.Sprintf(":%d\r\n", ttl))
			}
		case "DEL":
			delete(s.kv, args[1])
			delete(s.ttls, args[1])
			_, _ = io.WriteString(conn, ":1\r\n")
		case "INCR":
			cur, _ := strconv.Atoi(s.kv[args[1]])
			cur++
			s.kv[args[1]] = strconv.Itoa(cur)
			_, _ = io.WriteString(conn, fmt.Sprintf(":%d\r\n", cur))
		case "EXPIRE":
			ttl, _ := strconv.Atoi(args[2])
			s.ttls[args[1]] = ttl
			_, _ = io.WriteString(conn, ":1\r\n")
		default:
			_, _ = io.WriteString(conn, "-ERR unknown command\r\n")
		}
		s.mu.Unlock()
	}
}

func TestRedisStoreAndFallback(t *testing.T) {
	srv := startMiniRedis(t)
	ctx := context.Background()

	rs, err := NewRedisStoreWithDialer(ctx, "redis://127.0.0.1:6379/0", srv.dial)
	if err != nil {
		t.Fatalf("NewRedisStore failed: %v", err)
	}
	defer rs.Close()

	rec := ChallengeRecord{ClientIP: "198.51.100.2", Host: "app.example.com", Difficulty: 3, Attempts: 0}
	if err := rs.SaveChallenge(ctx, "chal-redis", rec, 120); err != nil {
		t.Fatal(err)
	}
	if err := rs.SaveChallenge(ctx, "chal-redis", rec.Failed(), 0); err != nil {
		t.Fatal(err)
	}
	got, err := rs.GetChallenge(ctx, "chal-redis")
	if err != nil || got == nil || got.Attempts != 1 || got.Host != "app.example.com" {
		t.Fatalf("GetChallenge = %+v, err = %v", got, err)
	}
	if err := rs.DeleteChallenge(ctx, "chal-redis"); err != nil {
		t.Fatal(err)
	}
	got, _ = rs.GetChallenge(ctx, "chal-redis")
	if got != nil {
		t.Fatalf("expected deleted challenge, got %+v", got)
	}

	n1, err := rs.Increment(ctx, "ip:app.example.com:198.51.100.2", 60)
	if err != nil || n1 != 1 {
		t.Fatalf("Increment 1 = %d, %v", n1, err)
	}
	n2, err := rs.Increment(ctx, "ip:app.example.com:198.51.100.2", 60)
	if err != nil || n2 != 2 {
		t.Fatalf("Increment 2 = %d, %v", n2, err)
	}

	// Open fallback when Redis is unreachable
	fallback := Open(ctx, "redis://127.0.0.1:1/0", nil)
	defer fallback.Close()
	if c, err := fallback.Increment(ctx, "k", 10); err != nil || c != 1 {
		t.Fatalf("fallback Increment = %d, %v", c, err)
	}
}
