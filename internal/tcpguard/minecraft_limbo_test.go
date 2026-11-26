package tcpguard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

func buildMCLoginStartWithUUID(username string, uuid [16]byte) []byte {
	var payload []byte
	payload = AppendVarInt(payload, 0x00)
	payload = appendMCString(payload, username)
	payload = append(payload, uuid[:]...)
	var frame []byte
	frame = AppendVarInt(frame, int32(len(payload)))
	frame = append(frame, payload...)
	return frame
}

func buildMCPlayerPosPacket(pid int32, x, y, z float64, protoVer int32) []byte {
	var payload []byte
	payload = appendF64(payload, x)
	payload = appendF64(payload, y)
	payload = appendF64(payload, z)
	if protoVer >= 768 {
		payload = append(payload, 0x01) // MovementFlags
	} else {
		payload = append(payload, 0x01) // onGround
	}
	return buildMCPacket(pid, payload)
}

func buildMCServerKnownPacksPacket(versions []string) []byte {
	var payload []byte
	payload = AppendVarInt(payload, int32(len(versions)))
	for _, v := range versions {
		payload = appendMCString(payload, "minecraft")
		payload = appendMCString(payload, "core")
		payload = appendMCString(payload, v)
	}
	return buildMCPacket(mcCfgKnownPacksSPacketID, payload)
}

func readSyncPositionCoords(payload []byte, spec mcLimboProto) (x, y, z float64, teleportID int32, err error) {
	rest := payload
	if spec.posNewFormat {
		teleportID, rest, err = readVarIntBytes(rest)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if len(rest) < 24 {
			return 0, 0, 0, 0, io.ErrUnexpectedEOF
		}
		x = math.Float64frombits(binary.BigEndian.Uint64(rest[0:8]))
		y = math.Float64frombits(binary.BigEndian.Uint64(rest[8:16]))
		z = math.Float64frombits(binary.BigEndian.Uint64(rest[16:24]))
		return x, y, z, teleportID, nil
	}
	if len(rest) < 33 {
		return 0, 0, 0, 0, io.ErrUnexpectedEOF
	}
	x = math.Float64frombits(binary.BigEndian.Uint64(rest[0:8]))
	y = math.Float64frombits(binary.BigEndian.Uint64(rest[8:16]))
	z = math.Float64frombits(binary.BigEndian.Uint64(rest[16:24]))
	teleportID, _, err = readVarIntBytes(rest[33:])
	return x, y, z, teleportID, err
}

func assertChunkPacketValid(t *testing.T, payload []byte, spec mcLimboProto) (cx, cz int32) {
	t.Helper()
	if len(payload) < 8 {
		t.Fatalf("chunk packet too short: %d", len(payload))
	}
	cx = int32(binary.BigEndian.Uint32(payload[0:4]))
	cz = int32(binary.BigEndian.Uint32(payload[4:8]))
	rest := payload[8:]

	if spec.chunkHeightmapsPrefixed {
		count, r2, err := readVarIntBytes(rest)
		if err != nil || count != 1 {
			t.Fatalf("expected 1 prefixed heightmap, count=%d err=%v", count, err)
		}
		hmType, r2, err := readVarIntBytes(r2)
		if err != nil || hmType != 4 {
			t.Fatalf("expected MOTION_BLOCKING (4), got %d err=%v", hmType, err)
		}
		longLen, r2, err := readVarIntBytes(r2)
		if err != nil || longLen != 37 || len(r2) < 37*8 {
			t.Fatalf("expected 37 heightmap longs, got %d len=%d err=%v", longLen, len(r2), err)
		}
		rest = r2[37*8:]
	} else {
		nbtPrefix := encodeNBTEmptyHeightmaps()
		if !bytes.HasPrefix(rest, nbtPrefix) {
			t.Fatalf("expected NBT heightmap prefix on proto <=769")
		}
		rest = rest[len(nbtPrefix):]
	}

	dataLen, rest, err := readVarIntBytes(rest)
	if err != nil || int(dataLen) > len(rest) || dataLen <= 0 {
		t.Fatalf("invalid chunkData length %d (remaining %d) err=%v", dataLen, len(rest), err)
	}
	chunkBuf := rest[:dataLen]
	rest = rest[dataLen:]

	// Verify section 7 blockCount == 25 on (0,0) and 0 elsewhere.
	cur := chunkBuf
	for s := 0; s < limboSections; s++ {
		if len(cur) < 2 {
			t.Fatalf("section %d truncated", s)
		}
		blockCount := int16(binary.BigEndian.Uint16(cur[:2]))
		cur = cur[2:]
		if spec.sectionHasFluidCount {
			if len(cur) < 2 {
				t.Fatalf("section %d fluidCount truncated", s)
			}
			cur = cur[2:]
		}
		wantBlocks := int16(0)
		if cx == 0 && cz == 0 && s == 7 {
			wantBlocks = 25
		}
		if blockCount != wantBlocks {
			t.Fatalf("chunk (%d,%d) section %d blockCount=%d, want %d", cx, cz, s, blockCount, wantBlocks)
		}

		// Parse blockStates paletted container
		bpe := cur[0]
		cur = cur[1:]
		if bpe == 0 {
			_, cur, err = readVarIntBytes(cur)
			if err != nil {
				t.Fatalf("section %d single block palette err=%v", s, err)
			}
			if !spec.palettedNoDataLen {
				dl, c2, err := readVarIntBytes(cur)
				if err != nil || dl != 0 {
					t.Fatalf("section %d expected 0 data len, got %d err=%v", s, dl, err)
				}
				cur = c2
			}
		} else {
			plen, c2, err := readVarIntBytes(cur)
			if err != nil || plen != 2 {
				t.Fatalf("section %d expected palette len 2, got %d err=%v", s, plen, err)
			}
			cur = c2
			for i := 0; i < int(plen); i++ {
				_, cur, err = readVarIntBytes(cur)
				if err != nil {
					t.Fatalf("section %d palette entry err=%v", s, err)
				}
			}
			if !spec.palettedNoDataLen {
				dl, c2, err := readVarIntBytes(cur)
				if err != nil || dl != 256 {
					t.Fatalf("section %d expected 256 longs, got %d err=%v", s, dl, err)
				}
				cur = c2
			}
			if len(cur) < 256*8 {
				t.Fatalf("section %d truncated longs", s)
			}
			cur = cur[256*8:]
		}

		// Parse biomes single-valued paletted container
		if cur[0] != 0 {
			t.Fatalf("section %d expected single-valued biome", s)
		}
		cur = cur[1:]
		_, cur, err = readVarIntBytes(cur)
		if err != nil {
			t.Fatalf("section %d biome val err=%v", s, err)
		}
		if !spec.palettedNoDataLen {
			dl, c2, err := readVarIntBytes(cur)
			if err != nil || dl != 0 {
				t.Fatalf("section %d expected biome data len 0, got %d err=%v", s, dl, err)
			}
			cur = c2
		}
	}
	if len(cur) != 0 {
		t.Fatalf("chunk (%d,%d) had %d trailing bytes in chunkData", cx, cz, len(cur))
	}

	beCount, rest, err := readVarIntBytes(rest)
	if err != nil || beCount != 0 {
		t.Fatalf("expected 0 block entities, got %d err=%v", beCount, err)
	}
	skyMaskLen, _, err := readVarIntBytes(rest)
	if err != nil {
		t.Fatalf("light skyMaskLen err=%v", err)
	}
	if cx == 0 && cz == 0 && skyMaskLen != 1 {
		t.Fatalf("expected skyLight mask on (0,0), got %d", skyMaskLen)
	}
	return cx, cz
}

func newTestLimboEnv(t *testing.T, portal *VerifyPortal, upstreamFn func(net.Conn)) (*MinecraftProxy, *store.MemoryStore) {
	t.Helper()
	st := store.NewMemoryStore()
	challenges := waf.NewChallengeService(st, 1, 300)
	if portal == nil {
		portal = NewVerifyPortal(st, challenges, nil, nil)
	}

	mcCfg := config.DefaultMinecraftConfig()
	mcCfg.Enabled = true
	mcCfg.Port = 25565
	mcCfg.Upstream = "127.0.0.1:45565"
	mcCfg.Difficulty = 1
	mcCfg.VerifyLevel = config.VerifyLevelAlways

	proxy := NewMinecraftProxy(mcCfg, config.VerifyServerConfig{}, nil, "verify", st, st, nil, "203.0.113.200", nil)
	if portal != nil {
		portal.SetOnCleared(func(service, clientIP string) {
			if service == "minecraft" {
				proxy.NotifyCleared(clientIP)
			}
		})
	}
	proxy.dialer = func(_ context.Context, _, _ string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			defer c2.Close()
			if upstreamFn != nil {
				upstreamFn(c2)
				return
			}
			rd := bufio.NewReader(c2)
			pid, payload, _, err := ReadMCPacket(rd, 4096)
			if err != nil || pid != 0x00 {
				return
			}
			_, rest, _ := readVarIntBytes(payload)
			_, rest, _ = ReadMCString(rest)
			nextState, _, _ := readVarIntBytes(rest[2:])
			lpid, lpay, _, err := ReadMCPacket(rd, 4096)
			if err != nil || lpid != 0x00 {
				return
			}
			user, _, _ := ReadMCString(lpay)
			_, _ = io.WriteString(c2, fmt.Sprintf("UPSTREAM_OK:ns=%d:user=%s", nextState, user))
		}()
		return c1, nil
	}
	return proxy, st
}

func TestMinecraftLimboEndToEndAndTransferRedirect(t *testing.T) {
	// Ensure zero world files are ever saved to disk.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	versions := []struct {
		name  string
		proto int32
	}{
		{"1.20.5", 766},
		{"1.21.1", 767},
		{"1.21.3", 768},
		{"1.21.4", 769},
		{"1.21.5", 770},
		{"1.21.6", 771},
		{"1.21.9", 773},
		{"1.21.11", 774},
		{"26.1", 775},
	}

	for _, tc := range versions {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			st := store.NewMemoryStore()
			challenges := waf.NewChallengeService(st, 1, 300)
			portal := NewVerifyPortal(st, challenges, nil, nil)
			proxy, _ := newTestLimboEnv(t, nil, nil)
			proxy.gateStore = st
			proxy.rateCounter = st
			proxy.UpdateConfig(proxy.cfg, proxy.verifyCfg, nil, "verify")
			portal.SetOnCleared(func(service, clientIP string) {
				if service == "minecraft" {
					proxy.NotifyCleared(clientIP)
				}
			})

			spec, ok := lookupLimboProto(tc.proto)
			if !ok {
				t.Fatalf("missing proto %d", tc.proto)
			}

			clientIP := "198.51.100.42"
			cClient, cServer := net.Pipe()
			wrapped := &addrConn{
				Conn:   cServer,
				local:  mustTCPAddr("203.0.113.200", 25565),
				remote: mustTCPAddr(clientIP, 55123),
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				proxy.HandleConn(ctx, wrapped)
			}()

			// Handshake + Login Start
			_, _ = cClient.Write(buildMCHandshakePacket(tc.proto, "mc.example.com", 25565, 2))
			_, _ = cClient.Write(buildMCLoginStartWithUUID("LimboPlayer", offlineUUID("LimboPlayer")))

			rd := bufio.NewReader(cClient)
			_ = cClient.SetReadDeadline(time.Now().Add(3 * time.Second))

			// 1. Login Success
			pid, pay, _, err := ReadMCPacket(rd, 65536)
			if err != nil || pid != mcLoginSuccessPacketID {
				t.Fatalf("expected Login Success (0x02), got pid=0x%x err=%v", pid, err)
			}
			if len(pay) < 16 {
				t.Fatalf("login success payload too short: %d", len(pay))
			}
			uname, rest, err := ReadMCString(pay[16:])
			if err != nil || uname != "LimboPlayer" {
				t.Fatalf("login success username=%q err=%v", uname, err)
			}
			propCount, rest, err := readVarIntBytes(rest)
			if err != nil || propCount != 0 {
				t.Fatalf("expected 0 properties, got %d err=%v", propCount, err)
			}
			if spec.loginSuccessHasStrictErr {
				if len(rest) != 1 || rest[0] != 0 {
					t.Fatalf("expected strictErrorHandling=false byte on proto %d, got %x", tc.proto, rest)
				}
			} else if len(rest) != 0 {
				t.Fatalf("unexpected trailing bytes in Login Success on proto %d: %x", tc.proto, rest)
			}

			// 2. Login Acknowledged -> Configuration
			_, _ = cClient.Write(buildMCPacket(mcLoginAckPacketID, nil))

			// Expect Select Known Packs
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != mcCfgKnownPacksCPacketID {
				t.Fatalf("expected Select Known Packs (0x0E), got pid=0x%x err=%v", pid, err)
			}
			if !parseServerKnownPacksHasCore(pay) {
				t.Fatalf("server known packs missing minecraft:core")
			}

			// Simulate client sending Client Information (0x00) before Known Packs (0x07)
			_, _ = cClient.Write(buildMCPacket(0x00, []byte{0x05, 'e', 'n', '_', 'u', 's'}))
			_, _ = cClient.Write(buildMCServerKnownPacksPacket(spec.knownPackVersions[:1]))

			// Expect Registry Data packets followed by Finish Configuration
			seenRegistries := make(map[string]bool)
			for {
				pid, pay, _, err = ReadMCPacket(rd, 65536)
				if err != nil {
					t.Fatalf("reading config packets: %v", err)
				}
				if pid == mcCfgFinishCPacketID {
					break
				}
				if pid != mcCfgRegistryPacketID {
					t.Fatalf("unexpected config packet pid=0x%x", pid)
				}
				regID, regRest, err := ReadMCString(pay)
				if err != nil {
					t.Fatal(err)
				}
				seenRegistries[regID] = true
				entryCount, regRest, err := readVarIntBytes(regRest)
				if err != nil || entryCount <= 0 {
					t.Fatalf("registry %s invalid entryCount=%d err=%v", regID, entryCount, err)
				}
				for i := int32(0); i < entryCount; i++ {
					_, regRest, err = ReadMCString(regRest)
					if err != nil || len(regRest) < 1 || regRest[0] != 0x00 {
						t.Fatalf("registry %s expected hasData=false, err=%v", regID, err)
					}
					regRest = regRest[1:]
				}
			}
			if !seenRegistries["minecraft:dimension_type"] || !seenRegistries["minecraft:worldgen/biome"] {
				t.Fatalf("missing core registries: %v", seenRegistries)
			}
			if tc.proto >= 771 && !seenRegistries["minecraft:dialog"] {
				t.Fatalf("proto %d missing minecraft:dialog registry", tc.proto)
			}
			if tc.proto >= 774 && !seenRegistries["minecraft:timeline"] {
				t.Fatalf("proto %d missing minecraft:timeline registry", tc.proto)
			}
			if tc.proto >= 775 && !seenRegistries["minecraft:world_clock"] {
				t.Fatalf("proto %d missing minecraft:world_clock registry", tc.proto)
			}

			// 3. Acknowledge Finish Configuration -> Play
			_, _ = cClient.Write(buildMCPacket(mcCfgFinishSPacketID, nil))

			// Expect Play Login, Abilities, Game Event (13), Set Center Chunk, 9 Chunks, Sync Pos, Verify Chat, Action Bar
			pid, _, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playLogin {
				t.Fatalf("expected Play Login (0x%x), got 0x%x err=%v", spec.playLogin, pid, err)
			}
			pid, _, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playAbilities {
				t.Fatalf("expected Abilities (0x%x), got 0x%x err=%v", spec.playAbilities, pid, err)
			}
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playGameEvent || len(pay) < 5 || pay[0] != 13 {
				t.Fatalf("expected Game Event 13 (0x%x), got 0x%x err=%v", spec.playGameEvent, pid, err)
			}
			pid, _, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playCenterChunk {
				t.Fatalf("expected Set Center Chunk (0x%x), got 0x%x err=%v", spec.playCenterChunk, pid, err)
			}

			seenCenterChunk := false
			for i := 0; i < 9; i++ {
				pid, pay, _, err = ReadMCPacket(rd, 65536)
				if err != nil || pid != spec.playChunk {
					t.Fatalf("expected Chunk #%d (0x%x), got 0x%x err=%v", i, spec.playChunk, pid, err)
				}
				cx, cz := assertChunkPacketValid(t, pay, spec)
				if cx == 0 && cz == 0 {
					seenCenterChunk = true
				}
			}
			if !seenCenterChunk {
				t.Fatalf("missing center chunk (0,0)")
			}

			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playPosC {
				t.Fatalf("expected Sync Pos (0x%x), got 0x%x err=%v", spec.playPosC, pid, err)
			}
			x, y, z, tpID, err := readSyncPositionCoords(pay, spec)
			if err != nil || x != limboSpawnX || y != limboSpawnY || z != limboSpawnZ || tpID != 1 {
				t.Fatalf("unexpected spawn coords (%v,%v,%v) tp=%d err=%v", x, y, z, tpID, err)
			}

			// Verify chat link
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playSystemChat {
				t.Fatalf("expected Verify System Chat (0x%x), got 0x%x err=%v", spec.playSystemChat, pid, err)
			}
			if pay[len(pay)-1] != 0 {
				t.Fatalf("expected overlay=false on chat link")
			}
			if spec.modernTextEvents {
				if !bytes.Contains(pay, []byte("click_event")) || bytes.Contains(pay, []byte("clickEvent")) {
					t.Fatalf("expected modern click_event NBT on proto %d", tc.proto)
				}
			} else {
				if !bytes.Contains(pay, []byte("clickEvent")) || bytes.Contains(pay, []byte("click_event")) {
					t.Fatalf("expected legacy clickEvent NBT on proto %d", tc.proto)
				}
			}
			linkMatch := regexp.MustCompile(`https://203\.0\.113\.200:8443/v/[A-Za-z0-9]+`).Find(pay)
			if len(linkMatch) == 0 {
				t.Fatalf("missing verify URL in system chat NBT")
			}
			verifyURL := string(linkMatch)

			// Action bar hint
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playSystemChat || pay[len(pay)-1] != 1 {
				t.Fatalf("expected Action Bar System Chat (overlay=1), got 0x%x err=%v", pid, err)
			}

			// 4. Void fall rescue: send y=30.0 -> server teleports back to spawn
			_, _ = cClient.Write(buildMCPlayerPosPacket(spec.playPosS, 8.5, 30.0, 8.5, tc.proto))
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playPosC {
				t.Fatalf("expected void-rescue Sync Pos, got 0x%x err=%v", pid, err)
			}
			_, y, _, tpID, err = readSyncPositionCoords(pay, spec)
			if err != nil || y != limboSpawnY || tpID != 2 {
				t.Fatalf("expected void rescue y=%v tp=2, got y=%v tp=%d err=%v", limboSpawnY, y, tpID, err)
			}

			// 5. Chat re-prompt: send chat message -> server re-sends verify link
			_, _ = cClient.Write(buildMCPacket(spec.playChatS, appendMCString(nil, "help")))
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playSystemChat || !bytes.Contains(pay, []byte(verifyURL)) {
				t.Fatalf("expected re-sent verify chat link, got pid=0x%x err=%v", pid, err)
			}

			// 6. Complete browser verification -> instant completion chat + Transfer packet!
			solvePortalVerifyLink(t, portal, verifyURL, clientIP)

			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playSystemChat || !bytes.Contains(pay, []byte("Redirecting you to the server")) {
				t.Fatalf("expected completion system chat, got pid=0x%x err=%v", pid, err)
			}
			pid, pay, _, err = ReadMCPacket(rd, 65536)
			if err != nil || pid != spec.playTransfer {
				t.Fatalf("expected Transfer packet (0x%x), got 0x%x err=%v", spec.playTransfer, pid, err)
			}
			trHost, trRest, err := ReadMCString(pay)
			if err != nil {
				t.Fatal(err)
			}
			trPort, _, err := readVarIntBytes(trRest)
			if err != nil || trHost != "mc.example.com" || trPort != 25565 {
				t.Fatalf("unexpected Transfer target (%q, %d) err=%v", trHost, trPort, err)
			}

			_ = cClient.Close()
			<-done

			// 7. Client follows Transfer by connecting with nextState=3 -> proxy rewrites to 2 for upstream!
			cClient2, cServer2 := net.Pipe()
			wrapped2 := &addrConn{
				Conn:   cServer2,
				local:  mustTCPAddr("203.0.113.200", 25565),
				remote: mustTCPAddr(clientIP, 55124),
			}
			done2 := make(chan struct{})
			go func() {
				defer close(done2)
				proxy.HandleConn(ctx, wrapped2)
			}()
			_, _ = cClient2.Write(buildMCHandshakePacket(tc.proto, "mc.example.com", 25565, 3))
			_, _ = cClient2.Write(buildMCLoginStartWithUUID("LimboPlayer", offlineUUID("LimboPlayer")))
			_ = cClient2.SetReadDeadline(time.Now().Add(2 * time.Second))
			upOut, _ := io.ReadAll(cClient2)
			_ = cClient2.Close()
			<-done2

			if string(upOut) != "UPSTREAM_OK:ns=2:user=LimboPlayer" {
				t.Fatalf("expected upstream to receive rewritten nextState=2, got %q", string(upOut))
			}
		})
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected zero disk files in working directory, found %v", entries)
	}
}

func TestMinecraftLimboTransferSanitizesForgeSuffix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := store.NewMemoryStore()
	challenges := waf.NewChallengeService(st, 1, 300)
	portal := NewVerifyPortal(st, challenges, nil, nil)
	proxy, _ := newTestLimboEnv(t, nil, nil)
	proxy.gateStore = st
	proxy.rateCounter = st
	proxy.UpdateConfig(proxy.cfg, proxy.verifyCfg, nil, "verify")
	portal.SetOnCleared(func(service, clientIP string) {
		if service == "minecraft" {
			proxy.NotifyCleared(clientIP)
		}
	})

	spec, _ := lookupLimboProto(770)
	clientIP := "198.51.100.77"
	cClient, cServer := net.Pipe()
	wrapped := &addrConn{
		Conn:   cServer,
		local:  mustTCPAddr("203.0.113.200", 25565),
		remote: mustTCPAddr(clientIP, 55001),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		proxy.HandleConn(ctx, wrapped)
	}()

	_, _ = cClient.Write(buildMCHandshakePacket(770, "play.example.com\x00FML3\x00", 25565, 2))
	_, _ = cClient.Write(buildMCLoginStartWithUUID("ForgeUser", offlineUUID("ForgeUser")))

	rd := bufio.NewReader(cClient)
	_ = cClient.SetReadDeadline(time.Now().Add(3 * time.Second))

	_, _, _, _ = ReadMCPacket(rd, 65536) // Login Success
	_, _ = cClient.Write(buildMCPacket(mcLoginAckPacketID, nil))
	_, _, _, _ = ReadMCPacket(rd, 65536) // Known Packs
	_, _ = cClient.Write(buildMCServerKnownPacksPacket(spec.knownPackVersions))
	for {
		pid, _, _, err := ReadMCPacket(rd, 65536)
		if err != nil {
			t.Fatal(err)
		}
		if pid == mcCfgFinishCPacketID {
			break
		}
	}
	_, _ = cClient.Write(buildMCPacket(mcCfgFinishSPacketID, nil))

	var verifyURL string
	for i := 0; i < 15; i++ {
		pid, pay, _, err := ReadMCPacket(rd, 65536)
		if err != nil {
			t.Fatal(err)
		}
		if pid == spec.playSystemChat {
			if m := regexp.MustCompile(`https://203\.0\.113\.200:8443/v/[A-Za-z0-9]+`).Find(pay); len(m) > 0 {
				verifyURL = string(m)
				break
			}
		}
	}
	if verifyURL == "" {
		t.Fatalf("did not receive verify URL")
	}
	// Drain action bar
	_, _, _, _ = ReadMCPacket(rd, 65536)

	solvePortalVerifyLink(t, portal, verifyURL, clientIP)

	_, _, _, _ = ReadMCPacket(rd, 65536) // completion chat
	pid, pay, _, err := ReadMCPacket(rd, 65536)
	if err != nil || pid != spec.playTransfer {
		t.Fatalf("expected Transfer packet, got pid=0x%x err=%v", pid, err)
	}
	host, _, err := ReadMCString(pay)
	if err != nil || host != "play.example.com" {
		t.Fatalf("expected sanitized Transfer host play.example.com, got %q err=%v", host, err)
	}
	_ = cClient.Close()
	<-done
}

func TestMinecraftLimboFallbacks(t *testing.T) {
	ctx := context.Background()

	t.Run("unsupported_versions_disconnect", func(t *testing.T) {
		for _, proto := range []int32{765, 999} {
			proxy, _ := newTestLimboEnv(t, nil, nil)
			cClient, cServer := net.Pipe()
			wrapped := &addrConn{
				Conn:   cServer,
				local:  mustTCPAddr("203.0.113.200", 25565),
				remote: mustTCPAddr("198.51.100.10", 55000),
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				proxy.HandleConn(ctx, wrapped)
			}()
			_, _ = cClient.Write(buildMCHandshakePacket(proto, "mc.example.com", 25565, 2))
			_, _ = cClient.Write(buildMCLoginStartPacket("LegacyPlayer"))

			_ = cClient.SetReadDeadline(time.Now().Add(2 * time.Second))
			pid, pay, _, err := ReadMCPacket(bufio.NewReader(cClient), 16384)
			_ = cClient.Close()
			<-done
			if err != nil || pid != 0x00 {
				t.Fatalf("proto %d expected Login Disconnect (0x00), got pid=0x%x err=%v", proto, pid, err)
			}
			msg, _, err := ReadMCString(pay)
			if err != nil || !strings.Contains(msg, "https://203.0.113.200:8443/v/") {
				t.Fatalf("proto %d missing verify URL in disconnect: %q err=%v", proto, msg, err)
			}
		}
	})

	t.Run("rejected_known_packs_config_disconnect", func(t *testing.T) {
		proxy, _ := newTestLimboEnv(t, nil, nil)
		cClient, cServer := net.Pipe()
		wrapped := &addrConn{
			Conn:   cServer,
			local:  mustTCPAddr("203.0.113.200", 25565),
			remote: mustTCPAddr("198.51.100.11", 55000),
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			proxy.HandleConn(ctx, wrapped)
		}()
		_, _ = cClient.Write(buildMCHandshakePacket(770, "mc.example.com", 25565, 2))
		_, _ = cClient.Write(buildMCLoginStartWithUUID("NoCore", offlineUUID("NoCore")))

		rd := bufio.NewReader(cClient)
		_ = cClient.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, _, _ = ReadMCPacket(rd, 65536) // Login Success
		_, _ = cClient.Write(buildMCPacket(mcLoginAckPacketID, nil))
		_, _, _, _ = ReadMCPacket(rd, 65536) // Known Packs
		_, _ = cClient.Write(buildMCServerKnownPacksPacket(nil))

		pid, pay, _, err := ReadMCPacket(rd, 16384)
		_ = cClient.Close()
		<-done
		if err != nil || pid != mcCfgDisconnectPacketID {
			t.Fatalf("expected Config Disconnect (0x02), got pid=0x%x err=%v", pid, err)
		}
		if !bytes.Contains(pay, []byte("https://203.0.113.200:8443/v/")) {
			t.Fatalf("expected verify URL in config disconnect NBT")
		}
	})

	t.Run("limbo_cap_falls_back_to_disconnect", func(t *testing.T) {
		proxy, _ := newTestLimboEnv(t, nil, nil)
		proxy.maxLimboSessions = 0
		cClient, cServer := net.Pipe()
		wrapped := &addrConn{
			Conn:   cServer,
			local:  mustTCPAddr("203.0.113.200", 25565),
			remote: mustTCPAddr("198.51.100.12", 55000),
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			proxy.HandleConn(ctx, wrapped)
		}()
		_, _ = cClient.Write(buildMCHandshakePacket(770, "mc.example.com", 25565, 2))
		_, _ = cClient.Write(buildMCLoginStartPacket("CappedUser"))

		_ = cClient.SetReadDeadline(time.Now().Add(2 * time.Second))
		pid, pay, _, err := ReadMCPacket(bufio.NewReader(cClient), 16384)
		_ = cClient.Close()
		<-done
		if err != nil || pid != 0x00 {
			t.Fatalf("expected Login Disconnect (0x00) when capped, got pid=0x%x err=%v", pid, err)
		}
		msg, _, err := ReadMCString(pay)
		if err != nil || !strings.Contains(msg, "https://203.0.113.200:8443/v/") {
			t.Fatalf("expected verify URL when capped, got %q err=%v", msg, err)
		}
	})
}

func TestMinecraftLimboPollingClearanceWithoutCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No SetOnCleared callback wired -> verifies 500ms polling catches external/Redis clearances.
	proxy, st := newTestLimboEnv(t, nil, nil)
	spec, _ := lookupLimboProto(770)
	clientIP := "198.51.100.55"

	cClient, cServer := net.Pipe()
	wrapped := &addrConn{
		Conn:   cServer,
		local:  mustTCPAddr("203.0.113.200", 25565),
		remote: mustTCPAddr(clientIP, 55002),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		proxy.HandleConn(ctx, wrapped)
	}()

	_, _ = cClient.Write(buildMCHandshakePacket(770, "mc.example.com", 25565, 2))
	_, _ = cClient.Write(buildMCLoginStartWithUUID("PollUser", offlineUUID("PollUser")))

	rd := bufio.NewReader(cClient)
	_ = cClient.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _, _ = ReadMCPacket(rd, 65536) // Login Success
	_, _ = cClient.Write(buildMCPacket(mcLoginAckPacketID, nil))
	_, _, _, _ = ReadMCPacket(rd, 65536) // Known Packs
	_, _ = cClient.Write(buildMCServerKnownPacksPacket(spec.knownPackVersions))
	for {
		pid, _, _, err := ReadMCPacket(rd, 65536)
		if err != nil {
			t.Fatal(err)
		}
		if pid == mcCfgFinishCPacketID {
			break
		}
	}
	_, _ = cClient.Write(buildMCPacket(mcCfgFinishSPacketID, nil))

	// Drain initial Play burst (16 packets: Login, Abilities, GameEvent, CenterChunk, 9 Chunks, SyncPos, VerifyChat, ActionBar)
	for i := 0; i < 16; i++ {
		if _, _, _, err := ReadMCPacket(rd, 65536); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate another replica persisting clearance directly in the store.
	if err := st.SaveClearance(ctx, "minecraft", clientIP, 3600); err != nil {
		t.Fatal(err)
	}

	var sawCompleteChat, sawTransfer bool
	for i := 0; i < 5; i++ {
		pid, pay, _, err := ReadMCPacket(rd, 65536)
		if err != nil {
			t.Fatal(err)
		}
		if pid == spec.playSystemChat && bytes.Contains(pay, []byte("Redirecting you to the server")) {
			sawCompleteChat = true
		}
		if pid == spec.playTransfer {
			sawTransfer = true
			break
		}
	}
	if !sawCompleteChat || !sawTransfer {
		t.Fatalf("expected polled completion chat (%v) and Transfer packet (%v)", sawCompleteChat, sawTransfer)
	}
	_ = cClient.Close()
	<-done
}
