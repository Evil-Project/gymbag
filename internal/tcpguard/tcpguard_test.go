package tcpguard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

type addrConn struct {
	net.Conn
	local  net.Addr
	remote net.Addr
}

func (c *addrConn) LocalAddr() net.Addr  { return c.local }
func (c *addrConn) RemoteAddr() net.Addr { return c.remote }

type stubIntel struct {
	threat map[string]bool
	vpn    map[string]bool
	ai     map[string]bool
}

func (s *stubIntel) IsThreatIP(_ context.Context, ip string) bool     { return s.threat[ip] }
func (s *stubIntel) IsVPNOrProxyIP(_ context.Context, ip string) bool { return s.vpn[ip] }
func (s *stubIntel) IsAIIP(ip string) bool                            { return s.ai[ip] }

func mustTCPAddr(ip string, port int) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

func readSSHBinaryPacket(r io.Reader) (msgType byte, reasonCode uint32, description string, err error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, 0, "", err
	}
	pktLen := binary.BigEndian.Uint32(hdr[0:4])
	padLen := int(hdr[4])
	if pktLen < 2 || int(pktLen-1) < padLen {
		return 0, 0, "", io.ErrUnexpectedEOF
	}
	rest := make([]byte, pktLen-1)
	if _, err := io.ReadFull(r, rest); err != nil {
		return 0, 0, "", err
	}
	payload := rest[:len(rest)-padLen]
	msgType = payload[0]
	if msgType == 1 && len(payload) >= 9 {
		reasonCode = binary.BigEndian.Uint32(payload[1:5])
		dlen := binary.BigEndian.Uint32(payload[5:9])
		if 9+int(dlen) <= len(payload) {
			description = string(payload[9 : 9+dlen])
		}
	}
	return msgType, reasonCode, description, nil
}

func buildMCHandshakePacket(proto int32, addr string, port uint16, nextState int32) []byte {
	var payload []byte
	payload = AppendVarInt(payload, 0x00)
	payload = AppendVarInt(payload, proto)
	payload = AppendVarInt(payload, int32(len(addr)))
	payload = append(payload, addr...)
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], port)
	payload = append(payload, pb[:]...)
	payload = AppendVarInt(payload, nextState)

	var frame []byte
	frame = AppendVarInt(frame, int32(len(payload)))
	frame = append(frame, payload...)
	return frame
}

func buildMCLoginStartPacket(username string) []byte {
	var payload []byte
	payload = AppendVarInt(payload, 0x00)
	payload = AppendVarInt(payload, int32(len(username)))
	payload = append(payload, username...)

	var frame []byte
	frame = AppendVarInt(frame, int32(len(payload)))
	frame = append(frame, payload...)
	return frame
}

func solvePortalVerifyLink(t *testing.T, portal *VerifyPortal, verifyURL string, browserIP string) {
	t.Helper()
	// Extract path starting from /v/
	idx := strings.Index(verifyURL, "/v/")
	if idx == -1 {
		t.Fatalf("verifyURL missing /v/: %q", verifyURL)
	}
	path := verifyURL[idx:]

	getReq := httptest.NewRequest(http.MethodGet, "https://verify.test"+path, nil)
	getReq.RemoteAddr = browserIP + ":44444"
	getRec := httptest.NewRecorder()
	portal.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", path, getRec.Code, getRec.Body.String())
	}

	m := regexp.MustCompile(`<script type="application/json" id="gymbag-gate-challenge">(.*?)</script>`).FindStringSubmatch(getRec.Body.String())
	if len(m) != 2 {
		t.Fatalf("missing gymbag-gate-challenge JSON in:\n%s", getRec.Body.String())
	}
	var spec struct {
		Token       string `json:"token"`
		ChallengeID string `json:"challenge_id"`
		Difficulty  int    `json:"difficulty"`
	}
	if err := json.Unmarshal([]byte(m[1]), &spec); err != nil {
		t.Fatal(err)
	}

	var nonce int64
	for !waf.Accepts(spec.ChallengeID, nonce, spec.Difficulty) {
		nonce++
	}
	body, _ := json.Marshal(map[string]any{
		"token":        spec.Token,
		"challenge_id": spec.ChallengeID,
		"nonce":        nonce,
		"verified":     true,
	})
	postReq := httptest.NewRequest(http.MethodPost, "https://verify.test/.waf/gate/verify", bytes.NewReader(body))
	postReq.RemoteAddr = browserIP + ":44444"
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	portal.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /.waf/gate/verify = %d: %s", postRec.Code, postRec.Body.String())
	}

	// Replay of same token must fail
	replayReq := httptest.NewRequest(http.MethodPost, "https://verify.test/.waf/gate/verify", bytes.NewReader(body))
	replayReq.RemoteAddr = browserIP + ":44444"
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	portal.ServeHTTP(replayRec, replayReq)
	if replayRec.Code == http.StatusOK {
		t.Fatalf("expected replay of single-use gate ticket to fail")
	}
}

func TestSSHProtectionFlowAndVerifyLevels(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	challenges := waf.NewChallengeService(st, 1, 300)
	portal := NewVerifyPortal(st, challenges, nil, nil)
	intelStub := &stubIntel{
		threat: map[string]bool{"198.51.100.99": true},
		vpn:    map[string]bool{"203.0.113.77": true},
		ai:     map[string]bool{"20.15.240.66": true},
	}

	sshCfg := config.DefaultSSHConfig()
	sshCfg.Enabled = true
	sshCfg.Upstream = "127.0.0.1:42022"
	sshCfg.Difficulty = 1
	sshCfg.VerifyLevel = config.VerifyLevelSuspiciousAndAI // 3

	proxy := NewSSHProxy(sshCfg, config.VerifyServerConfig{}, nil, "verify", st, st, intelStub, "198.51.100.10", nil)
	proxy.dialer = func(_ context.Context, _, _ string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			defer c2.Close()
			rd := bufio.NewReader(c2)
			line, _ := rd.ReadString('\n')
			_, _ = io.WriteString(c2, "SSH-2.0-OpenSSH_Upstream_Echo:"+strings.TrimSpace(line)+"\r\n")
		}()
		return c1, nil
	}

	runSSHAttempt := func(clientIP, clientBanner string) (isChallenge bool, output string) {
		cClient, cServer := net.Pipe()
		wrapped := &addrConn{
			Conn:   cServer,
			local:  mustTCPAddr("198.51.100.10", 22),
			remote: mustTCPAddr(clientIP, 50000),
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			proxy.HandleConn(ctx, wrapped)
		}()

		if clientBanner != "" {
			_, _ = io.WriteString(cClient, clientBanner+"\r\n")
		}
		rd := bufio.NewReader(cClient)
		srvBanner, err := rd.ReadString('\n')
		if err != nil {
			_ = cClient.Close()
			<-done
			return false, ""
		}
		if strings.HasPrefix(srvBanner, "SSH-2.0-Gymbag_Verify") {
			msgType, code, desc, pErr := readSSHBinaryPacket(rd)
			_ = cClient.Close()
			<-done
			if pErr != nil || msgType != 1 || code != sshDisconnectByApplication {
				t.Fatalf("unexpected SSH disconnect packet: msg=%d code=%d err=%v", msgType, code, pErr)
			}
			return true, desc
		}
		_ = cClient.Close()
		<-done
		return false, srvBanner
	}

	// 1. Level 3: Normal OpenSSH from clean IP passes straight to upstream!
	isChal, out := runSSHAttempt("203.0.113.5", "SSH-2.0-OpenSSH_9.6")
	if isChal || !strings.Contains(out, "OpenSSH_Upstream_Echo:SSH-2.0-OpenSSH_9.6") {
		t.Fatalf("clean OpenSSH should pass directly to upstream, got chal=%v out=%q", isChal, out)
	}

	// 2. Level 3: Scanner banner (paramiko) on clean IP is challenged with default IP:8443 verify link!
	isChal, out = runSSHAttempt("203.0.113.6", "SSH-2.0-paramiko_3.4")
	if !isChal || !strings.Contains(out, "https://198.51.100.10:8443/v/") {
		t.Fatalf("paramiko banner should be challenged with IP:8443 link, got chal=%v out=%q", isChal, out)
	}

	// 3. Level 3: Normal OpenSSH from VPN IP gets challenged -> solves verify link -> next attempt passes!
	isChal, out = runSSHAttempt("203.0.113.77", "SSH-2.0-OpenSSH_9.6")
	if !isChal {
		t.Fatalf("VPN IP should be challenged at level 3, got out=%q", out)
	}
	linkMatch := regexp.MustCompile(`https://198\.51\.100\.10:8443/v/[A-Za-z0-9_-]+`).FindString(out)
	if linkMatch == "" {
		t.Fatalf("expected verify link in SSH prompt, got:\n%s", out)
	}
	solvePortalVerifyLink(t, portal, linkMatch, "203.0.113.77")

	isChal, out = runSSHAttempt("203.0.113.77", "SSH-2.0-OpenSSH_9.6")
	if isChal || !strings.Contains(out, "OpenSSH_Upstream_Echo") {
		t.Fatalf("cleared VPN IP should now relay to upstream, got chal=%v out=%q", isChal, out)
	}

	// 4. Custom verify_url + port overrides default IP:8443
	sshCfg.VerifyLevel = config.VerifyLevelAlways // 0
	proxy.UpdateConfig(sshCfg, config.VerifyServerConfig{URL: "https://gate.example.com", Port: 9443}, nil, "block")
	isChal, out = runSSHAttempt("203.0.113.5", "SSH-2.0-OpenSSH_9.6")
	if !isChal || !strings.Contains(out, "https://gate.example.com:9443/v/") {
		t.Fatalf("expected custom verify URL in SSH prompt, got chal=%v out=%q", isChal, out)
	}

	// 5. Threat IP under ip_security_action=block is denied without verify link
	isChal, out = runSSHAttempt("198.51.100.99", "SSH-2.0-OpenSSH_9.6")
	if !isChal || !strings.Contains(out, "Connection blocked") || strings.Contains(out, "https://") {
		t.Fatalf("expected threat IP block disconnect, got chal=%v out=%q", isChal, out)
	}
}

func TestMinecraftProtectionFlowPingLoginAndVerifyLevels(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	challenges := waf.NewChallengeService(st, 1, 300)
	portal := NewVerifyPortal(st, challenges, nil, nil)
	intelStub := &stubIntel{
		vpn: map[string]bool{"198.51.100.88": true},
		ai:  map[string]bool{"20.15.240.66": true},
	}

	mcCfg := config.DefaultMinecraftConfig()
	mcCfg.Enabled = true
	mcCfg.Upstream = "127.0.0.1:45565"
	mcCfg.Difficulty = 1
	mcCfg.VerifyLevel = config.VerifyLevelOnlySuspicious // 1

	proxy := NewMinecraftProxy(mcCfg, config.VerifyServerConfig{}, nil, "verify", st, st, intelStub, "203.0.113.200", nil)
	proxy.dialer = func(_ context.Context, _, _ string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			defer c2.Close()
			rd := bufio.NewReader(c2)
			pid, payload, _, err := ReadMCPacket(rd, 4096)
			if err != nil || pid != 0x00 {
				return
			}
			_, rest, _ := readVarIntBytes(payload)
			_, rest, _ = ReadMCString(rest)
			nextState, _, _ := readVarIntBytes(rest[2:])
			if nextState == 1 {
				_, _ = io.WriteString(c2, `{"description":"MOTD_UPSTREAM_OK"}`)
				return
			}
			lpid, lpay, _, err := ReadMCPacket(rd, 4096)
			if err != nil || lpid != 0x00 {
				return
			}
			user, _, _ := ReadMCString(lpay)
			_, _ = io.WriteString(c2, "LOGIN_UPSTREAM_OK:"+user)
		}()
		return c1, nil
	}

	sendMCLogin := func(clientIP, username string, nextState int32) (disconnected bool, msg string) {
		cClient, cServer := net.Pipe()
		wrapped := &addrConn{
			Conn:   cServer,
			local:  mustTCPAddr("203.0.113.200", 25565),
			remote: mustTCPAddr(clientIP, 55000),
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			proxy.HandleConn(ctx, wrapped)
		}()

		_, _ = cClient.Write(buildMCHandshakePacket(765, "mc.example.com", 25565, nextState))
		if nextState == 2 {
			_, _ = cClient.Write(buildMCLoginStartPacket(username))
		}
		_ = cClient.SetReadDeadline(time.Now().Add(2 * time.Second))
		if nextState == 1 {
			b, _ := io.ReadAll(cClient)
			_ = cClient.Close()
			<-done
			return false, string(b)
		}

		// Try reading raw upstream marker OR MC Login Disconnect (0x00) frame
		rd := bufio.NewReader(cClient)
		peek, err := rd.Peek(5)
		if err != nil {
			_ = cClient.Close()
			<-done
			return false, ""
		}
		if strings.HasPrefix(string(peek), "LOGIN") {
			rest, _ := io.ReadAll(rd)
			_ = cClient.Close()
			<-done
			return false, string(rest)
		}
		pid, pay, _, err := ReadMCPacket(rd, 16384)
		_ = cClient.Close()
		<-done
		if err != nil || pid != 0x00 {
			t.Fatalf("expected Login Disconnect packet (0x00), pid=%d err=%v", pid, err)
		}
		chatJSON, _, err := ReadMCString(pay)
		if err != nil {
			t.Fatalf("failed reading Login Disconnect JSON string: %v", err)
		}
		return true, chatJSON
	}

	// 1. Status ping (Server List MOTD) passes transparently even from VPN IP!
	disc, motd := sendMCLogin("198.51.100.88", "", 1)
	if disc || !strings.Contains(motd, "MOTD_UPSTREAM_OK") {
		t.Fatalf("status ping should relay transparently, got disc=%v motd=%q", disc, motd)
	}

	// 2. Clean player at verify 1 passes straight to upstream MC server!
	disc, out := sendMCLogin("203.0.113.5", "Notch", 2)
	if disc || out != "LOGIN_UPSTREAM_OK:Notch" {
		t.Fatalf("clean MC login should relay, got disc=%v out=%q", disc, out)
	}

	// 3. Player connecting via VPN IP gets Login Disconnect ("Lost connection") with verify link!
	disc, chatJSON := sendMCLogin("198.51.100.88", "Alex", 2)
	if !disc || !strings.Contains(chatJSON, "https://203.0.113.200:8443/v/") || !strings.Contains(chatJSON, "Alex") {
		t.Fatalf("VPN MC login should show verify link in disconnect JSON, got disc=%v json=%s", disc, chatJSON)
	}
	linkMatch := regexp.MustCompile(`https://203\.0\.113\.200:8443/v/[A-Za-z0-9_-]+`).FindString(chatJSON)
	if linkMatch == "" {
		t.Fatalf("missing verify link in chat JSON: %s", chatJSON)
	}
	solvePortalVerifyLink(t, portal, linkMatch, "198.51.100.88")

	// 4. After solving PoW in browser, player reconnects and joins upstream!
	disc, out = sendMCLogin("198.51.100.88", "Alex", 2)
	if disc || out != "LOGIN_UPSTREAM_OK:Alex" {
		t.Fatalf("cleared MC player should join upstream, got disc=%v out=%q", disc, out)
	}

	// 5. VerifyLevelAlways (0) with custom verify URL
	mcCfg.VerifyLevel = config.VerifyLevelAlways
	proxy.UpdateConfig(mcCfg, config.VerifyServerConfig{URL: "https://mc-verify.example.com"}, nil, "verify")
	disc, chatJSON = sendMCLogin("203.0.113.5", "Steve", 2)
	if !disc || !strings.Contains(chatJSON, "https://mc-verify.example.com:8443/v/") {
		t.Fatalf("expected custom verify URL with default 8443 port, got disc=%v json=%s", disc, chatJSON)
	}
}
