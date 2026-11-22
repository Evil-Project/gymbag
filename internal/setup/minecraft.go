package setup

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MinecraftSetupOptions configures automated server.properties random-port setup.
type MinecraftSetupOptions struct {
	PropertiesPath      string
	BackendPort         int
	MinecraftListenPort int
	VerifyPort          int
	BindLoopbackIfEmpty bool
	CreateIfMissing     bool
}

// MinecraftSetupResult summarizes updates applied to a Minecraft server.properties file.
type MinecraftSetupResult struct {
	PropertiesPath string
	BackupPath     string
	Created        bool
	PreviousPort   string
	BackendPort    int
	UpstreamHost   string
	Upstream       string
}

// RewriteServerProperties updates server-port (and optionally binds empty/wildcard server-ip to 127.0.0.1)
// while preserving comments, ordering, and line endings.
func RewriteServerProperties(content string, newPort int, bindLoopbackIfEmpty bool) (updated, upstreamHost, prevPort string) {
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	hadTrailingNL := strings.HasSuffix(normalized, "\n")
	if hadTrailingNL && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	foundPort := false
	foundIP := false
	upstreamHost = "127.0.0.1"

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		k, v, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		switch key {
		case "server-port":
			prevPort = val
			lines[i] = fmt.Sprintf("server-port=%d", newPort)
			foundPort = true
		case "server-ip":
			foundIP = true
			if val == "" || val == "0.0.0.0" || val == "::" {
				if bindLoopbackIfEmpty {
					lines[i] = "server-ip=127.0.0.1"
				}
				upstreamHost = "127.0.0.1"
			} else {
				upstreamHost = val
			}
		}
	}

	if !foundPort {
		lines = append(lines, fmt.Sprintf("server-port=%d", newPort))
	}
	if !foundIP && bindLoopbackIfEmpty {
		lines = append(lines, "server-ip=127.0.0.1")
	}

	res := strings.Join(lines, lineEnding)
	if hadTrailingNL || res != "" {
		res += lineEnding
	}
	return res, upstreamHost, prevPort
}

// SetupMinecraftProtection rewrites server.properties to listen on a random backend port
// and returns the upstream address for Gymbag's Minecraft protection proxy.
func SetupMinecraftProtection(opts MinecraftSetupOptions) (*MinecraftSetupResult, error) {
	propPath := strings.TrimSpace(opts.PropertiesPath)
	if propPath == "" {
		propPath = "server.properties"
	}
	mcPort := opts.MinecraftListenPort
	if mcPort <= 0 {
		mcPort = 25565
	}
	verifyPort := opts.VerifyPort
	if verifyPort <= 0 {
		verifyPort = 8443
	}
	backendPort := opts.BackendPort
	if backendPort <= 0 {
		var err error
		backendPort, err = PickRandomFreePort(mcPort, verifyPort)
		if err != nil {
			return nil, err
		}
	}

	info, err := os.Stat(propPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && opts.CreateIfMissing {
			if dir := filepath.Dir(propPath); dir != "" && dir != "." {
				if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
					return nil, fmt.Errorf("failed to create directory for %s: %w", propPath, mkErr)
				}
			}
			initial := fmt.Sprintf("# Minecraft server properties (initialized by gymbag)\nserver-ip=127.0.0.1\nserver-port=%d\n", backendPort)
			if wErr := writeFilePreserveMode(propPath, []byte(initial), 0o644); wErr != nil {
				return nil, fmt.Errorf("failed to create %s: %w", propPath, wErr)
			}
			return &MinecraftSetupResult{
				PropertiesPath: propPath,
				Created:        true,
				BackendPort:    backendPort,
				UpstreamHost:   "127.0.0.1",
				Upstream:       fmt.Sprintf("127.0.0.1:%d", backendPort),
			}, nil
		}
		return nil, fmt.Errorf("failed to stat %s: %w", propPath, err)
	}
	origBytes, err := os.ReadFile(propPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", propPath, err)
	}

	backupPath := propPath + ".gymbag.bak"
	if _, statErr := os.Stat(backupPath); errors.Is(statErr, os.ErrNotExist) {
		if wErr := os.WriteFile(backupPath, origBytes, info.Mode().Perm()); wErr != nil {
			return nil, fmt.Errorf("failed to create backup %s: %w", backupPath, wErr)
		}
	}

	updated, upHost, prevPort := RewriteServerProperties(string(origBytes), backendPort, opts.BindLoopbackIfEmpty)
	if err := writeFilePreserveMode(propPath, []byte(updated), info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", propPath, err)
	}

	return &MinecraftSetupResult{
		PropertiesPath: propPath,
		BackupPath:     backupPath,
		PreviousPort:   prevPort,
		BackendPort:    backendPort,
		UpstreamHost:   upHost,
		Upstream:       net.JoinHostPort(upHost, strconv.Itoa(backendPort)),
	}, nil
}
