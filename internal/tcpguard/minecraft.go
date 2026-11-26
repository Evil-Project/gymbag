package tcpguard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

// AppendVarInt appends an unsigned/signed 32-bit Minecraft VarInt to dst.
func AppendVarInt(dst []byte, v int32) []byte {
	u := uint32(v)
	for {
		b := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if u == 0 {
			return dst
		}
	}
}

// ReadVarInt reads a Minecraft VarInt (at most 5 bytes) returning value and raw encoded bytes.
func ReadVarInt(r io.Reader) (int32, []byte, error) {
	var val uint32
	var raw []byte
	var one [1]byte
	for shift := 0; shift < 35; shift += 7 {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return 0, raw, err
		}
		raw = append(raw, one[0])
		val |= uint32(one[0]&0x7F) << shift
		if one[0]&0x80 == 0 {
			return int32(val), raw, nil
		}
	}
	return 0, raw, errors.New("minecraft varint exceeds 5 bytes")
}

func readVarIntBytes(data []byte) (int32, []byte, error) {
	var val uint32
	for i := 0; i < len(data) && i < 5; i++ {
		b := data[i]
		val |= uint32(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return int32(val), data[i+1:], nil
		}
	}
	return 0, nil, errors.New("invalid varint in packet")
}

// ReadMCString parses a Minecraft VarInt-length-prefixed UTF-8 string.
func ReadMCString(data []byte) (string, []byte, error) {
	n, rest, err := readVarIntBytes(data)
	if err != nil {
		return "", nil, err
	}
	if n < 0 || int(n) > len(rest) {
		return "", nil, errors.New("minecraft string length out of bounds")
	}
	return string(rest[:n]), rest[n:], nil
}

// ReadMCPacket reads one uncompressed Minecraft frame (length + packetID + payload) and returns its exact raw bytes.
func ReadMCPacket(r io.Reader, maxLen int) (packetID int32, payload []byte, rawFrame []byte, err error) {
	pktLen, lenRaw, err := ReadVarInt(r)
	if err != nil {
		return 0, nil, nil, err
	}
	if pktLen <= 0 || int(pktLen) > maxLen {
		return 0, nil, nil, errors.New("invalid minecraft packet length")
	}
	body := make([]byte, pktLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, nil, err
	}
	pid, rest, err := readVarIntBytes(body)
	if err != nil {
		return 0, nil, nil, err
	}
	rawFrame = make([]byte, 0, len(lenRaw)+len(body))
	rawFrame = append(rawFrame, lenRaw...)
	rawFrame = append(rawFrame, body...)
	return pid, rest, rawFrame, nil
}

// BuildMCLoginDisconnectPacket encodes a Minecraft Login state Disconnect packet (Packet ID 0x00)
// containing a JSON Chat Component string, which renders in the client's "Connection Lost" dialog.
func BuildMCLoginDisconnectPacket(chatJSON string) []byte {
	var payload []byte
	payload = AppendVarInt(payload, 0x00)
	payload = AppendVarInt(payload, int32(len(chatJSON)))
	payload = append(payload, chatJSON...)

	var frame []byte
	frame = AppendVarInt(frame, int32(len(payload)))
	frame = append(frame, payload...)
	return frame
}

// BuildVerifyDisconnectJSON renders a clickable/readable Minecraft Chat Component JSON with the verification link
// using legacy 1.21.4-compatible clickEvent/hoverEvent formatting.
func BuildVerifyDisconnectJSON(verifyURL, username string) string {
	return BuildVerifyDisconnectJSONForProtocol(verifyURL, username, 0)
}

// BuildVerifyDisconnectJSONForProtocol renders a clickable Minecraft Chat Component JSON with the verification link,
// emitting 1.21.5+ (protocol >= 770) snake_case click_event/hover_event or legacy <= 1.21.4 clickEvent/hoverEvent.
func BuildVerifyDisconnectJSONForProtocol(verifyURL, username string, protoVer int32) string {
	greeting := "Verification required before joining.\n"
	if strings.TrimSpace(username) != "" {
		greeting = "Verification required for " + username + " before joining.\n"
	}
	linkComp := map[string]any{
		"text":       verifyURL,
		"color":      "aqua",
		"underlined": true,
	}
	if protoVer >= 770 {
		linkComp["click_event"] = map[string]any{
			"action": "copy_to_clipboard",
			"value":  verifyURL,
		}
		linkComp["hover_event"] = map[string]any{
			"action": "show_text",
			"value":  map[string]any{"text": "Click to copy verification link"},
		}
	} else {
		linkComp["clickEvent"] = map[string]any{
			"action": "copy_to_clipboard",
			"value":  verifyURL,
		}
		linkComp["hoverEvent"] = map[string]any{
			"action":   "show_text",
			"contents": map[string]any{"text": "Click to copy verification link"},
		}
	}
	comp := map[string]any{
		"text": "",
		"extra": []map[string]any{
			{"text": "Gymbag Protection\n\n", "color": "gold", "bold": true},
			{"text": greeting, "color": "white"},
			{"text": "Open this link in your browser:\n\n", "color": "gray"},
			linkComp,
			{"text": "\n\nThen click Reconnect / Join Server.", "color": "gray"},
		},
	}
	b, _ := json.Marshal(comp)
	return string(b)
}

// BuildBlockDisconnectJSON renders a block/rate-limit Minecraft Disconnect JSON message.
func BuildBlockDisconnectJSON(reason string) string {
	msg := "Connection blocked by Gymbag protection."
	if reason == "rate" {
		msg = "Too many connection attempts. Please wait a moment and try again."
	}
	comp := map[string]any{
		"text": "",
		"extra": []map[string]any{
			{"text": "Gymbag Protection\n\n", "color": "red", "bold": true},
			{"text": msg, "color": "white"},
		},
	}
	b, _ := json.Marshal(comp)
	return string(b)
}

func isValidMCUsername(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

const (
	defaultMaxLimboSessions int32 = 256
	pendingTransferTTL            = 30 * time.Second
)

// MinecraftProxy protects a Minecraft Java Edition server by transparently forwarding Server List Pings
// and gating unverified logins behind an in-memory empty-world verification limbo (1.20.5+ with automatic
// Transfer redirect) or a disconnect verification prompt for older clients.
type MinecraftProxy struct {
	mu               sync.RWMutex
	cfg              config.MinecraftConfig
	verifyCfg        config.VerifyServerConfig
	engine           *PolicyEngine
	gateStore        store.GateStore
	rateCounter      store.RateCounter
	intel            waf.IntelProvider
	detectedPublicIP string
	dialer           func(ctx context.Context, network, addr string) (net.Conn, error)
	logger           *slog.Logger

	limboMu          sync.Mutex
	limboWaiters     map[string]map[chan struct{}]struct{}
	pendingTransfers map[string]time.Time
	activeLimbo      atomic.Int32
	maxLimboSessions int32
}

// NewMinecraftProxy constructs a MinecraftProxy wired to the shared store and intelligence manager.
func NewMinecraftProxy(
	cfg config.MinecraftConfig,
	verifyCfg config.VerifyServerConfig,
	globalRules []config.CustomRule,
	ipSecurityAction string,
	rateCounter store.RateCounter,
	gateStore store.GateStore,
	intel waf.IntelProvider,
	detectedPublicIP string,
	logger *slog.Logger,
) *MinecraftProxy {
	if logger == nil {
		logger = slog.Default()
	}
	p := &MinecraftProxy{
		rateCounter:      rateCounter,
		gateStore:        gateStore,
		intel:            intel,
		detectedPublicIP: detectedPublicIP,
		dialer:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		logger:           logger,
		maxLimboSessions: defaultMaxLimboSessions,
	}
	p.UpdateConfig(cfg, verifyCfg, globalRules, ipSecurityAction)
	return p
}

// NotifyCleared wakes any active limbo sessions waiting for clientIP to complete verification.
func (p *MinecraftProxy) NotifyCleared(clientIP string) {
	p.limboMu.Lock()
	waiters := p.limboWaiters[clientIP]
	if len(waiters) > 0 {
		delete(p.limboWaiters, clientIP)
	}
	p.limboMu.Unlock()

	for ch := range waiters {
		close(ch)
	}
}

func (p *MinecraftProxy) subscribeClearance(clientIP string) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	p.limboMu.Lock()
	if p.limboWaiters == nil {
		p.limboWaiters = make(map[string]map[chan struct{}]struct{})
	}
	set := p.limboWaiters[clientIP]
	if set == nil {
		set = make(map[chan struct{}]struct{})
		p.limboWaiters[clientIP] = set
	}
	set[ch] = struct{}{}
	p.limboMu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			p.limboMu.Lock()
			if cur := p.limboWaiters[clientIP]; cur != nil {
				delete(cur, ch)
				if len(cur) == 0 {
					delete(p.limboWaiters, clientIP)
				}
			}
			p.limboMu.Unlock()
		})
	}
	return ch, unsub
}

func (p *MinecraftProxy) markPendingTransfer(clientIP string) {
	now := time.Now()
	p.limboMu.Lock()
	if p.pendingTransfers == nil {
		p.pendingTransfers = make(map[string]time.Time)
	}
	for k, exp := range p.pendingTransfers {
		if !exp.After(now) {
			delete(p.pendingTransfers, k)
		}
	}
	p.pendingTransfers[clientIP] = now.Add(pendingTransferTTL)
	p.limboMu.Unlock()
}

func (p *MinecraftProxy) hasPendingTransfer(clientIP string) bool {
	now := time.Now()
	p.limboMu.Lock()
	defer p.limboMu.Unlock()
	exp, ok := p.pendingTransfers[clientIP]
	if !ok {
		return false
	}
	if !exp.After(now) {
		delete(p.pendingTransfers, clientIP)
		return false
	}
	return true
}

func (p *MinecraftProxy) consumePendingTransfer(clientIP string) bool {
	now := time.Now()
	p.limboMu.Lock()
	defer p.limboMu.Unlock()
	exp, ok := p.pendingTransfers[clientIP]
	if !ok {
		return false
	}
	delete(p.pendingTransfers, clientIP)
	return exp.After(now)
}

func rewriteMCHandshakePacket(protoVer int32, serverAddr string, serverPort uint16, nextState int32) []byte {
	var payload []byte
	payload = AppendVarInt(payload, 0x00)
	payload = AppendVarInt(payload, protoVer)
	payload = AppendVarInt(payload, int32(len(serverAddr)))
	payload = append(payload, serverAddr...)
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], serverPort)
	payload = append(payload, pb[:]...)
	payload = AppendVarInt(payload, nextState)
	var frame []byte
	frame = AppendVarInt(frame, int32(len(payload)))
	frame = append(frame, payload...)
	return frame
}

// UpdateConfig atomically swaps the Minecraft protection policy and verify link settings.
func (p *MinecraftProxy) UpdateConfig(
	cfg config.MinecraftConfig,
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
	eng := NewPolicyEngine("minecraft", pol, globalRules, p.rateCounter, p.gateStore, p.intel, ipSecurityAction)
	p.mu.Lock()
	p.cfg = cfg
	p.verifyCfg = verifyCfg
	p.engine = eng
	p.mu.Unlock()
}

// Serve accepts Minecraft client connections on ln until ctx is cancelled or ln closes.
func (p *MinecraftProxy) Serve(ctx context.Context, ln net.Listener) error {
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

// HandleConn inspects the Minecraft Java handshake and login packets before relaying to upstream.
func (p *MinecraftProxy) HandleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	p.mu.RLock()
	cfg := p.cfg
	verifyCfg := p.verifyCfg
	eng := p.engine
	p.mu.RUnlock()

	clientIP := extractRemoteIP(conn.RemoteAddr())
	pendingTransfer := p.hasPendingTransfer(clientIP)

	// Fast path: unprotected, or already cleared without custom rules -> transparent relay.
	// Skip fast path when a limbo Transfer reconnect is pending so nextState=3 can be rewritten to 2.
	if !pendingTransfer {
		if allow, decided, _ := eng.IsFastAllow(ctx, clientIP); decided && allow {
			p.relayUpstream(ctx, conn, cfg.Upstream, nil)
			return
		}
	}

	br := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	peekByte, peekErr := br.Peek(1)
	if peekErr != nil {
		return
	}

	// Legacy Minecraft 1.6 Server List Ping (0xFE): passthrough unless hard-blocked.
	if peekByte[0] == 0xFE {
		_ = conn.SetReadDeadline(time.Time{})
		dec := eng.Evaluate(ctx, ConnInfo{Service: "minecraft", ClientIP: clientIP})
		if dec.Action == waf.ActionDeny {
			return
		}
		p.relayBuffered(ctx, conn, br, cfg.Upstream, nil)
		return
	}

	// Modern Handshake packet (Packet ID 0x00)
	hsPID, hsPayload, rawHS, err := ReadMCPacket(br, 4096)
	if err != nil || hsPID != 0x00 {
		_ = conn.SetReadDeadline(time.Time{})
		if cfg.VerifyLevel == config.VerifyLevelOnlySuspicious || cfg.VerifyLevel == config.VerifyLevelSuspiciousAndAI || cfg.VerifyLevel == config.VerifyLevelAlways {
			if verifyURL, vErr := p.issueVerifyURL(ctx, cfg, verifyCfg, conn.LocalAddr(), clientIP, ""); vErr == nil {
				p.sendDisconnect(conn, BuildVerifyDisconnectJSON(verifyURL, ""))
			}
		}
		return
	}

	protoVer, rest, err := readVarIntBytes(hsPayload)
	if err != nil {
		return
	}
	serverAddr, rest, err := ReadMCString(rest)
	if err != nil || len(rest) < 2 {
		return
	}
	serverPort := binary.BigEndian.Uint16(rest[:2])
	nextState, _, err := readVarIntBytes(rest[2:])
	if err != nil {
		return
	}

	// Status / Server List Ping (nextState == 1): let players view real MOTD & player counts
	// unless IP is hard-denied by block rules or threat-intel block mode.
	if nextState == 1 {
		_ = conn.SetReadDeadline(time.Time{})
		dec := eng.Evaluate(ctx, ConnInfo{
			Service:      "minecraft",
			ClientIP:     clientIP,
			ClientBanner: serverAddr,
		})
		if dec.Action == waf.ActionDeny {
			return
		}
		p.relayBuffered(ctx, conn, br, cfg.Upstream, rawHS)
		return
	}

	// Many upstream Minecraft servers run with accepts-transfers=false and reject nextState=3.
	// Rewrite a limbo Transfer reconnect (nextState=3 -> 2) so Login succeeds cleanly upstream.
	if nextState == 3 && p.consumePendingTransfer(clientIP) {
		nextState = 2
		rawHS = rewriteMCHandshakePacket(protoVer, serverAddr, serverPort, 2)
	}
	if pendingTransfer && nextState == 2 {
		if allow, decided, _ := eng.IsFastAllow(ctx, clientIP); decided && allow {
			_ = conn.SetReadDeadline(time.Time{})
			p.relayBuffered(ctx, conn, br, cfg.Upstream, rawHS)
			return
		}
	}

	// Login (nextState == 2) or Transfer (nextState == 3)
	var rawLogin []byte
	var username string
	var playerUUID [16]byte
	protoSuspicious := protoVer <= 0
	if nextState == 2 || nextState == 3 {
		lPID, lPayload, lRaw, lErr := ReadMCPacket(br, 4096)
		_ = conn.SetReadDeadline(time.Time{})
		if lErr != nil || lPID != 0x00 {
			protoSuspicious = true
		} else {
			rawLogin = lRaw
			if u, restAfterUser, uErr := ReadMCString(lPayload); uErr != nil || !isValidMCUsername(u) {
				protoSuspicious = true
			} else {
				username = u
				if len(restAfterUser) >= 16 {
					copy(playerUUID[:], restAfterUser[:16])
				} else {
					playerUUID = offlineUUID(username)
				}
			}
		}
	} else {
		_ = conn.SetReadDeadline(time.Time{})
		protoSuspicious = true
	}

	dec := eng.Evaluate(ctx, ConnInfo{
		Service:         "minecraft",
		ClientIP:        clientIP,
		ClientBanner:    serverAddr,
		Username:        username,
		ProtoSuspicious: protoSuspicious,
	})

	switch dec.Action {
	case waf.ActionAllow:
		var preRead bytes.Buffer
		preRead.Write(rawHS)
		preRead.Write(rawLogin)
		p.relayBuffered(ctx, conn, br, cfg.Upstream, preRead.Bytes())
		return
	case waf.ActionDeny:
		p.sendDisconnect(conn, BuildBlockDisconnectJSON(dec.Reason))
		return
	default: // ActionChallenge -> enter empty limbo world (1.20.5+) or disconnect with verify link.
		verifyURL, err := p.issueVerifyURL(ctx, cfg, verifyCfg, conn.LocalAddr(), clientIP, username)
		if err != nil {
			p.logger.Warn("failed to issue Minecraft verification link", "client_ip", clientIP, "error", err)
			p.sendDisconnect(conn, BuildBlockDisconnectJSON("error"))
			return
		}
		if !protoSuspicious && supportsLimbo(protoVer) {
			if p.activeLimbo.Add(1) <= p.maxLimboSessions {
				defer p.activeLimbo.Add(-1)
				p.serveLimbo(ctx, conn, br, cfg, protoVer, serverAddr, serverPort, username, playerUUID, clientIP, verifyURL)
				return
			}
			p.activeLimbo.Add(-1)
		}
		p.sendDisconnect(conn, BuildVerifyDisconnectJSONForProtocol(verifyURL, username, protoVer))
	}
}

func (p *MinecraftProxy) issueVerifyURL(
	ctx context.Context,
	cfg config.MinecraftConfig,
	verifyCfg config.VerifyServerConfig,
	localAddr net.Addr,
	clientIP, username string,
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
		Service:    "minecraft",
		ClientIP:   clientIP,
		Username:   username,
		Difficulty: diff,
		ClearTTL:   clearTTL,
	}
	if err := p.gateStore.SaveGateTicket(ctx, token, ticket, ttl); err != nil {
		return "", err
	}
	base := ResolveVerifyBaseURL(verifyCfg, cfg.VerifyURL, cfg.VerifyPort, localAddr, clientIP, p.detectedPublicIP)
	return strings.TrimRight(base, "/") + "/v/" + token, nil
}

func (p *MinecraftProxy) sendDisconnect(conn net.Conn, chatJSON string) {
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Write(BuildMCLoginDisconnectPacket(chatJSON))
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var drain [256]byte
	_, _ = conn.Read(drain[:])
}

func (p *MinecraftProxy) relayUpstream(ctx context.Context, client net.Conn, upstream string, preRead []byte) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	up, err := p.dialer(dialCtx, "tcp", upstream)
	cancel()
	if err != nil {
		p.logger.Warn("Minecraft upstream dial failed", "upstream", upstream, "error", err)
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

func (p *MinecraftProxy) relayBuffered(ctx context.Context, client net.Conn, br *bufio.Reader, upstream string, preRead []byte) {
	var buffered []byte
	if n := br.Buffered(); n > 0 {
		buffered, _ = br.Peek(n)
		_, _ = br.Discard(n)
	}
	if len(buffered) > 0 {
		combined := make([]byte, 0, len(preRead)+len(buffered))
		combined = append(combined, preRead...)
		combined = append(combined, buffered...)
		p.relayUpstream(ctx, client, upstream, combined)
		return
	}
	p.relayUpstream(ctx, client, upstream, preRead)
}
