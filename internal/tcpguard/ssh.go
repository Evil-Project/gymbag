package tcpguard

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

const sshDisconnectByApplication uint32 = 11

var suspiciousSSHBannerTokens = []string{
	"libssh",
	"paramiko",
	"zgrab",
	"masscan",
	"nmap",
	"asyncssh",
	"ssh2go",
	"jsch",
	"scanner",
	"bot",
	"python",
	"curl",
	"ssh-2.0-go",
	"hydra",
	"medusa",
	"patator",
}

// IsSuspiciousSSHBanner reports whether banner looks like a malformed probe, scanner, or scripted SSH library.
func IsSuspiciousSSHBanner(banner string) bool {
	trimmed := strings.TrimSpace(banner)
	if trimmed == "" || !strings.HasPrefix(trimmed, "SSH-") {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, tok := range suspiciousSSHBannerTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

// BuildSSHDisconnectPacket constructs an unencrypted RFC 4253 SSH_MSG_DISCONNECT binary packet.
func BuildSSHDisconnectPacket(reasonCode uint32, description string) []byte {
	desc := []byte(description)
	payloadLen := 1 + 4 + 4 + len(desc) + 4
	padLen := 8 - ((4 + 1 + payloadLen) % 8)
	if padLen < 4 {
		padLen += 8
	}
	pktLen := 1 + payloadLen + padLen
	buf := make([]byte, 4+pktLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(pktLen))
	buf[4] = byte(padLen)

	p := buf[5:]
	p[0] = 1 // SSH_MSG_DISCONNECT
	binary.BigEndian.PutUint32(p[1:5], reasonCode)
	binary.BigEndian.PutUint32(p[5:9], uint32(len(desc)))
	copy(p[9:], desc)
	binary.BigEndian.PutUint32(p[9+len(desc):13+len(desc)], 0) // empty language tag

	_, _ = rand.Read(p[payloadLen : payloadLen+padLen])
	return buf
}

// SSHProxy protects an SSH daemon by intercepting unverified connections before authentication
// and presenting a browser verification URL in the SSH console disconnect prompt.
type SSHProxy struct {
	mu               sync.RWMutex
	cfg              config.SSHConfig
	verifyCfg        config.VerifyServerConfig
	engine           *PolicyEngine
	gateStore        store.GateStore
	rateCounter      store.RateCounter
	intel            waf.IntelProvider
	detectedPublicIP string
	dialer           func(ctx context.Context, network, addr string) (net.Conn, error)
	logger           *slog.Logger
}

// NewSSHProxy constructs an SSHProxy wired to the shared store and threat/VPN/AI intelligence provider.
func NewSSHProxy(
	cfg config.SSHConfig,
	verifyCfg config.VerifyServerConfig,
	globalRules []config.CustomRule,
	ipSecurityAction string,
	rateCounter store.RateCounter,
	gateStore store.GateStore,
	intel waf.IntelProvider,
	detectedPublicIP string,
	logger *slog.Logger,
) *SSHProxy {
	if logger == nil {
		logger = slog.Default()
	}
	p := &SSHProxy{
		rateCounter:      rateCounter,
		gateStore:        gateStore,
		intel:            intel,
		detectedPublicIP: detectedPublicIP,
		dialer:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		logger:           logger,
	}
	p.UpdateConfig(cfg, verifyCfg, globalRules, ipSecurityAction)
	return p
}

// UpdateConfig atomically swaps the SSH protection policy and verify link settings.
func (p *SSHProxy) UpdateConfig(
	cfg config.SSHConfig,
	verifyCfg config.VerifyServerConfig,
	globalRules []config.CustomRule,
	ipSecurityAction string,
) {
	pol := ServicePolicy{
		Protect:      cfg.Protect,
		VerifyLevel:  cfg.VerifyLevel,
		Difficulty:   cfg.Difficulty,
		ChallengeTTL: cfg.ChallengeTTL,
		ClearanceTTL: cfg.ClearanceTTL,
		RateLimit:    cfg.RateLimit,
		RateWindow:   cfg.RateWindow,
		Rules:        cfg.Rules,
	}
	eng := NewPolicyEngine("ssh", pol, globalRules, p.rateCounter, p.gateStore, p.intel, ipSecurityAction)
	p.mu.Lock()
	p.cfg = cfg
	p.verifyCfg = verifyCfg
	p.engine = eng
	p.mu.Unlock()
}

// Serve accepts SSH connections on ln until ctx is cancelled or ln returns a non-temporary error.
func (p *SSHProxy) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go p.HandleConn(ctx, conn)
	}
}

// HandleConn evaluates an incoming SSH connection and either relays it transparently to upstream sshd
// or sends an SSH_MSG_DISCONNECT prompt containing the HTTPS verification link.
func (p *SSHProxy) HandleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	p.mu.RLock()
	cfg := p.cfg
	verifyCfg := p.verifyCfg
	eng := p.engine
	p.mu.RUnlock()

	clientIP := extractRemoteIP(conn.RemoteAddr())

	// Fast path: unprotected, or already cleared without custom rules -> zero-peek raw TCP relay.
	if allow, decided, _ := eng.IsFastAllow(ctx, clientIP); decided && allow {
		p.relayUpstream(ctx, conn, cfg.Upstream, nil)
		return
	}

	// Read client's SSH-2.0-... identification banner (RFC 4253 clients send immediately).
	peekTimeout := 1500 * time.Millisecond
	if cfg.VerifyLevel == config.VerifyLevelAlways && !eng.HasBannerRules() {
		peekTimeout = 250 * time.Millisecond
	}
	banner, rawBanner := readSSHBanner(conn, peekTimeout)

	// VerifyLevelOnlyAI without custom rules doesn't inspect SSH banners; otherwise inspect banner.
	protoSuspicious := false
	if cfg.VerifyLevel == config.VerifyLevelOnlySuspicious || cfg.VerifyLevel == config.VerifyLevelSuspiciousAndAI {
		protoSuspicious = IsSuspiciousSSHBanner(banner)
	}

	dec := eng.Evaluate(ctx, ConnInfo{
		Service:         "ssh",
		ClientIP:        clientIP,
		ClientBanner:    banner,
		ProtoSuspicious: protoSuspicious,
	})

	switch dec.Action {
	case waf.ActionAllow:
		p.relayUpstream(ctx, conn, cfg.Upstream, rawBanner)
		return
	case waf.ActionDeny:
		msg := "\n[Gymbag SSH Protection] Connection blocked.\n"
		if dec.Reason == "rate" {
			msg = "\n[Gymbag SSH Protection] Too many connection attempts. Please try again later.\n"
		}
		p.sendSSHDisconnect(conn, msg)
		return
	default: // ActionChallenge
		verifyURL, err := p.issueVerifyURL(ctx, cfg, verifyCfg, conn.LocalAddr(), clientIP)
		if err != nil {
			p.logger.Warn("failed to issue SSH verification link", "client_ip", clientIP, "error", err)
			p.sendSSHDisconnect(conn, "\n[Gymbag SSH Protection] Verification temporarily unavailable.\n")
			return
		}
		prompt := fmt.Sprintf(
			"\n[Gymbag SSH Protection] Verification required.\nOpen this link in your browser to unlock SSH access:\n  %s\nThen reconnect via SSH.\n",
			verifyURL,
		)
		p.sendSSHDisconnect(conn, prompt)
	}
}

func (p *SSHProxy) issueVerifyURL(
	ctx context.Context,
	cfg config.SSHConfig,
	verifyCfg config.VerifyServerConfig,
	localAddr net.Addr,
	clientIP string,
) (string, error) {
	if p.gateStore == nil {
		return "", errors.New("gate store unavailable")
	}
	token, err := generateShortVerifyToken()
	if err != nil {
		return "", err
	}
	diff := cfg.Difficulty
	if diff <= 0 {
		diff = 4
	}
	ttl := cfg.ChallengeTTL
	if ttl <= 0 {
		ttl = 300
	}
	clearTTL := cfg.ClearanceTTL
	if clearTTL <= 0 {
		clearTTL = 86400
	}
	ticket := store.GateTicket{
		Service:    "ssh",
		ClientIP:   clientIP,
		Difficulty: diff,
		ClearTTL:   clearTTL,
	}
	if err := p.gateStore.SaveGateTicket(ctx, token, ticket, ttl); err != nil {
		return "", err
	}
	base := ResolveVerifyBaseURL(verifyCfg, cfg.VerifyURL, cfg.VerifyPort, localAddr, clientIP, p.detectedPublicIP)
	return strings.TrimRight(base, "/") + "/v/" + token, nil
}

func (p *SSHProxy) sendSSHDisconnect(conn net.Conn, message string) {
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = io.WriteString(conn, "SSH-2.0-Gymbag_Verify\r\n")
	_, _ = conn.Write(BuildSSHDisconnectPacket(sshDisconnectByApplication, message))
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var drain [256]byte
	_, _ = conn.Read(drain[:])
}

func (p *SSHProxy) relayUpstream(ctx context.Context, client net.Conn, upstream string, preRead []byte) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	up, err := p.dialer(dialCtx, "tcp", upstream)
	cancel()
	if err != nil {
		p.logger.Warn("SSH upstream dial failed", "upstream", upstream, "error", err)
		return
	}
	defer up.Close()

	if len(preRead) > 0 {
		_ = up.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := up.Write(preRead); err != nil {
			return
		}
		_ = up.SetWriteDeadline(time.Time{})
	}
	relayTCP(ctx, client, up)
}

func readSSHBanner(conn net.Conn, timeout time.Duration) (banner string, raw []byte) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	buf := make([]byte, 0, 256)
	one := make([]byte, 1)
	for len(buf) < 512 {
		n, err := conn.Read(one)
		if n > 0 {
			buf = append(buf, one[0])
			if one[0] == '\n' {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if len(buf) == 0 {
		return "", nil
	}
	return strings.TrimRight(string(buf), "\r\n"), buf
}

func relayTCP(ctx context.Context, a, b net.Conn) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = a.Close()
			_ = b.Close()
		case <-done:
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	copySide := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}
	go copySide(b, a)
	go copySide(a, b)
	wg.Wait()
	close(done)
}
