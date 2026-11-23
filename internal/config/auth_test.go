package config

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestForwardAuthValidationAndEndpointURL(t *testing.T) {
	a := &ForwardAuthConfig{
		URL: "authelia:9091",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if a.Provider != "authelia" || a.URI != DefaultAutheliaURI || a.Timeout != DefaultAuthTimeout {
		t.Fatalf("unexpected defaults: %+v", a)
	}
	if !slices.Equal(a.CopyHeaders, DefaultAutheliaCopyHeaders) {
		t.Fatalf("unexpected copy headers: %v", a.CopyHeaders)
	}
	ep, err := a.EndpointURL()
	if err != nil || ep != "http://authelia:9091/api/authz/forward-auth" {
		t.Fatalf("EndpointURL() = (%q, %v)", ep, err)
	}

	legacy := &ForwardAuthConfig{
		URL:         "https://auth.internal:9091",
		URI:         "/api/verify",
		PortalURL:   "https://auth.example.com",
		CopyHeaders: []string{"remote-user>X-Auth-User", "Remote-Groups"},
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("Validate legacy failed: %v", err)
	}
	epLegacy, err := legacy.EndpointURL()
	if err != nil || epLegacy != "https://auth.internal:9091/api/verify?rd=https%3A%2F%2Fauth.example.com" {
		t.Fatalf("legacy EndpointURL() = (%q, %v)", epLegacy, err)
	}
	if !slices.Equal(legacy.CopyHeaders, []string{"Remote-User>X-Auth-User", "Remote-Groups"}) {
		t.Fatalf("expected canonical copy_headers mapping, got %v", legacy.CopyHeaders)
	}
}

func TestAutheliaCaddyfileAndForwardAuthRoundTrip(t *testing.T) {
	doc := `
{
    authelia http://authelia:9091 {
        portal_url https://auth.example.com
        except /healthz /public/*
        timeout 8s
    }
}

auth.example.com {
    reverse_proxy http://authelia:9091
    protect off
}

app.example.com {
    reverse_proxy http://localhost:3000
    authelia
}

api.example.com {
    @admin path /admin/* /metrics
    forward_auth @admin http://auth-edge:9091 {
        uri /api/authz/forward-auth
        copy_headers {
            Remote-User
            Remote-Groups > X-Forwarded-Groups
        }
        header_up X-Custom-Edge "gymbag"
        transport http {
            tls_insecure_skip_verify
        }
    }
    reverse_proxy http://localhost:8080
}

scoped.example.com {
    handle /private/* {
        authelia {
            except /private/ping
        }
        reverse_proxy http://localhost:9000
    }
    reverse_proxy http://localhost:3001
}
`
	cfg, err := ParseCaddyfile(doc)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}
	if cfg.Authelia == nil || cfg.Authelia.URL != "http://authelia:9091" || cfg.Authelia.PortalURL != "https://auth.example.com" || cfg.Authelia.Timeout != 8 {
		t.Fatalf("unexpected global Authelia config: %+v", cfg.Authelia)
	}
	if len(cfg.Sites) != 4 {
		t.Fatalf("expected 4 sites, got %d", len(cfg.Sites))
	}
	if cfg.Sites[0].Auth != nil {
		t.Fatalf("expected auth.example.com not to inherit global authelia automatically")
	}
	if cfg.Sites[1].Auth == nil || cfg.Sites[1].Auth.PortalURL != "https://auth.example.com" {
		t.Fatalf("expected app.example.com to inherit global authelia, got %+v", cfg.Sites[1].Auth)
	}
	apiAuth := cfg.Sites[2].Auth
	if apiAuth == nil || apiAuth.Provider != "forward_auth" || !apiAuth.InsecureSkipTLS {
		t.Fatalf("unexpected api.example.com forward_auth: %+v", apiAuth)
	}
	if !slices.Equal(apiAuth.Paths, []string{"/admin/*", "/metrics"}) {
		t.Fatalf("unexpected api.example.com auth paths: %v", apiAuth.Paths)
	}
	if !slices.Equal(apiAuth.CopyHeaders, []string{"Remote-User", "Remote-Groups>X-Forwarded-Groups"}) {
		t.Fatalf("unexpected api.example.com copy_headers: %v", apiAuth.CopyHeaders)
	}
	if apiAuth.Headers["X-Custom-Edge"] != "gymbag" {
		t.Fatalf("expected X-Custom-Edge header, got %+v", apiAuth.Headers)
	}

	scoped := cfg.Sites[3]
	if scoped.Auth != nil || len(scoped.Routes) != 1 || scoped.Routes[0].Auth == nil {
		t.Fatalf("expected scoped route auth on /private/*, got siteAuth=%+v routes=%+v", scoped.Auth, scoped.Routes)
	}
	if !slices.Equal(scoped.Routes[0].Auth.ExceptPaths, []string{"/private/ping"}) {
		t.Fatalf("unexpected route auth except_paths: %v", scoped.Routes[0].Auth.ExceptPaths)
	}

	formatted := FormatConfig(cfg)
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("reparse formatted config failed: %v\n%s", err, formatted)
	}
	if reparsed.Sites[1].Auth == nil || reparsed.Sites[2].Auth == nil || len(reparsed.Sites[3].Routes) != 1 || reparsed.Sites[3].Routes[0].Auth == nil {
		t.Fatalf("formatted round-trip lost auth fields:\n%s", formatted)
	}
	if !strings.Contains(formatted, "\n    authelia\n") {
		t.Fatalf("expected concise site-level `authelia` shorthand when matching global:\n%s", formatted)
	}
}

func TestNginxAuthRequestAndJSONEnvAuthelia(t *testing.T) {
	nginxConf := `
server {
    listen 443 ssl;
    server_name app.example.com;

    location /internal/authelia/authz {
        internal;
        proxy_pass http://authelia:9091/api/authz/auth-request;
    }

    location /admin/ {
        auth_request /internal/authelia/authz;
        auth_request_set $user $upstream_http_remote_user;
        auth_request_set $groups $upstream_http_remote_groups;
        proxy_pass http://localhost:8080;
    }

    location / {
        proxy_pass http://localhost:3000;
    }
}
`
	cfg, err := ParseNginx(nginxConf)
	if err != nil {
		t.Fatalf("ParseNginx failed: %v", err)
	}
	if len(cfg.Sites) != 1 || len(cfg.Sites[0].Routes) != 2 {
		t.Fatalf("expected internal location excluded and 2 public routes kept, got %+v", cfg.Sites)
	}
	adminRoute := cfg.Sites[0].Routes[0]
	if adminRoute.Auth == nil || adminRoute.Auth.URL != "http://authelia:9091/api/authz/auth-request" {
		t.Fatalf("unexpected nginx auth_request resolution: %+v", adminRoute.Auth)
	}
	if !slices.Equal(adminRoute.Auth.CopyHeaders, []string{"Remote-User", "Remote-Groups"}) {
		t.Fatalf("unexpected nginx auth_request_set headers: %v", adminRoute.Auth.CopyHeaders)
	}

	// JSON concise marshal & strict unmarshal round-trip
	rawJSON, err := MarshalConciseJSONConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConciseJSONConfig failed: %v", err)
	}
	var parsed CheckJSON
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		t.Fatalf("invalid concise JSON: %v", err)
	}
	if _, err := ParseConfigBytes(rawJSON, "gymbag.json", ""); err != nil {
		t.Fatalf("reparse concise JSON failed: %v\n%s", err, string(rawJSON))
	}

	// Env var loading: skips portal host automatically
	envMap := map[string]string{
		"GYMBAG_DOMAINS":               "app.example.com,auth.example.com",
		"GYMBAG_UPSTREAMS":             "app.example.com=http://localhost:3000;auth.example.com=http://authelia:9091",
		"GYMBAG_AUTHELIA_URL":          "http://authelia:9091",
		"GYMBAG_AUTHELIA_PORTAL_URL":   "https://auth.example.com",
		"GYMBAG_AUTHELIA_TIMEOUT":      "7s",
		"GYMBAG_AUTHELIA_COPY_HEADERS": "Remote-User,Remote-Email",
	}
	envCfg, err := LoadWithEnv(LoadOptions{}, func(k string) string { return envMap[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if envCfg.Sites[0].Auth == nil || envCfg.Sites[0].Auth.Timeout != 7 {
		t.Fatalf("expected app.example.com protected via env, got %+v", envCfg.Sites[0].Auth)
	}
	if !slices.Equal(envCfg.Sites[0].Auth.CopyHeaders, []string{"Remote-User", "Remote-Email"}) {
		t.Fatalf("unexpected env copy headers: %v", envCfg.Sites[0].Auth.CopyHeaders)
	}
	if envCfg.Sites[1].Auth != nil {
		t.Fatalf("expected auth.example.com portal host skipped by GYMBAG_AUTHELIA_URL, got %+v", envCfg.Sites[1].Auth)
	}
}

type CheckJSON map[string]any

func TestMinecraftConsoleConfig(t *testing.T) {
	// 1. JSON parsing & concise JSON round-trip
	rawJSON := `{
		"minecraft": {
			"port": 25565,
			"upstream": "127.0.0.1:25566",
			"server_jar": "/srv/mc/paper.jar",
			"console": true
		}
	}`
	cfg, err := ParseConfigBytes([]byte(rawJSON), "gymbag.json", "/tmp")
	if err != nil {
		t.Fatalf("ParseConfigBytes JSON failed: %v", err)
	}
	if cfg.Minecraft == nil || !cfg.Minecraft.Console {
		t.Fatalf("expected minecraft.console=true, got %+v", cfg.Minecraft)
	}
	concise, err := MarshalConciseJSONConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConciseJSONConfig failed: %v", err)
	}
	if !strings.Contains(string(concise), `"console": true`) {
		t.Fatalf("expected concise JSON to include console: true, got:\n%s", string(concise))
	}

	// 2. Gymbagfile bare `console`, `console on`, `console off` & FormatConfig round-trip
	gf := `
minecraft {
	upstream 127.0.0.1:25566
	server_jar /srv/mc/server.jar
	console
}
`
	gfCfg, err := ParseCaddyfile(gf)
	if err != nil {
		t.Fatalf("ParseCaddyfile minecraft console failed: %v", err)
	}
	if gfCfg.Minecraft == nil || !gfCfg.Minecraft.Console {
		t.Fatalf("expected bare 'console' directive to enable Console, got %+v", gfCfg.Minecraft)
	}
	formatted := FormatConfig(gfCfg)
	if !strings.Contains(formatted, "console on") {
		t.Fatalf("expected FormatConfig to emit 'console on', got:\n%s", formatted)
	}
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("reparse FormatConfig failed: %v", err)
	}
	if reparsed.Minecraft == nil || !reparsed.Minecraft.Console {
		t.Fatalf("expected reparsed Console=true, got %+v", reparsed.Minecraft)
	}

	gfOff, err := ParseCaddyfile("minecraft {\n upstream 127.0.0.1:25566\n console off\n}\n")
	if err != nil {
		t.Fatalf("ParseCaddyfile console off failed: %v", err)
	}
	if gfOff.Minecraft == nil || gfOff.Minecraft.Console {
		t.Fatalf("expected console off to set Console=false, got %+v", gfOff.Minecraft)
	}

	// 3. Reject console inside ssh block
	if _, err := ParseCaddyfile("ssh {\n upstream 127.0.0.1:2222\n console on\n}\n"); err == nil {
		t.Fatalf("expected error when using console directive inside ssh block")
	}

	// 4. Env var GYMBAG_MC_CONSOLE=true
	envMap := map[string]string{
		"GYMBAG_MC_UPSTREAM": "127.0.0.1:25566",
		"GYMBAG_MC_JAR":      "/srv/mc/server.jar",
		"GYMBAG_MC_CONSOLE":  "true",
	}
	envCfg, err := LoadWithEnv(LoadOptions{}, func(k string) string { return envMap[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if envCfg.Minecraft == nil || !envCfg.Minecraft.Console {
		t.Fatalf("expected GYMBAG_MC_CONSOLE=true to enable Console, got %+v", envCfg.Minecraft)
	}

	// 5. GenerateConfig with TCPSpec.Console=true
	genCfg, err := GenerateConfig(GenerateOptions{
		Templates: []string{"minecraft"},
		Minecraft: &TCPSpec{
			Upstream:  "127.0.0.1:25566",
			ServerJar: "/srv/mc/server.jar",
			Console:   true,
		},
	})
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}
	if genCfg.Minecraft == nil || !genCfg.Minecraft.Console {
		t.Fatalf("expected GenerateConfig to copy Console=true, got %+v", genCfg.Minecraft)
	}
}
