package tcpguard

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/binary"
	"math"
	"net"
	"strings"
	"time"

	"gymbag/internal/config"
)

const (
	limboSpawnX = 8.5
	limboSpawnY = 64.0
	limboSpawnZ = 8.5

	limboMinY     = -64
	limboSections = 24 // 384 / 16 (overworld -64..319)

	limboActionBarText = "Open chat (T) and click the verification link"
)

// mcLimboProto holds exact per-protocol-version packet IDs and wire layout flags
// for Minecraft Java Edition 1.20.5–26.1 (protocol versions 766–775).
type mcLimboProto struct {
	knownPackVersions        []string
	loginSuccessHasStrictErr bool
	loginHasSeaLevel         bool
	posNewFormat             bool
	chunkHeightmapsPrefixed  bool
	palettedNoDataLen        bool
	sectionHasFluidCount     bool
	modernTextEvents         bool

	// Clientbound Play packet IDs
	playDisconnect  int32
	playGameEvent   int32
	playKeepAliveC  int32
	playChunk       int32
	playLogin       int32
	playAbilities   int32
	playPosC        int32
	playCenterChunk int32
	playSystemChat  int32
	playTransfer    int32

	// Serverbound Play packet IDs
	playKeepAliveS int32
	playChatS      int32
	playPosS       int32
	playPosRotS    int32
}

const (
	mcLoginSuccessPacketID = 0x02
	mcLoginAckPacketID     = 0x03

	mcCfgDisconnectPacketID  = 0x02
	mcCfgFinishCPacketID     = 0x03
	mcCfgKeepAliveCPacketID  = 0x04
	mcCfgRegistryPacketID    = 0x07
	mcCfgKnownPacksCPacketID = 0x0E

	mcCfgFinishSPacketID     = 0x03
	mcCfgKeepAliveSPacketID  = 0x04
	mcCfgKnownPacksSPacketID = 0x07
)

var limboProtoTable = map[int32]mcLimboProto{
	766: { // 1.20.5 / 1.20.6
		knownPackVersions:        []string{"1.20.5", "1.20.6"},
		loginSuccessHasStrictErr: true,
		playDisconnect:           0x1D,
		playGameEvent:            0x22,
		playKeepAliveC:           0x26,
		playChunk:                0x27,
		playLogin:                0x2B,
		playAbilities:            0x38,
		playPosC:                 0x40,
		playCenterChunk:          0x54,
		playSystemChat:           0x6C,
		playTransfer:             0x73,
		playKeepAliveS:           0x18,
		playChatS:                0x06,
		playPosS:                 0x1A,
		playPosRotS:              0x1B,
	},
	767: { // 1.21 / 1.21.1
		knownPackVersions:        []string{"1.21", "1.21.1"},
		loginSuccessHasStrictErr: true,
		playDisconnect:           0x1D,
		playGameEvent:            0x22,
		playKeepAliveC:           0x26,
		playChunk:                0x27,
		playLogin:                0x2B,
		playAbilities:            0x38,
		playPosC:                 0x40,
		playCenterChunk:          0x54,
		playSystemChat:           0x6C,
		playTransfer:             0x73,
		playKeepAliveS:           0x18,
		playChatS:                0x06,
		playPosS:                 0x1A,
		playPosRotS:              0x1B,
	},
	768: { // 1.21.2 / 1.21.3
		knownPackVersions: []string{"1.21.2", "1.21.3"},
		loginHasSeaLevel:  true,
		posNewFormat:      true,
		playDisconnect:    0x1D,
		playGameEvent:     0x23,
		playKeepAliveC:    0x27,
		playChunk:         0x28,
		playLogin:         0x2C,
		playAbilities:     0x3A,
		playPosC:          0x42,
		playCenterChunk:   0x58,
		playSystemChat:    0x73,
		playTransfer:      0x7A,
		playKeepAliveS:    0x1A,
		playChatS:         0x07,
		playPosS:          0x1C,
		playPosRotS:       0x1D,
	},
	769: { // 1.21.4
		knownPackVersions: []string{"1.21.4"},
		loginHasSeaLevel:  true,
		posNewFormat:      true,
		playDisconnect:    0x1D,
		playGameEvent:     0x23,
		playKeepAliveC:    0x27,
		playChunk:         0x28,
		playLogin:         0x2C,
		playAbilities:     0x3A,
		playPosC:          0x42,
		playCenterChunk:   0x58,
		playSystemChat:    0x73,
		playTransfer:      0x7A,
		playKeepAliveS:    0x1A,
		playChatS:         0x07,
		playPosS:          0x1C,
		playPosRotS:       0x1D,
	},
	770: { // 1.21.5
		knownPackVersions:       []string{"1.21.5"},
		loginHasSeaLevel:        true,
		posNewFormat:            true,
		chunkHeightmapsPrefixed: true,
		palettedNoDataLen:       true,
		modernTextEvents:        true,
		playDisconnect:          0x1C,
		playGameEvent:           0x22,
		playKeepAliveC:          0x26,
		playChunk:               0x27,
		playLogin:               0x2B,
		playAbilities:           0x39,
		playPosC:                0x41,
		playCenterChunk:         0x57,
		playSystemChat:          0x72,
		playTransfer:            0x7A,
		playKeepAliveS:          0x1A,
		playChatS:               0x07,
		playPosS:                0x1C,
		playPosRotS:             0x1D,
	},
	771: { // 1.21.6
		knownPackVersions:       []string{"1.21.6"},
		loginHasSeaLevel:        true,
		posNewFormat:            true,
		chunkHeightmapsPrefixed: true,
		palettedNoDataLen:       true,
		modernTextEvents:        true,
		playDisconnect:          0x1C,
		playGameEvent:           0x22,
		playKeepAliveC:          0x26,
		playChunk:               0x27,
		playLogin:               0x2B,
		playAbilities:           0x39,
		playPosC:                0x41,
		playCenterChunk:         0x57,
		playSystemChat:          0x72,
		playTransfer:            0x7A,
		playKeepAliveS:          0x1B,
		playChatS:               0x08,
		playPosS:                0x1D,
		playPosRotS:             0x1E,
	},
	772: { // 1.21.7 / 1.21.8
		knownPackVersions:       []string{"1.21.7", "1.21.8"},
		loginHasSeaLevel:        true,
		posNewFormat:            true,
		chunkHeightmapsPrefixed: true,
		palettedNoDataLen:       true,
		modernTextEvents:        true,
		playDisconnect:          0x1C,
		playGameEvent:           0x22,
		playKeepAliveC:          0x26,
		playChunk:               0x27,
		playLogin:               0x2B,
		playAbilities:           0x39,
		playPosC:                0x41,
		playCenterChunk:         0x57,
		playSystemChat:          0x72,
		playTransfer:            0x7A,
		playKeepAliveS:          0x1B,
		playChatS:               0x08,
		playPosS:                0x1D,
		playPosRotS:             0x1E,
	},
	773: { // 1.21.9 / 1.21.10
		knownPackVersions:       []string{"1.21.9", "1.21.10"},
		loginHasSeaLevel:        true,
		posNewFormat:            true,
		chunkHeightmapsPrefixed: true,
		palettedNoDataLen:       true,
		modernTextEvents:        true,
		playDisconnect:          0x20,
		playGameEvent:           0x26,
		playKeepAliveC:          0x2B,
		playChunk:               0x2C,
		playLogin:               0x30,
		playAbilities:           0x3E,
		playPosC:                0x46,
		playCenterChunk:         0x5C,
		playSystemChat:          0x77,
		playTransfer:            0x7F,
		playKeepAliveS:          0x1B,
		playChatS:               0x08,
		playPosS:                0x1D,
		playPosRotS:             0x1E,
	},
	774: { // 1.21.11
		knownPackVersions:       []string{"1.21.11"},
		loginHasSeaLevel:        true,
		posNewFormat:            true,
		chunkHeightmapsPrefixed: true,
		palettedNoDataLen:       true,
		modernTextEvents:        true,
		playDisconnect:          0x20,
		playGameEvent:           0x26,
		playKeepAliveC:          0x2B,
		playChunk:               0x2C,
		playLogin:               0x30,
		playAbilities:           0x3E,
		playPosC:                0x46,
		playCenterChunk:         0x5C,
		playSystemChat:          0x77,
		playTransfer:            0x7F,
		playKeepAliveS:          0x1B,
		playChatS:               0x08,
		playPosS:                0x1D,
		playPosRotS:             0x1E,
	},
	775: { // 26.1 / 26.1.1 / 26.1.2
		knownPackVersions:       []string{"26.1", "26.1.1", "26.1.2"},
		loginHasSeaLevel:        true,
		posNewFormat:            true,
		chunkHeightmapsPrefixed: true,
		palettedNoDataLen:       true,
		sectionHasFluidCount:    true,
		modernTextEvents:        true,
		playDisconnect:          0x20,
		playGameEvent:           0x26,
		playKeepAliveC:          0x2C,
		playChunk:               0x2D,
		playLogin:               0x31,
		playAbilities:           0x40,
		playPosC:                0x48,
		playCenterChunk:         0x5E,
		playSystemChat:          0x79,
		playTransfer:            0x81,
		playKeepAliveS:          0x1C,
		playChatS:               0x09,
		playPosS:                0x1E,
		playPosRotS:             0x1F,
	},
}

func lookupLimboProto(protoVer int32) (mcLimboProto, bool) {
	spec, ok := limboProtoTable[protoVer]
	return spec, ok
}

func supportsLimbo(protoVer int32) bool {
	_, ok := lookupLimboProto(protoVer)
	return ok
}

// --- Binary & Network NBT helpers ---

func buildMCPacket(packetID int32, payload []byte) []byte {
	var hdr [10]byte
	pidBytes := AppendVarInt(hdr[:0], packetID)
	bodyLen := len(pidBytes) + len(payload)

	var lenBuf [10]byte
	lenBytes := AppendVarInt(lenBuf[:0], int32(bodyLen))

	frame := make([]byte, 0, len(lenBytes)+bodyLen)
	frame = append(frame, lenBytes...)
	frame = append(frame, pidBytes...)
	frame = append(frame, payload...)
	return frame
}

func appendMCString(dst []byte, s string) []byte {
	dst = AppendVarInt(dst, int32(len(s)))
	return append(dst, s...)
}

func appendU16(dst []byte, v uint16) []byte {
	return binary.BigEndian.AppendUint16(dst, v)
}

func appendU32(dst []byte, v uint32) []byte {
	return binary.BigEndian.AppendUint32(dst, v)
}

func appendU64(dst []byte, v uint64) []byte {
	return binary.BigEndian.AppendUint64(dst, v)
}

func appendF32(dst []byte, v float32) []byte {
	return appendU32(dst, math.Float32bits(v))
}

func appendF64(dst []byte, v float64) []byte {
	return appendU64(dst, math.Float64bits(v))
}

const (
	nbtTagEnd       byte = 0x00
	nbtTagByte      byte = 0x01
	nbtTagString    byte = 0x08
	nbtTagList      byte = 0x09
	nbtTagCompound  byte = 0x0A
	nbtTagLongArray byte = 0x0C
)

func nbtWriteName(dst []byte, name string) []byte {
	dst = appendU16(dst, uint16(len(name)))
	return append(dst, name...)
}

func nbtWriteStringPayload(dst []byte, s string) []byte {
	dst = appendU16(dst, uint16(len(s)))
	return append(dst, s...)
}

func nbtFieldByte(dst []byte, name string, v byte) []byte {
	dst = append(dst, nbtTagByte)
	dst = nbtWriteName(dst, name)
	return append(dst, v)
}

func nbtFieldString(dst []byte, name, v string) []byte {
	dst = append(dst, nbtTagString)
	dst = nbtWriteName(dst, name)
	return nbtWriteStringPayload(dst, v)
}

func nbtStartCompoundField(dst []byte, name string) []byte {
	dst = append(dst, nbtTagCompound)
	return nbtWriteName(dst, name)
}

func nbtStartRootCompound(dst []byte) []byte {
	// Network NBT (1.20.2+) omits the root compound's name.
	return append(dst, nbtTagCompound)
}

func nbtEndCompound(dst []byte) []byte {
	return append(dst, nbtTagEnd)
}

func encodeNBTSimpleText(text, color string, bold bool) []byte {
	var b []byte
	b = nbtStartRootCompound(b)
	b = nbtFieldString(b, "text", text)
	if color != "" {
		b = nbtFieldString(b, "color", color)
	}
	if bold {
		b = nbtFieldByte(b, "bold", 1)
	}
	b = nbtEndCompound(b)
	return b
}

func encodeNBTVerifyChat(verifyURL, username string, modernEvents bool) []byte {
	greeting := "Verification required before joining.\n"
	if strings.TrimSpace(username) != "" {
		greeting = "Verification required for " + username + " before joining.\n"
	}

	var b []byte
	b = nbtStartRootCompound(b)
	b = nbtFieldString(b, "text", "")

	// extra: TAG_List of TAG_Compound (5 items)
	b = append(b, nbtTagList)
	b = nbtWriteName(b, "extra")
	b = append(b, nbtTagCompound)
	b = appendU32(b, 5)

	// 1. Header
	b = nbtFieldString(b, "text", "[Gymbag] ")
	b = nbtFieldString(b, "color", "gold")
	b = nbtFieldByte(b, "bold", 1)
	b = nbtEndCompound(b)

	// 2. Greeting
	b = nbtFieldString(b, "text", greeting)
	b = nbtFieldString(b, "color", "white")
	b = nbtEndCompound(b)

	// 3. Prompt
	b = nbtFieldString(b, "text", "Click to verify in your browser:\n")
	b = nbtFieldString(b, "color", "gray")
	b = nbtEndCompound(b)

	// 4. Clickable link
	b = nbtFieldString(b, "text", verifyURL)
	b = nbtFieldString(b, "color", "aqua")
	b = nbtFieldByte(b, "underlined", 1)
	if modernEvents {
		b = nbtStartCompoundField(b, "click_event")
		b = nbtFieldString(b, "action", "open_url")
		b = nbtFieldString(b, "url", verifyURL)
		b = nbtEndCompound(b)

		b = nbtStartCompoundField(b, "hover_event")
		b = nbtFieldString(b, "action", "show_text")
		b = nbtStartCompoundField(b, "value")
		b = nbtFieldString(b, "text", "Click to open verification page")
		b = nbtEndCompound(b)
		b = nbtEndCompound(b)
	} else {
		b = nbtStartCompoundField(b, "clickEvent")
		b = nbtFieldString(b, "action", "open_url")
		b = nbtFieldString(b, "value", verifyURL)
		b = nbtEndCompound(b)

		b = nbtStartCompoundField(b, "hoverEvent")
		b = nbtFieldString(b, "action", "show_text")
		b = nbtStartCompoundField(b, "contents")
		b = nbtFieldString(b, "text", "Click to open verification page")
		b = nbtEndCompound(b)
		b = nbtEndCompound(b)
	}
	b = nbtEndCompound(b)

	// 5. Footer
	b = nbtFieldString(b, "text", "\nYou will be redirected to the server automatically once verified.")
	b = nbtFieldString(b, "color", "gray")
	b = nbtEndCompound(b)

	b = nbtEndCompound(b)
	return b
}

func encodeNBTVerifyCompleteChat() []byte {
	var b []byte
	b = nbtStartRootCompound(b)
	b = nbtFieldString(b, "text", "")

	b = append(b, nbtTagList)
	b = nbtWriteName(b, "extra")
	b = append(b, nbtTagCompound)
	b = appendU32(b, 2)

	b = nbtFieldString(b, "text", "[Gymbag] ")
	b = nbtFieldString(b, "color", "gold")
	b = nbtFieldByte(b, "bold", 1)
	b = nbtEndCompound(b)

	b = nbtFieldString(b, "text", "Verification complete! Redirecting you to the server...")
	b = nbtFieldString(b, "color", "green")
	b = nbtEndCompound(b)

	b = nbtEndCompound(b)
	return b
}

func encodeNBTEmptyHeightmaps() []byte {
	var b []byte
	b = nbtStartRootCompound(b)
	b = append(b, nbtTagLongArray)
	b = nbtWriteName(b, "MOTION_BLOCKING")
	b = appendU32(b, 37)
	for i := 0; i < 37; i++ {
		b = appendU64(b, 0)
	}
	b = nbtEndCompound(b)
	return b
}

// --- Packet builders ---

func offlineUUID(username string) [16]byte {
	sum := md5.Sum([]byte("OfflinePlayer:" + username))
	sum[6] = (sum[6] & 0x0F) | 0x30 // Version 3
	sum[8] = (sum[8] & 0x3F) | 0x80 // RFC 4122 variant
	return sum
}

func buildMCLoginSuccessPacket(spec mcLimboProto, uuid [16]byte, username string) []byte {
	var p []byte
	p = append(p, uuid[:]...)
	p = appendMCString(p, username)
	p = AppendVarInt(p, 0) // properties count
	if spec.loginSuccessHasStrictErr {
		p = append(p, 0) // strictErrorHandling = false
	}
	return buildMCPacket(mcLoginSuccessPacketID, p)
}

func buildMCKnownPacksPacket(versions []string) []byte {
	var p []byte
	p = AppendVarInt(p, int32(len(versions)))
	for _, v := range versions {
		p = appendMCString(p, "minecraft")
		p = appendMCString(p, "core")
		p = appendMCString(p, v)
	}
	return buildMCPacket(mcCfgKnownPacksCPacketID, p)
}

func parseServerKnownPacksHasCore(payload []byte) bool {
	count, rest, err := readVarIntBytes(payload)
	if err != nil || count <= 0 || count > 64 {
		return false
	}
	for i := int32(0); i < count; i++ {
		var ns, id string
		ns, rest, err = ReadMCString(rest)
		if err != nil {
			return false
		}
		id, rest, err = ReadMCString(rest)
		if err != nil {
			return false
		}
		_, rest, err = ReadMCString(rest)
		if err != nil {
			return false
		}
		if ns == "minecraft" && id == "core" {
			return true
		}
	}
	return false
}

var baseDamageTypes766 = []string{
	"minecraft:arrow",
	"minecraft:bad_respawn_point",
	"minecraft:cactus",
	"minecraft:cramming",
	"minecraft:dragon_breath",
	"minecraft:drown",
	"minecraft:dry_out",
	"minecraft:explosion",
	"minecraft:fall",
	"minecraft:falling_anvil",
	"minecraft:falling_block",
	"minecraft:falling_stalactite",
	"minecraft:fireball",
	"minecraft:fireworks",
	"minecraft:fly_into_wall",
	"minecraft:freeze",
	"minecraft:generic",
	"minecraft:generic_kill",
	"minecraft:hot_floor",
	"minecraft:in_fire",
	"minecraft:in_wall",
	"minecraft:indirect_magic",
	"minecraft:lava",
	"minecraft:lightning_bolt",
	"minecraft:magic",
	"minecraft:mob_attack",
	"minecraft:mob_attack_no_aggro",
	"minecraft:mob_projectile",
	"minecraft:on_fire",
	"minecraft:out_of_world",
	"minecraft:outside_border",
	"minecraft:player_attack",
	"minecraft:player_explosion",
	"minecraft:sonic_boom",
	"minecraft:spit",
	"minecraft:stalagmite",
	"minecraft:starve",
	"minecraft:sting",
	"minecraft:sweet_berry_bush",
	"minecraft:thorns",
	"minecraft:thrown",
	"minecraft:trident",
	"minecraft:unattributed_fireball",
	"minecraft:wind_charge",
	"minecraft:wither",
	"minecraft:wither_skull",
}

func buildRegistryDataPacket(registryID string, entries []string) []byte {
	var p []byte
	p = appendMCString(p, registryID)
	p = AppendVarInt(p, int32(len(entries)))
	for _, entry := range entries {
		p = appendMCString(p, entry)
		p = append(p, 0x00) // hasData = false (client resolves from known pack minecraft:core)
	}
	return buildMCPacket(mcCfgRegistryPacketID, p)
}

func buildMCRegistryDataPackets(protoVer int32) [][]byte {
	damageTypes := make([]string, 0, len(baseDamageTypes766)+3)
	damageTypes = append(damageTypes, baseDamageTypes766...)
	if protoVer >= 767 {
		damageTypes = append(damageTypes, "minecraft:campfire")
	}
	if protoVer >= 768 {
		damageTypes = append(damageTypes, "minecraft:ender_pearl", "minecraft:mace_smash")
	}

	pkts := [][]byte{
		buildRegistryDataPacket("minecraft:dimension_type", []string{"minecraft:overworld"}),
		buildRegistryDataPacket("minecraft:worldgen/biome", []string{"minecraft:plains"}),
		buildRegistryDataPacket("minecraft:damage_type", damageTypes),
		buildRegistryDataPacket("minecraft:wolf_variant", []string{"minecraft:pale"}),
	}
	if protoVer >= 767 {
		pkts = append(pkts, buildRegistryDataPacket("minecraft:painting_variant", []string{"minecraft:kebab"}))
	}
	if protoVer >= 770 {
		pkts = append(pkts,
			buildRegistryDataPacket("minecraft:cat_variant", []string{"minecraft:tabby"}),
			buildRegistryDataPacket("minecraft:chicken_variant", []string{"minecraft:temperate"}),
			buildRegistryDataPacket("minecraft:cow_variant", []string{"minecraft:temperate"}),
			buildRegistryDataPacket("minecraft:frog_variant", []string{"minecraft:temperate"}),
			buildRegistryDataPacket("minecraft:pig_variant", []string{"minecraft:temperate"}),
			buildRegistryDataPacket("minecraft:wolf_sound_variant", []string{"minecraft:classic"}),
		)
	}
	if protoVer >= 771 {
		pkts = append(pkts, buildRegistryDataPacket("minecraft:dialog", []string{"minecraft:server_links"}))
	}
	if protoVer >= 774 {
		pkts = append(pkts,
			buildRegistryDataPacket("minecraft:timeline", []string{"minecraft:day"}),
			buildRegistryDataPacket("minecraft:zombie_nautilus_variant", []string{"minecraft:temperate"}),
		)
	}
	if protoVer >= 775 {
		pkts = append(pkts,
			buildRegistryDataPacket("minecraft:banner_pattern", []string{"minecraft:base"}),
			buildRegistryDataPacket("minecraft:cat_sound_variant", []string{"minecraft:classic"}),
			buildRegistryDataPacket("minecraft:chicken_sound_variant", []string{"minecraft:classic"}),
			buildRegistryDataPacket("minecraft:cow_sound_variant", []string{"minecraft:classic"}),
			buildRegistryDataPacket("minecraft:instrument", []string{"minecraft:ponder_goat_horn"}),
			buildRegistryDataPacket("minecraft:jukebox_song", []string{"minecraft:13"}),
			buildRegistryDataPacket("minecraft:pig_sound_variant", []string{"minecraft:classic"}),
			buildRegistryDataPacket("minecraft:trim_material", []string{"minecraft:quartz"}),
			buildRegistryDataPacket("minecraft:world_clock", []string{"minecraft:overworld"}),
		)
	}
	return pkts
}

func buildMCPlayLoginPacket(spec mcLimboProto) []byte {
	var p []byte
	p = appendU32(p, 1) // entityId = 1
	p = append(p, 0)    // isHardcore = false
	p = AppendVarInt(p, 1)
	p = appendMCString(p, "minecraft:overworld")
	p = AppendVarInt(p, 1)  // maxPlayers
	p = AppendVarInt(p, 2)  // viewDistance
	p = AppendVarInt(p, 2)  // simulationDistance
	p = append(p, 0, 0, 0)  // reducedDebugInfo, enableRespawnScreen, doLimitedCrafting
	p = AppendVarInt(p, 0)  // dimensionType = 0 (minecraft:overworld)
	p = appendMCString(p, "minecraft:overworld")
	p = appendU64(p, 0)     // hashedSeed
	p = append(p, 2)        // gamemode = 2 (Adventure)
	p = append(p, 0xFF)     // previousGamemode = -1
	p = append(p, 0, 1, 0)  // isDebug=false, isFlat=true, hasDeathLocation=false
	p = AppendVarInt(p, 0)  // portalCooldown = 0
	if spec.loginHasSeaLevel {
		p = AppendVarInt(p, 63) // seaLevel
	}
	p = append(p, 0) // enforcesSecureChat = false
	return buildMCPacket(spec.playLogin, p)
}

func buildMCPlayerAbilitiesPacket(spec mcLimboProto) []byte {
	var p []byte
	p = append(p, 0x05) // Invulnerable (0x01) | Allow Flying (0x04)
	p = appendF32(p, 0.05)
	p = appendF32(p, 0.1)
	return buildMCPacket(spec.playAbilities, p)
}

func buildMCGameEventPacket(spec mcLimboProto, event uint8, value float32) []byte {
	var p []byte
	p = append(p, event)
	p = appendF32(p, value)
	return buildMCPacket(spec.playGameEvent, p)
}

func buildMCSetCenterChunkPacket(spec mcLimboProto, cx, cz int32) []byte {
	var p []byte
	p = AppendVarInt(p, cx)
	p = AppendVarInt(p, cz)
	return buildMCPacket(spec.playCenterChunk, p)
}

func appendSingleValuedPalette(dst []byte, val int32, noDataLen bool) []byte {
	dst = append(dst, 0) // bitsPerEntry = 0
	dst = AppendVarInt(dst, val)
	if !noDataLen {
		dst = AppendVarInt(dst, 0) // empty long array
	}
	return dst
}

func buildMCLimboChunkPacket(spec mcLimboProto, cx, cz int32, withPlatform bool) []byte {
	var p []byte
	p = appendU32(p, uint32(cx))
	p = appendU32(p, uint32(cz))

	// Heightmaps
	if spec.chunkHeightmapsPrefixed {
		p = AppendVarInt(p, 1)  // 1 heightmap entry
		p = AppendVarInt(p, 4)  // MOTION_BLOCKING
		p = AppendVarInt(p, 37) // 37 longs
		for i := 0; i < 37; i++ {
			p = appendU64(p, 0)
		}
	} else {
		p = append(p, encodeNBTEmptyHeightmaps()...)
	}

	// Build 24 chunk sections (y = -64..319). Section 7 covers y = 48..63; local y=15 is world y=63.
	var chunkData []byte
	const platformSectionIdx = (63 - limboMinY) / 16 // 7
	for s := 0; s < limboSections; s++ {
		if withPlatform && s == platformSectionIdx {
			chunkData = appendU16(chunkData, 25) // 5x5 stone platform
			if spec.sectionHasFluidCount {
				chunkData = appendU16(chunkData, 0)
			}
			// Indirect paletted container: bits=4, palette=[0 (air), 1 (stone)], 256 longs.
			chunkData = append(chunkData, 4)
			chunkData = AppendVarInt(chunkData, 2)
			chunkData = AppendVarInt(chunkData, 0)
			chunkData = AppendVarInt(chunkData, 1)
			if !spec.palettedNoDataLen {
				chunkData = AppendVarInt(chunkData, 256)
			}
			const localY = 15
			var rowMask uint64
			for x := 6; x <= 10; x++ {
				rowMask |= uint64(1) << (x * 4)
			}
			for longIdx := 0; longIdx < 256; longIdx++ {
				z := longIdx & 0x0F
				y := longIdx >> 4
				if y == localY && z >= 6 && z <= 10 {
					chunkData = appendU64(chunkData, rowMask)
				} else {
					chunkData = appendU64(chunkData, 0)
				}
			}
		} else {
			chunkData = appendU16(chunkData, 0)
			if spec.sectionHasFluidCount {
				chunkData = appendU16(chunkData, 0)
			}
			chunkData = appendSingleValuedPalette(chunkData, 0, spec.palettedNoDataLen)
		}
		// Biomes: single-valued 0 (minecraft:plains)
		chunkData = appendSingleValuedPalette(chunkData, 0, spec.palettedNoDataLen)
	}

	p = AppendVarInt(p, int32(len(chunkData)))
	p = append(p, chunkData...)
	p = AppendVarInt(p, 0) // blockEntities count = 0

	// Light data (26 light sections total: 24 + 2 padding)
	if withPlatform {
		// Light section index = sectionIdx + 1. Platform is in section 7 (y=63), player stands at y=64 (section 8).
		// Illuminate light sections 8 and 9 with full sky light (0xFF).
		const skyMask = (uint64(1) << 8) | (uint64(1) << 9)
		p = AppendVarInt(p, 1)
		p = appendU64(p, skyMask)
		p = AppendVarInt(p, 0) // blockLightMask
		p = AppendVarInt(p, 0) // emptySkyLightMask
		p = AppendVarInt(p, 0) // emptyBlockLightMask

		p = AppendVarInt(p, 2) // skyLight arrays
		for i := 0; i < 2; i++ {
			p = AppendVarInt(p, 2048)
			for j := 0; j < 2048; j++ {
				p = append(p, 0xFF)
			}
		}
		p = AppendVarInt(p, 0) // blockLight arrays
	} else {
		p = AppendVarInt(p, 0) // skyLightMask
		p = AppendVarInt(p, 0) // blockLightMask
		p = AppendVarInt(p, 0) // emptySkyLightMask
		p = AppendVarInt(p, 0) // emptyBlockLightMask
		p = AppendVarInt(p, 0) // skyLight arrays
		p = AppendVarInt(p, 0) // blockLight arrays
	}

	return buildMCPacket(spec.playChunk, p)
}

func buildMCSyncPositionPacket(spec mcLimboProto, x, y, z float64, yaw, pitch float32, teleportID int32) []byte {
	var p []byte
	if spec.posNewFormat {
		p = AppendVarInt(p, teleportID)
		p = appendF64(p, x)
		p = appendF64(p, y)
		p = appendF64(p, z)
		p = appendF64(p, 0) // dx
		p = appendF64(p, 0) // dy
		p = appendF64(p, 0) // dz
		p = appendF32(p, yaw)
		p = appendF32(p, pitch)
		p = appendU32(p, 0) // flags
	} else {
		p = appendF64(p, x)
		p = appendF64(p, y)
		p = appendF64(p, z)
		p = appendF32(p, yaw)
		p = appendF32(p, pitch)
		p = append(p, 0) // flags
		p = AppendVarInt(p, teleportID)
	}
	return buildMCPacket(spec.playPosC, p)
}

func buildMCSystemChatPacket(spec mcLimboProto, nbtBytes []byte, overlay bool) []byte {
	p := make([]byte, 0, len(nbtBytes)+1)
	p = append(p, nbtBytes...)
	if overlay {
		p = append(p, 1)
	} else {
		p = append(p, 0)
	}
	return buildMCPacket(spec.playSystemChat, p)
}

func buildMCKeepAlivePacket(packetID int32, id int64) []byte {
	var p []byte
	p = appendU64(p, uint64(id))
	return buildMCPacket(packetID, p)
}

func buildMCTransferPacket(spec mcLimboProto, host string, port int32) []byte {
	var p []byte
	p = appendMCString(p, host)
	p = AppendVarInt(p, port)
	return buildMCPacket(spec.playTransfer, p)
}

func buildMCDisconnectWithNBT(packetID int32, nbtBytes []byte) []byte {
	return buildMCPacket(packetID, nbtBytes)
}

func sanitizeTransferTarget(
	serverAddr string,
	serverPort uint16,
	localAddr net.Addr,
	clientIP, detectedPublicIP string,
	fallbackPort int,
) (string, int32) {
	host := serverAddr
	if nul := strings.IndexByte(host, 0); nul != -1 {
		host = host[:nul]
	}
	host = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		host = pickVerifyIP(localAddr, clientIP, detectedPublicIP)
	}
	port := int32(serverPort)
	if port <= 0 {
		if fallbackPort > 0 {
			port = int32(fallbackPort)
		} else {
			port = int32(config.DefaultMinecraftPort)
		}
	}
	return host, port
}

type limboEventKind uint8

const (
	limboEventClosed limboEventKind = iota + 1
	limboEventFell
	limboEventChat
)

type limboEvent struct {
	kind limboEventKind
}

func writeAllBytes(conn net.Conn, data []byte, timeout time.Duration) error {
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	}
	_, err := conn.Write(data)
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Time{})
	}
	return err
}

// serveLimbo runs an in-memory empty-world Minecraft session for supported 1.20.5+ clients,
// sends the clickable verification URL in chat, and redirects the player via Transfer once cleared.
func (p *MinecraftProxy) serveLimbo(
	ctx context.Context,
	conn net.Conn,
	br *bufio.Reader,
	cfg config.MinecraftConfig,
	protoVer int32,
	serverAddr string,
	serverPort uint16,
	username string,
	playerUUID [16]byte,
	clientIP, verifyURL string,
) {
	spec, ok := lookupLimboProto(protoVer)
	if !ok {
		p.sendDisconnect(conn, BuildVerifyDisconnectJSONForProtocol(verifyURL, username, protoVer))
		return
	}

	// 1. Login -> Configuration transition
	if err := writeAllBytes(conn, buildMCLoginSuccessPacket(spec, playerUUID, username), 5*time.Second); err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		pid, _, _, err := ReadMCPacket(br, 65536)
		if err != nil {
			return
		}
		if pid == mcLoginAckPacketID {
			break
		}
	}

	// 2. Configuration phase: negotiate minecraft:core Known Packs, send registry IDs + Finish
	if err := writeAllBytes(conn, buildMCKnownPacksPacket(spec.knownPackVersions), 5*time.Second); err != nil {
		return
	}
	for {
		pid, payload, _, err := ReadMCPacket(br, 65536)
		if err != nil {
			return
		}
		if pid == mcCfgKeepAliveSPacketID {
			continue
		}
		if pid == mcCfgKnownPacksSPacketID {
			if !parseServerKnownPacksHasCore(payload) {
				msg := "Verification required before joining.\nOpen in your browser: " + verifyURL
				_ = writeAllBytes(conn, buildMCDisconnectWithNBT(mcCfgDisconnectPacketID, encodeNBTSimpleText(msg, "aqua", false)), 3*time.Second)
				return
			}
			break
		}
	}

	var cfgBurst []byte
	for _, rp := range buildMCRegistryDataPackets(protoVer) {
		cfgBurst = append(cfgBurst, rp...)
	}
	cfgBurst = append(cfgBurst, buildMCPacket(mcCfgFinishCPacketID, nil)...)
	if err := writeAllBytes(conn, cfgBurst, 5*time.Second); err != nil {
		return
	}

	for {
		pid, _, _, err := ReadMCPacket(br, 65536)
		if err != nil {
			return
		}
		if pid == mcCfgKeepAliveSPacketID {
			continue
		}
		if pid == mcCfgFinishSPacketID {
			break
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	// 3. Play phase: spawn in small in-memory stone platform world & send verification link in chat
	clearedCh, unsub := p.subscribeClearance(clientIP)
	defer unsub()

	verifyChatPkt := buildMCSystemChatPacket(spec, encodeNBTVerifyChat(verifyURL, username, spec.modernTextEvents), false)
	actionBarPkt := buildMCSystemChatPacket(spec, encodeNBTSimpleText(limboActionBarText, "gold", false), true)

	var playBurst []byte
	playBurst = append(playBurst, buildMCPlayLoginPacket(spec)...)
	playBurst = append(playBurst, buildMCPlayerAbilitiesPacket(spec)...)
	playBurst = append(playBurst, buildMCGameEventPacket(spec, 13, 0)...)
	playBurst = append(playBurst, buildMCSetCenterChunkPacket(spec, 0, 0)...)
	for cx := int32(-1); cx <= 1; cx++ {
		for cz := int32(-1); cz <= 1; cz++ {
			playBurst = append(playBurst, buildMCLimboChunkPacket(spec, cx, cz, cx == 0 && cz == 0)...)
		}
	}
	var teleportID int32 = 1
	playBurst = append(playBurst, buildMCSyncPositionPacket(spec, limboSpawnX, limboSpawnY, limboSpawnZ, 0, 0, teleportID)...)
	playBurst = append(playBurst, verifyChatPkt...)
	playBurst = append(playBurst, actionBarPkt...)

	if err := writeAllBytes(conn, playBurst, 5*time.Second); err != nil {
		return
	}

	transferToRealServer := func() {
		completeChat := buildMCSystemChatPacket(spec, encodeNBTVerifyCompleteChat(), false)
		host, port := sanitizeTransferTarget(serverAddr, serverPort, conn.LocalAddr(), clientIP, p.detectedPublicIP, cfg.Port)
		p.markPendingTransfer(clientIP)
		transferPkt := buildMCTransferPacket(spec, host, port)

		var out []byte
		out = append(out, completeChat...)
		out = append(out, transferPkt...)
		_ = writeAllBytes(conn, out, 3*time.Second)
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}

	// Check immediately in case clearance completed during Login/Configuration.
	if p.gateStore != nil {
		if _, cleared, err := p.gateStore.IsCleared(ctx, "minecraft", clientIP); err == nil && cleared {
			transferToRealServer()
			return
		}
	}

	events := make(chan limboEvent, 16)
	go func() {
		defer func() {
			select {
			case events <- limboEvent{kind: limboEventClosed}:
			default:
			}
		}()
		for {
			pid, payload, _, err := ReadMCPacket(br, 65536)
			if err != nil {
				return
			}
			switch pid {
			case spec.playPosS, spec.playPosRotS:
				if len(payload) >= 24 {
					y := math.Float64frombits(binary.BigEndian.Uint64(payload[8:16]))
					if y < 40.0 {
						select {
						case events <- limboEvent{kind: limboEventFell}:
						default:
						}
					}
				}
			case spec.playChatS:
				select {
				case events <- limboEvent{kind: limboEventChat}:
				default:
				}
			}
		}
	}()

	ttl := time.Duration(cfg.ChallengeTTL) * time.Second
	if ttl < 60*time.Second {
		ttl = 60 * time.Second
	}
	sessionTimer := time.NewTimer(ttl)
	defer sessionTimer.Stop()
	pollTicker := time.NewTicker(500 * time.Millisecond)
	defer pollTicker.Stop()
	keepAliveTicker := time.NewTicker(10 * time.Second)
	defer keepAliveTicker.Stop()
	actionBarTicker := time.NewTicker(3 * time.Second)
	defer actionBarTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = writeAllBytes(conn, buildMCDisconnectWithNBT(spec.playDisconnect, encodeNBTSimpleText("Server shutting down.", "red", false)), 2*time.Second)
			return
		case <-sessionTimer.C:
			_ = writeAllBytes(conn, buildMCDisconnectWithNBT(spec.playDisconnect, encodeNBTSimpleText("Verification link expired. Please reconnect to get a new link.", "red", false)), 2*time.Second)
			return
		case <-clearedCh:
			transferToRealServer()
			waitForClientClose(events)
			return
		case <-pollTicker.C:
			if p.gateStore != nil {
				if _, cleared, err := p.gateStore.IsCleared(ctx, "minecraft", clientIP); err == nil && cleared {
					transferToRealServer()
					waitForClientClose(events)
					return
				}
			}
		case <-keepAliveTicker.C:
			ka := buildMCKeepAlivePacket(spec.playKeepAliveC, time.Now().UnixMilli())
			if err := writeAllBytes(conn, ka, 3*time.Second); err != nil {
				return
			}
		case <-actionBarTicker.C:
			if err := writeAllBytes(conn, actionBarPkt, 3*time.Second); err != nil {
				return
			}
		case ev := <-events:
			switch ev.kind {
			case limboEventClosed:
				return
			case limboEventFell:
				teleportID++
				tp := buildMCSyncPositionPacket(spec, limboSpawnX, limboSpawnY, limboSpawnZ, 0, 0, teleportID)
				if err := writeAllBytes(conn, tp, 3*time.Second); err != nil {
					return
				}
			case limboEventChat:
				if err := writeAllBytes(conn, verifyChatPkt, 3*time.Second); err != nil {
					return
				}
			}
		}
	}
}

func waitForClientClose(events <-chan limboEvent) {
	drainTimer := time.NewTimer(300 * time.Millisecond)
	defer drainTimer.Stop()
	for {
		select {
		case ev := <-events:
			if ev.kind == limboEventClosed {
				return
			}
		case <-drainTimer.C:
			return
		}
	}
}
