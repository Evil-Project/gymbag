package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPickRandomFreePort(t *testing.T) {
	p, err := PickRandomFreePort(22, 8443, 25565)
	if err != nil {
		t.Fatalf("PickRandomFreePort: %v", err)
	}
	if p < minRandomPort || p > maxRandomPort {
		t.Fatalf("picked port %d outside [%d, %d]", p, minRandomPort, maxRandomPort)
	}
	if slices.Contains([]int{22, 8443, 25565}, p) {
		t.Fatalf("picked excluded port %d", p)
	}
}

func TestRewriteSSHDConfig(t *testing.T) {
	orig := "# OpenSSH config\n#Port 22\nPort 22\nPort 2222\nPermitRootLogin prohibit-password\nMatch User deploy\n    Port 2200\n    AllowTcpForwarding no\n"
	first := RewriteSSHDConfig(orig, 34567)

	if !strings.Contains(first, "# Port 22 # disabled by gymbag\n") {
		t.Fatalf("expected active Port 22 commented out:\n%s", first)
	}
	if !strings.Contains(first, "# Port 2222 # disabled by gymbag\n") {
		t.Fatalf("expected active Port 2222 commented out:\n%s", first)
	}
	if !strings.Contains(first, "#Port 22\n") {
		t.Fatalf("expected existing comment preserved:\n%s", first)
	}
	idxManaged := strings.Index(first, sshdManagedStart+"\nPort 34567\n"+sshdManagedEnd)
	idxMatch := strings.Index(first, "Match User deploy")
	if idxManaged < 0 || idxMatch < 0 || idxManaged >= idxMatch {
		t.Fatalf("expected managed block before Match User:\n%s", first)
	}
	if !strings.Contains(first, "    Port 2200\n") {
		t.Fatalf("expected Port inside Match block untouched:\n%s", first)
	}

	// Idempotent rewrite with a different port replaces only the managed block.
	second := RewriteSSHDConfig(first, 45678)
	if strings.Count(second, sshdManagedStart) != 1 {
		t.Fatalf("expected exactly 1 managed block:\n%s", second)
	}
	if strings.Contains(second, "Port 34567") || !strings.Contains(second, "Port 45678") {
		t.Fatalf("expected managed port updated to 45678:\n%s", second)
	}

	// Preserves CRLF line endings when present.
	crlf := RewriteSSHDConfig("Port 22\r\nPasswordAuthentication no\r\n", 31234)
	if !strings.Contains(crlf, "\r\nPort 31234\r\n") {
		t.Fatalf("expected CRLF preservation:\n%q", crlf)
	}
}

func TestSetupSSHProtectionValidationRollbackAndFirewalls(t *testing.T) {
	t.Run("rollback when sshd -t fails", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "sshd_config")
		orig := "Port 22\nPermitRootLogin no\n"
		if err := os.WriteFile(cfgPath, []byte(orig), 0o600); err != nil {
			t.Fatal(err)
		}

		_, err := SetupSSHProtection(context.Background(), SSHSetupOptions{
			ConfigPath:   cfgPath,
			BackendPort:  32100,
			SkipFirewall: true,
			LookPath: func(file string) (string, error) {
				if file == "sshd" {
					return "/usr/sbin/sshd", nil
				}
				return "", errors.New("not found")
			},
			Runner: func(_ context.Context, name string, args ...string) ([]byte, error) {
				if name == "/usr/sbin/sshd" {
					return []byte("sshd_config: line 99: Bad configuration option"), errors.New("exit 255")
				}
				return nil, nil
			},
			GOOS: "linux",
		})
		if err == nil || !strings.Contains(err.Error(), "sshd -t validation failed") {
			t.Fatalf("expected validation error, got: %v", err)
		}
		got, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != orig {
			t.Fatalf("expected rollback to original config:\n%s", string(got))
		}
		bak, err := os.ReadFile(cfgPath + ".gymbag.bak")
		if err != nil || string(bak) != orig {
			t.Fatalf("expected backup preserved: err=%v bak=%q", err, string(bak))
		}
	})

	for _, tc := range []struct {
		name            string
		available       []string
		wantBackend     string
		wantCmdContains string
	}{
		{
			name:            "ufw",
			available:       []string{"sshd", "systemctl", "ufw"},
			wantBackend:     "ufw",
			wantCmdContains: "ufw deny 34567/tcp",
		},
		{
			name:            "firewalld",
			available:       []string{"sshd", "systemctl", "firewall-cmd"},
			wantBackend:     "firewalld",
			wantCmdContains: `port port="34567" protocol="tcp" drop`,
		},
		{
			name:            "nftables",
			available:       []string{"sshd", "systemctl", "nft"},
			wantBackend:     "nftables",
			wantCmdContains: "nft add rule inet gymbag_ssh input tcp dport 34567 drop",
		},
		{
			name:            "iptables",
			available:       []string{"sshd", "systemctl", "iptables", "ip6tables"},
			wantBackend:     "iptables",
			wantCmdContains: "iptables -I INPUT 1 ! -i lo -p tcp --dport 34567 -j DROP",
		},
	} {
		t.Run("firewall_"+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "sshd_config")
			if err := os.WriteFile(cfgPath, []byte("Port 22\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			var invoked []string
			res, err := SetupSSHProtection(context.Background(), SSHSetupOptions{
				ConfigPath:    cfgPath,
				BackendPort:   34567,
				SSHListenPort: 22,
				VerifyPort:    8443,
				GOOS:          "linux",
				LookPath: func(file string) (string, error) {
					if slices.Contains(tc.available, file) {
						return "/usr/bin/" + file, nil
					}
					return "", errors.New("not found")
				},
				Runner: func(_ context.Context, name string, args ...string) ([]byte, error) {
					cmdLine := name + " " + strings.Join(args, " ")
					invoked = append(invoked, cmdLine)
					switch {
					case strings.HasSuffix(name, "ufw") && len(args) > 0 && args[0] == "status":
						return []byte("Status: active\n"), nil
					case strings.HasSuffix(name, "firewall-cmd") && len(args) > 0 && args[0] == "--state":
						return []byte("running\n"), nil
					case (name == "iptables" || name == "ip6tables") && len(args) > 0 && args[0] == "-C":
						return nil, errors.New("rule missing")
					}
					return []byte("ok"), nil
				},
			})
			if err != nil {
				t.Fatalf("SetupSSHProtection: %v", err)
			}
			if res.BackendPort != 34567 || res.Upstream != "127.0.0.1:34567" {
				t.Fatalf("unexpected result: %+v", res)
			}
			if res.FirewallBackend != tc.wantBackend {
				t.Fatalf("firewall backend = %q, want %q", res.FirewallBackend, tc.wantBackend)
			}
			joined := strings.Join(res.FirewallCommands, "\n")
			if !strings.Contains(joined, tc.wantCmdContains) {
				t.Fatalf("expected command containing %q in:\n%s", tc.wantCmdContains, joined)
			}
			updatedBytes, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(updatedBytes), "Port 34567") {
				t.Fatalf("expected sshd_config updated:\n%s", string(updatedBytes))
			}
			if !slices.ContainsFunc(invoked, func(s string) bool { return strings.Contains(s, "systemctl reload sshd") }) {
				t.Fatalf("expected systemctl reload sshd call in %v", invoked)
			}
		})
	}

	t.Run("non-linux emits warning without error", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "sshd_config")
		if err := os.WriteFile(cfgPath, []byte("Port 22\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		res, err := SetupSSHProtection(context.Background(), SSHSetupOptions{
			ConfigPath:  cfgPath,
			BackendPort: 39999,
			GOOS:        "darwin",
			LookPath:    func(string) (string, error) { return "", errors.New("not found") },
		})
		if err != nil {
			t.Fatalf("SetupSSHProtection: %v", err)
		}
		if res.FirewallBackend != "none" || len(res.Warnings) == 0 {
			t.Fatalf("expected non-linux warning, got %+v", res)
		}
	})
}

func TestSetupMinecraftProtection(t *testing.T) {
	t.Run("rewrites port, binds loopback when empty, creates backup", func(t *testing.T) {
		dir := t.TempDir()
		propPath := filepath.Join(dir, "server.properties")
		orig := "#Minecraft server properties\nserver-ip=\nserver-port=25565\nonline-mode=true\n"
		if err := os.WriteFile(propPath, []byte(orig), 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := SetupMinecraftProtection(MinecraftSetupOptions{
			PropertiesPath:      propPath,
			BackendPort:         41234,
			MinecraftListenPort: 25565,
			VerifyPort:          8443,
			BindLoopbackIfEmpty: true,
		})
		if err != nil {
			t.Fatalf("SetupMinecraftProtection: %v", err)
		}
		if res.PreviousPort != "25565" || res.BackendPort != 41234 || res.Upstream != "127.0.0.1:41234" {
			t.Fatalf("unexpected result: %+v", res)
		}
		gotBytes, err := os.ReadFile(propPath)
		if err != nil {
			t.Fatal(err)
		}
		got := string(gotBytes)
		if !strings.Contains(got, "server-port=41234\n") || !strings.Contains(got, "server-ip=127.0.0.1\n") || !strings.Contains(got, "online-mode=true\n") {
			t.Fatalf("unexpected updated server.properties:\n%s", got)
		}
		bak, err := os.ReadFile(res.BackupPath)
		if err != nil || string(bak) != orig {
			t.Fatalf("expected backup matching original: err=%v bak=%q", err, string(bak))
		}
	})

	t.Run("picks random port automatically and preserves explicit server-ip", func(t *testing.T) {
		dir := t.TempDir()
		propPath := filepath.Join(dir, "server.properties")
		orig := "server-ip=10.20.30.40\r\nmotd=Hello\r\n"
		if err := os.WriteFile(propPath, []byte(orig), 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := SetupMinecraftProtection(MinecraftSetupOptions{
			PropertiesPath:      propPath,
			BindLoopbackIfEmpty: true,
		})
		if err != nil {
			t.Fatalf("SetupMinecraftProtection: %v", err)
		}
		if res.BackendPort < minRandomPort || res.BackendPort > maxRandomPort {
			t.Fatalf("unexpected random backend port %d", res.BackendPort)
		}
		wantUp := fmt.Sprintf("10.20.30.40:%d", res.BackendPort)
		if res.Upstream != wantUp {
			t.Fatalf("upstream = %q, want %q", res.Upstream, wantUp)
		}
		gotBytes, err := os.ReadFile(propPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(gotBytes), fmt.Sprintf("server-port=%d\r\n", res.BackendPort)) {
			t.Fatalf("expected appended CRLF server-port:\n%q", string(gotBytes))
		}
	})
}

func TestSetupMinecraftProtectionCreateIfMissing(t *testing.T) {
	dir := t.TempDir()
	propPath := filepath.Join(dir, "fresh", "server.properties")
	res, err := SetupMinecraftProtection(MinecraftSetupOptions{
		PropertiesPath:      propPath,
		BackendPort:         45123,
		BindLoopbackIfEmpty: true,
		CreateIfMissing:     true,
	})
	if err != nil {
		t.Fatalf("SetupMinecraftProtection CreateIfMissing failed: %v", err)
	}
	if !res.Created || res.BackupPath != "" || res.Upstream != "127.0.0.1:45123" {
		t.Fatalf("unexpected result: %+v", res)
	}
	raw, err := os.ReadFile(propPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "server-ip=127.0.0.1\n") || !strings.Contains(string(raw), "server-port=45123\n") {
		t.Fatalf("unexpected created server.properties:\n%s", string(raw))
	}
}
