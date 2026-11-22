package store

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DialContextFunc dials a raw transport connection for RedisStore (DIP).
type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// RedisStore implements Store over the Redis Serialization Protocol (RESP2)
// using only the Go standard library (supporting both redis:// and rediss://).
type RedisStore struct {
	mu       sync.Mutex
	addr     string
	hostName string
	useTLS   bool
	username string
	password string
	db       int
	dial     DialContextFunc
	conn     net.Conn
	reader   *bufio.Reader
	closed   bool
}

// NewRedisStore parses redisURL, connects, authenticates, selects DB, and verifies connectivity via PING.
func NewRedisStore(ctx context.Context, redisURL string) (*RedisStore, error) {
	return NewRedisStoreWithDialer(ctx, redisURL, nil)
}

// NewRedisStoreWithDialer creates a RedisStore using an optional custom dialer (for tests or Unix sockets).
func NewRedisStoreWithDialer(ctx context.Context, redisURL string, dial DialContextFunc) (*RedisStore, error) {
	u, err := url.Parse(strings.TrimSpace(redisURL))
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}
	var useTLS bool
	switch strings.ToLower(u.Scheme) {
	case "redis":
		useTLS = false
	case "rediss":
		useTLS = true
	default:
		return nil, fmt.Errorf("unsupported redis scheme %q (expected redis:// or rediss://)", u.Scheme)
	}
	hostName := u.Hostname()
	if hostName == "" {
		hostName = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		port = "6379"
	}
	db := 0
	if path := strings.Trim(u.Path, "/"); path != "" {
		parsedDB, convErr := strconv.Atoi(path)
		if convErr != nil || parsedDB < 0 {
			return nil, fmt.Errorf("invalid redis database index %q", path)
		}
		db = parsedDB
	}
	var username, password string
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	rs := &RedisStore{
		addr:     net.JoinHostPort(hostName, port),
		hostName: hostName,
		useTLS:   useTLS,
		username: username,
		password: password,
		db:       db,
		dial:     dial,
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if err := rs.connectLocked(ctx); err != nil {
		return nil, err
	}
	if _, err := rs.execLocked(ctx, "PING"); err != nil {
		_ = rs.closeConnLocked()
		return nil, err
	}
	return rs, nil
}

func (r *RedisStore) SaveChallenge(ctx context.Context, challengeID string, record ChallengeRecord, ttlSeconds int) error {
	payload, err := record.Marshal()
	if err != nil {
		return err
	}
	key := challengeKey(challengeID)

	r.mu.Lock()
	defer r.mu.Unlock()

	expiry := ttlSeconds
	if expiry <= 0 {
		val, ttlErr := r.doLocked(ctx, "TTL", key)
		if ttlErr == nil {
			if rem, ok := val.(int64); ok && rem > 0 {
				expiry = int(rem)
			}
		}
		if expiry <= 0 {
			expiry = 1
		}
	}
	_, err = r.doLocked(ctx, "SET", key, payload, "EX", strconv.Itoa(expiry))
	return err
}

func (r *RedisStore) GetChallenge(ctx context.Context, challengeID string) (*ChallengeRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	val, err := r.doLocked(ctx, "GET", challengeKey(challengeID))
	if err != nil || val == nil {
		return nil, err
	}
	str, ok := val.(string)
	if !ok {
		return nil, nil
	}
	rec, err := UnmarshalChallengeRecord(str)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *RedisStore) DeleteChallenge(ctx context.Context, challengeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.doLocked(ctx, "DEL", challengeKey(challengeID))
	return err
}

// SaveGateTicket stores a verification ticket under gymbag:ticket:<token> in Redis.
func (r *RedisStore) SaveGateTicket(ctx context.Context, token string, ticket GateTicket, ttlSeconds int) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	payload, err := ticket.Marshal()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err = r.doLocked(ctx, "SET", gateTicketKey(token), payload, "EX", strconv.Itoa(ttlSeconds))
	return err
}

// GetGateTicket retrieves an active GateTicket from Redis, returning nil if missing.
func (r *RedisStore) GetGateTicket(ctx context.Context, token string) (*GateTicket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	val, err := r.doLocked(ctx, "GET", gateTicketKey(token))
	if err != nil || val == nil {
		return nil, err
	}
	str, ok := val.(string)
	if !ok {
		return nil, nil
	}
	ticket, err := UnmarshalGateTicket(str)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// DeleteGateTicket deletes a verification ticket from Redis.
func (r *RedisStore) DeleteGateTicket(ctx context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.doLocked(ctx, "DEL", gateTicketKey(token))
	return err
}

// SaveClearance persists an IP clearance entry for (service, clientIP) in Redis.
func (r *RedisStore) SaveClearance(ctx context.Context, service, clientIP string, ttlSeconds int) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 86400
	}
	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.doLocked(ctx, "SET", gateClearanceKey(service, clientIP), strconv.FormatInt(expiresAt, 10), "EX", strconv.Itoa(ttlSeconds))
	return err
}

// IsCleared reports whether (service, clientIP) currently holds a valid clearance in Redis.
func (r *RedisStore) IsCleared(ctx context.Context, service, clientIP string) (int64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	val, err := r.doLocked(ctx, "GET", gateClearanceKey(service, clientIP))
	if err != nil || val == nil {
		return 0, false, err
	}
	str, ok := val.(string)
	if !ok {
		return 0, false, nil
	}
	expiresAt, err := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
	if err != nil || expiresAt <= time.Now().Unix() {
		return 0, false, nil
	}
	return expiresAt, true, nil
}

func (r *RedisStore) Increment(ctx context.Context, key string, windowSeconds int) (int, error) {
	namespaced := "gymbag:rate:" + key
	r.mu.Lock()
	defer r.mu.Unlock()

	val, err := r.doLocked(ctx, "INCR", namespaced)
	if err != nil {
		return 0, err
	}
	count, ok := val.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected INCR reply type %T", val)
	}
	if count == 1 {
		_, _ = r.doLocked(ctx, "EXPIRE", namespaced, strconv.Itoa(windowSeconds))
	}
	return int(count), nil
}

func (r *RedisStore) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return r.closeConnLocked()
}

func challengeKey(id string) string {
	return "gymbag:challenge:" + id
}

func gateTicketKey(token string) string {
	return "gymbag:ticket:" + token
}

func gateClearanceKey(service, clientIP string) string {
	return "gymbag:clearance:" + service + ":" + clientIP
}

func (r *RedisStore) doLocked(ctx context.Context, args ...string) (any, error) {
	if r.closed {
		return nil, errors.New("redis store is closed")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if r.conn == nil {
			if err := r.connectLocked(ctx); err != nil {
				return nil, err
			}
		}
		reply, err := r.execLocked(ctx, args...)
		if err == nil {
			return reply, nil
		}
		var netErr net.Error
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &netErr) {
			_ = r.closeConnLocked()
			continue
		}
		return nil, err
	}
	return nil, errors.New("redis command failed after reconnect")
}

func (r *RedisStore) connectLocked(ctx context.Context) error {
	var conn net.Conn
	var err error
	if r.dial != nil {
		conn, err = r.dial(ctx, "tcp", r.addr)
		if err == nil && r.useTLS {
			tlsConn := tls.Client(conn, &tls.Config{ServerName: r.hostName, MinVersion: tls.VersionTLS12})
			if hsErr := tlsConn.HandshakeContext(ctx); hsErr != nil {
				_ = conn.Close()
				return hsErr
			}
			conn = tlsConn
		}
	} else {
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		if r.useTLS {
			tlsDialer := &tls.Dialer{
				NetDialer: dialer,
				Config:    &tls.Config{ServerName: r.hostName, MinVersion: tls.VersionTLS12},
			}
			conn, err = tlsDialer.DialContext(ctx, "tcp", r.addr)
		} else {
			conn, err = dialer.DialContext(ctx, "tcp", r.addr)
		}
	}
	if err != nil {
		return err
	}
	r.conn = conn
	r.reader = bufio.NewReader(conn)

	if r.password != "" {
		var authErr error
		if r.username != "" {
			_, authErr = r.execLocked(ctx, "AUTH", r.username, r.password)
		} else {
			_, authErr = r.execLocked(ctx, "AUTH", r.password)
		}
		if authErr != nil {
			_ = r.closeConnLocked()
			return authErr
		}
	}
	if r.db > 0 {
		if _, err := r.execLocked(ctx, "SELECT", strconv.Itoa(r.db)); err != nil {
			_ = r.closeConnLocked()
			return err
		}
	}
	return nil
}

func (r *RedisStore) closeConnLocked() error {
	if r.conn == nil {
		return nil
	}
	err := r.conn.Close()
	r.conn = nil
	r.reader = nil
	return err
}

func (r *RedisStore) execLocked(ctx context.Context, args ...string) (any, error) {
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = r.conn.SetDeadline(deadline)

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(args)))
	for _, arg := range args {
		buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}
	if _, err := io.WriteString(r.conn, buf.String()); err != nil {
		return nil, err
	}
	return readRESP(r.reader)
}

func readRESP(rd *bufio.Reader) (any, error) {
	line, err := rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || !strings.HasSuffix(line, "\r\n") {
		return nil, fmt.Errorf("malformed RESP line %q", line)
	}
	prefix := line[0]
	body := line[1 : len(line)-2]

	switch prefix {
	case '+':
		return body, nil
	case '-':
		return nil, fmt.Errorf("redis error: %s", body)
	case ':':
		return strconv.ParseInt(body, 10, 64)
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(rd, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		items := make([]any, 0, n)
		for i := 0; i < n; i++ {
			item, err := readRESP(rd)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unknown RESP prefix %q", prefix)
	}
}
