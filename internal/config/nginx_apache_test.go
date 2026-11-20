package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNginxNativeConfigAndAutoDetect(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	included := filepath.Join(confDir, "site.conf")
	if err := os.WriteFile(included, []byte(`
server {
	listen 80;
	server_name shop.example.com www.shop.example.com;
	return 301 https://$host$request_uri;
}

server {
	listen 443 ssl;
	server_name shop.example.com www.shop.example.com;
	ssl_certificate /etc/ssl/shop.crt;
	ssl_certificate_key /etc/ssl/shop.key;

	add_header Strict-Transport-Security "max-age=31536000";
	allow 10.0.0.0/8;
	deny 192.0.2.50;

	location = /health {
		return 200 "healthy";
	}

	location ^~ /assets/ {
		alias /srv/shop/assets;
		autoindex on;
	}

	location ~ ^/v[0-9]+/ {
		proxy_pass http://127.0.0.1:9100;
	}

	location /api/ {
		proxy_pass http://api_pool/v2/;
	}

	location / {
		root /srv/shop/dist;
		index index.html main.html;
		try_files $uri $uri/ /index.html;
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	mainConf := filepath.Join(dir, "custom.conf")
	if err := os.WriteFile(mainConf, []byte(`
events {
	worker_connections 1024;
}

http {
	upstream api_pool {
		server 127.0.0.1:8081;
		server 127.0.0.1:8082 weight=2;
		server 127.0.0.1:8083 down;
	}

	include conf.d/*.conf;
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// ParseFile must auto-detect nginx format even when filename is custom.conf.
	cfg, err := ParseFile(mainConf)
	if err != nil {
		t.Fatalf("ParseFile(nginx) failed: %v", err)
	}

	// HTTP-to-HTTPS redirect server block is deduplicated; two domains from the HTTPS block remain.
	if len(cfg.Sites) != 2 {
		t.Fatalf("len(Sites) = %d, want 2: %+v", len(cfg.Sites), cfg.Sites)
	}
	s := cfg.Sites[0]
	if s.Domain != "shop.example.com" || cfg.Sites[1].Domain != "www.shop.example.com" {
		t.Fatalf("unexpected domains: %s, %s", s.Domain, cfg.Sites[1].Domain)
	}
	if s.CertFile != "/etc/ssl/shop.crt" || s.KeyFile != "/etc/ssl/shop.key" {
		t.Fatalf("TLS cert/key mismatch: %q / %q", s.CertFile, s.KeyFile)
	}
	if s.Headers["Strict-Transport-Security"] != "max-age=31536000" {
		t.Fatalf("expected HSTS add_header preserved, got %+v", s.Headers)
	}
	if len(s.WAF.Rules) != 2 {
		t.Fatalf("expected 2 allow/deny CustomRules, got %+v", s.WAF.Rules)
	}

	// Check location precedence and proxy_pass URI stripping with named upstream.
	if len(s.Routes) < 5 {
		t.Fatalf("expected 5 routes, got %d: %+v", len(s.Routes), s.Routes)
	}
	if s.Routes[0].Path != "/health" || s.Routes[0].Respond == nil || s.Routes[0].Respond.Body != "healthy" {
		t.Fatalf("expected exact /health route first, got %+v", s.Routes[0])
	}

	var foundAPI, foundAssets, foundRegex, foundRoot bool
	for _, r := range s.Routes {
		switch {
		case r.Path == "/api/*":
			foundAPI = true
			if r.StripPrefix != "/api" {
				t.Fatalf("/api/* StripPrefix = %q, want /api", r.StripPrefix)
			}
			if len(r.Upstreams) != 2 || r.Upstreams[0] != "http://127.0.0.1:8081/v2" || r.Upstreams[1] != "http://127.0.0.1:8082/v2" {
				t.Fatalf("unexpected /api/* resolved upstreams (down server must be skipped): %+v", r.Upstreams)
			}
		case r.Path == "/assets/*":
			foundAssets = true
			if r.Root != "/srv/shop/assets" || r.StripPrefix != "/assets" || !r.Browse {
				t.Fatalf("unexpected /assets/* alias route: %+v", r)
			}
		case r.Path == "~^/v[0-9]+/":
			foundRegex = true
			if len(r.Upstreams) != 1 || r.Upstreams[0] != "http://127.0.0.1:9100" {
				t.Fatalf("unexpected regex proxy route: %+v", r)
			}
		case r.Path == "" || r.Path == "/*":
			if r.Root == "/srv/shop/dist" {
				foundRoot = true
				if r.Fallback != "/index.html" || len(r.Index) != 2 {
					t.Fatalf("unexpected static root route: %+v", r)
				}
			}
		}
	}
	if !foundAPI || !foundAssets || !foundRegex || !foundRoot {
		t.Fatalf("missing nginx routes (api=%t assets=%t regex=%t root=%t) in %+v", foundAPI, foundAssets, foundRegex, foundRoot, s.Routes)
	}
}

func TestParseApacheNativeConfigAndAutoDetect(t *testing.T) {
	dir := t.TempDir()
	sitesDir := filepath.Join(dir, "sites-enabled")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	vhostFile := filepath.Join(sitesDir, "app.conf")
	if err := os.WriteFile(vhostFile, []byte(`
<VirtualHost *:80>
	ServerName portal.example.com
	Redirect permanent / https://portal.example.com/
</VirtualHost>

<IfModule mod_ssl.c>
<VirtualHost *:443>
	ServerName portal.example.com
	ServerAlias www.portal.example.com
	SSLEngine on
	SSLCertificateFile /etc/letsencrypt/live/portal/fullchain.pem
	SSLCertificateKeyFile /etc/letsencrypt/live/portal/privkey.pem

	Header always set X-Content-Type-Options "nosniff"
	Header add X-Proxy "gymbag"
	Header unset X-Powered-By

	Alias /media /srv/portal/media
	<Directory "/srv/portal/media">
		Options Indexes FollowSymLinks
	</Directory>

	<Location "/admin">
		Require ip 10.1.0.0/16
		Require not ip 198.51.100.7
	</Location>

	Redirect 410 /deprecated
	Redirect permanent /docs https://docs.example.com/
	ProxyPass /api http://127.0.0.1:8080/api
	ProxyPassMatch "^/ws/(.*)$" "ws://127.0.0.1:8090/ws"
	ProxyPass / balancer://app_cluster/

	DocumentRoot /srv/portal/public
</VirtualHost>
</IfModule>
`), 0o644); err != nil {
		t.Fatal(err)
	}

	httpdConf := filepath.Join(dir, "my-vhost.conf")
	if err := os.WriteFile(httpdConf, []byte(`
Listen 8080
Listen 8443 https

<Proxy "balancer://app_cluster">
	BalancerMember http://127.0.0.1:7001
	BalancerMember http://127.0.0.1:7002
</Proxy>

IncludeOptional sites-enabled/*.conf
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseFile(httpdConf)
	if err != nil {
		t.Fatalf("ParseFile(apache) failed: %v", err)
	}
	if cfg.HTTPPort != 8080 || cfg.HTTPSPort != 8443 {
		t.Fatalf("ports = %d/%d, want 8080/8443", cfg.HTTPPort, cfg.HTTPSPort)
	}
	if len(cfg.Sites) != 2 {
		t.Fatalf("len(Sites) = %d, want 2: %+v", len(cfg.Sites), cfg.Sites)
	}

	s := cfg.Sites[0]
	if s.Domain != "portal.example.com" || cfg.Sites[1].Domain != "www.portal.example.com" {
		t.Fatalf("unexpected domains: %+v", cfg.Sites)
	}
	if s.CertFile != "/etc/letsencrypt/live/portal/fullchain.pem" {
		t.Fatalf("unexpected CertFile: %q", s.CertFile)
	}
	if s.Headers["X-Content-Type-Options"] != "nosniff" || s.Headers["+X-Proxy"] != "gymbag" {
		t.Fatalf("unexpected Apache headers: %+v", s.Headers)
	}
	if _, ok := s.Headers["-X-Powered-By"]; !ok {
		t.Fatalf("expected Header unset X-Powered-By in headers: %+v", s.Headers)
	}
	if len(s.WAF.Rules) != 2 {
		t.Fatalf("expected 2 Require ip rules, got %+v", s.WAF.Rules)
	}

	if len(s.Upstreams) != 2 || s.Upstreams[0] != "http://127.0.0.1:7001" || s.Upstreams[1] != "http://127.0.0.1:7002" {
		t.Fatalf("expected ProxyPass / balancer://app_cluster to populate s.Upstreams, got %+v", s.Upstreams)
	}

	var foundGone, foundRedir, foundMedia, foundAPI, foundWS bool
	for _, r := range s.Routes {
		switch {
		case r.Path == "/deprecated*" && r.Respond != nil && r.Respond.Status == 410:
			foundGone = true
		case r.Path == "/docs*" && r.Redirect != nil && r.Redirect.Code == 301 && r.Redirect.To == "https://docs.example.com/":
			foundRedir = true
		case r.Path == "/media/*" && r.Root == "/srv/portal/media" && r.StripPrefix == "/media" && r.Browse:
			foundMedia = true
		case r.Path == "/api*" && len(r.Upstreams) == 1 && r.Upstreams[0] == "http://127.0.0.1:8080/api":
			foundAPI = true
		case r.Path == "~^/ws/(.*)$" && len(r.Upstreams) == 1 && r.Upstreams[0] == "http://127.0.0.1:8090/ws":
			foundWS = true
		}
	}
	if !foundGone || !foundRedir || !foundMedia || !foundAPI || !foundWS {
		t.Fatalf("missing Apache routes (gone=%t redir=%t media=%t api=%t ws=%t) in %+v",
			foundGone, foundRedir, foundMedia, foundAPI, foundWS, s.Routes)
	}
}
