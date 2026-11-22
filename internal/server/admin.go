package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/host"
	"gymbag/internal/intel"
	"gymbag/internal/mcserver"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

// TLSStatus describes active TLS material, SSL provider configuration, HSTS state, and supported ACME providers.
type TLSStatus struct {
	Mode               string                      `json:"mode"`
	Source             string                      `json:"source"`
	Provider           string                      `json:"provider"`
	ConfiguredProvider string                      `json:"configured_provider,omitempty"`
	CertPath           string                      `json:"cert_path,omitempty"`
	KeyPath            string                      `json:"key_path,omitempty"`
	Issuer             string                      `json:"issuer,omitempty"`
	Subject            string                      `json:"subject,omitempty"`
	DNSNames           []string                    `json:"dns_names,omitempty"`
	SerialNumber       string                      `json:"serial_number,omitempty"`
	NotBefore          *time.Time                  `json:"not_before,omitempty"`
	NotAfter           *time.Time                  `json:"not_after,omitempty"`
	HSTS               bool                        `json:"hsts"`
	Providers          []tlsmanager.CAProviderInfo `json:"providers"`
}

// ConfigController abstracts live configuration, custom rules, remote rules, TLS status/renewal,
// and threat-intelligence inspection and mutation for the REST Admin API (DIP/ISP).
type ConfigController interface {
	Snapshot() config.Config
	ReplaceConfig(next config.Config) error
	TLSSource() string
	TLSStatus() TLSStatus
	RenewTLS(ctx context.Context) (TLSStatus, error)
	IntelStatus() intel.Status
	RefreshIntel(ctx context.Context) intel.Status
	RemoteRulesStatus() waf.RemoteRulesStatus
	RefreshRemoteRules(ctx context.Context) waf.RemoteRulesStatus
}

// SitePatchPayload defines partial update fields for PATCH /api/v1/sites/{domain}.
type SitePatchPayload struct {
	Upstreams    *[]string             `json:"upstreams,omitempty"`
	Routes       *[]config.RouteConfig `json:"routes,omitempty"`
	Headers      *map[string]string    `json:"headers,omitempty"`
	Protect      *bool                 `json:"protect,omitempty"`
	VerifyLevel  *config.VerifyLevel   `json:"verify_level,omitempty"`
	Difficulty   *int                  `json:"difficulty,omitempty"`
	RateLimit    *int                  `json:"rate_limit,omitempty"`
	RateWindow   *int                  `json:"rate_window,omitempty"`
	ChallengeTTL *int                  `json:"challenge_ttl,omitempty"`
	ClearanceTTL *int                  `json:"clearance_ttl,omitempty"`
	CookieName   *string               `json:"cookie_name,omitempty"`
	CookieSecure *bool                 `json:"cookie_secure,omitempty"`
	DefaultRules *bool                 `json:"default_rules,omitempty"`
	Rules        *[]config.CustomRule  `json:"rules,omitempty"`
	RemoteRules  *[]string             `json:"remote_rules,omitempty"`
	RuleSources  *[]config.RuleSource  `json:"rule_sources,omitempty"`
	CertFile     *string               `json:"cert_file,omitempty"`
	KeyFile      *string               `json:"key_file,omitempty"`
}

// NewAdminHandler returns a Caddy-like RESTful management API handler rooted at /api/v1.
func NewAdminHandler(ctrl ConfigController) http.Handler {
	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		path := strings.TrimRight(r.URL.Path, "/")
		if path == "" {
			path = "/"
		}

		switch {
		case path == "/api/v1/health":
			handleAdminHealth(w, r, ctrl)
		case path == "/api/v1/config":
			handleAdminConfig(w, r, ctrl)
		case path == "/api/v1/tls":
			handleAdminTLSStatus(w, r, ctrl)
		case path == "/api/v1/tls/renew":
			handleAdminTLSRenew(w, r, ctrl)
		case path == "/api/v1/intel":
			handleAdminIntelStatus(w, r, ctrl)
		case path == "/api/v1/intel/refresh":
			handleAdminIntelRefresh(w, r, ctrl)
		case path == "/api/v1/rules/remote":
			handleAdminRemoteRulesStatus(w, r, ctrl)
		case path == "/api/v1/rules/remote/refresh":
			handleAdminRemoteRulesRefresh(w, r, ctrl)
		case path == "/api/v1/sites":
			handleAdminSitesCollection(w, r, ctrl)
		case strings.HasPrefix(path, "/api/v1/sites/"):
			rest := strings.TrimPrefix(path, "/api/v1/sites/")
			parts := strings.Split(rest, "/")
			domain := host.Normalize(parts[0])
			if domain == "" {
				writeAdminError(w, http.StatusNotFound, "not_found", "resource not found")
				return
			}
			if len(parts) == 1 {
				handleAdminSiteItem(w, r, ctrl, domain)
				return
			}
			if len(parts) == 2 && parts[1] == "rules" {
				handleAdminSiteRulesCollection(w, r, ctrl, domain)
				return
			}
			if len(parts) == 3 && parts[1] == "rules" && parts[2] != "" {
				handleAdminSiteRuleDelete(w, r, ctrl, domain, parts[2])
				return
			}
			writeAdminError(w, http.StatusNotFound, "not_found", "resource not found")
		default:
			writeAdminError(w, http.StatusNotFound, "not_found", "endpoint not found")
		}
	})
	return ServerHeaderMiddleware("gymbag", core)
}

// MinecraftServerStatusProvider is an optional interface implemented by Runtime to report
// managed Minecraft server subprocess health in /api/v1/health.
type MinecraftServerStatusProvider interface {
	MinecraftServerStatus() *mcserver.Status
}

func handleAdminHealth(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	snap := ctrl.Snapshot()
	intelSt := ctrl.IntelStatus()
	remoteSt := ctrl.RemoteRulesStatus()
	resp := map[string]any{
		"status":            "ok",
		"sites_count":       len(snap.Sites),
		"ssh_enabled":       snap.SSHEnabled(),
		"minecraft_enabled": snap.MinecraftEnabled(),
		"tls_source":        ctrl.TLSSource(),
		"threat_entries":    intelSt.ThreatEntries,
		"ai_entries":        intelSt.AIEntries,
		"vpn_entries":       intelSt.VPNEntries,
		"remote_rules":      remoteSt.TotalRules,
	}
	if prov, ok := ctrl.(MinecraftServerStatusProvider); ok {
		if mcSt := prov.MinecraftServerStatus(); mcSt != nil {
			resp["minecraft_server"] = mcSt
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleAdminTLSStatus(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	writeJSON(w, http.StatusOK, ctrl.TLSStatus())
}

func handleAdminTLSRenew(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST is required")
		return
	}
	status, err := ctrl.RenewTLS(r.Context())
	if err != nil {
		if tlsmanager.IsCertificateError(err) {
			writeAdminError(w, http.StatusUnprocessableEntity, "certificate_error", err.Error())
			return
		}
		writeAdminError(w, http.StatusInternalServerError, "renewal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func handleAdminIntelStatus(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	writeJSON(w, http.StatusOK, ctrl.IntelStatus())
}

func handleAdminIntelRefresh(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST is required")
		return
	}
	writeJSON(w, http.StatusOK, ctrl.RefreshIntel(r.Context()))
}

func handleAdminRemoteRulesStatus(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	writeJSON(w, http.StatusOK, ctrl.RemoteRulesStatus())
}

func handleAdminRemoteRulesRefresh(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST is required")
		return
	}
	writeJSON(w, http.StatusOK, ctrl.RefreshRemoteRules(r.Context()))
}

func handleAdminConfig(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, ctrl.Snapshot().Redacted())
	case http.MethodPut:
		if !requireJSONContentType(w, r) {
			return
		}
		next := config.Default()
		next.Sites = nil
		if !decodeStrictJSON(w, r.Body, &next) {
			return
		}
		cur := ctrl.Snapshot()
		if next.Secret == "" || next.Secret == "***REDACTED***" {
			next.Secret = cur.Secret
		}
		if next.TLS.KeyPassword == "" || next.TLS.KeyPassword == "***REDACTED***" {
			next.TLS.KeyPassword = cur.TLS.KeyPassword
		}
		if next.TLS.EABHMACKey == "" || next.TLS.EABHMACKey == "***REDACTED***" {
			next.TLS.EABHMACKey = cur.TLS.EABHMACKey
		}
		if err := ctrl.ReplaceConfig(next); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ctrl.Snapshot().Redacted())
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or PUT is required")
	}
}

func handleAdminSitesCollection(w http.ResponseWriter, r *http.Request, ctrl ConfigController) {
	switch r.Method {
	case http.MethodGet:
		snap := ctrl.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"items": snap.Sites,
			"total": len(snap.Sites),
		})
	case http.MethodPost:
		if !requireJSONContentType(w, r) {
			return
		}
		snap := ctrl.Snapshot()
		site := config.SiteConfig{
			Protect: true,
			WAF:     snap.Defaults,
		}
		if !decodeStrictJSON(w, r.Body, &site) {
			return
		}
		if err := site.Validate(); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		for _, existing := range snap.Sites {
			if existing.Domain == site.Domain {
				writeAdminError(w, http.StatusConflict, "site_exists", "site already exists: "+site.Domain)
				return
			}
		}
		snap.Sites = append(slices.Clone(snap.Sites), site)
		if err := ctrl.ReplaceConfig(snap); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		w.Header().Set("Location", "/api/v1/sites/"+site.Domain)
		writeJSON(w, http.StatusCreated, site)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST is required")
	}
}

func findSiteIndex(snap config.Config, domain string) int {
	for i, s := range snap.Sites {
		if s.Domain == domain {
			return i
		}
	}
	return -1
}

func handleAdminSiteItem(w http.ResponseWriter, r *http.Request, ctrl ConfigController, domain string) {
	snap := ctrl.Snapshot()
	idx := findSiteIndex(snap, domain)
	if idx == -1 {
		writeAdminError(w, http.StatusNotFound, "not_found", "site not found: "+domain)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, snap.Sites[idx])

	case http.MethodPut:
		if !requireJSONContentType(w, r) {
			return
		}
		updated := config.SiteConfig{
			Domain:  domain,
			Protect: true,
			WAF:     snap.Defaults,
		}
		if !decodeStrictJSON(w, r.Body, &updated) {
			return
		}
		updated.Domain = domain
		if err := updated.Validate(); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		sites := slices.Clone(snap.Sites)
		sites[idx] = updated
		snap.Sites = sites
		if err := ctrl.ReplaceConfig(snap); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodPatch:
		if !requireJSONContentType(w, r) {
			return
		}
		var patch SitePatchPayload
		if !decodeStrictJSON(w, r.Body, &patch) {
			return
		}
		target := snap.Sites[idx]
		if patch.Upstreams != nil {
			target.Upstreams = append([]string(nil), (*patch.Upstreams)...)
		}
		if patch.Routes != nil {
			cloned := make([]config.RouteConfig, len(*patch.Routes))
			for i, rc := range *patch.Routes {
				cloned[i] = rc.Clone()
			}
			target.Routes = cloned
		}
		if patch.Headers != nil {
			cloned := make(map[string]string, len(*patch.Headers))
			for k, v := range *patch.Headers {
				cloned[k] = v
			}
			target.Headers = cloned
		}
		if patch.Protect != nil {
			target.Protect = *patch.Protect
		}
		if patch.VerifyLevel != nil {
			target.WAF.VerifyLevel = *patch.VerifyLevel
		}
		if patch.Difficulty != nil {
			target.WAF.Difficulty = *patch.Difficulty
		}
		if patch.RateLimit != nil {
			target.WAF.RateLimit = *patch.RateLimit
		}
		if patch.RateWindow != nil {
			target.WAF.RateWindow = *patch.RateWindow
		}
		if patch.ChallengeTTL != nil {
			target.WAF.ChallengeTTL = *patch.ChallengeTTL
		}
		if patch.ClearanceTTL != nil {
			target.WAF.ClearanceTTL = *patch.ClearanceTTL
		}
		if patch.CookieName != nil {
			target.WAF.CookieName = *patch.CookieName
		}
		if patch.CookieSecure != nil {
			target.WAF.CookieSecure = *patch.CookieSecure
		}
		if patch.DefaultRules != nil {
			v := *patch.DefaultRules
			target.WAF.DefaultRules = &v
		}
		if patch.Rules != nil {
			cloned := make([]config.CustomRule, len(*patch.Rules))
			for i, cr := range *patch.Rules {
				cloned[i] = cr.Clone()
			}
			target.WAF.Rules = cloned
		}
		if patch.RemoteRules != nil {
			target.WAF.RemoteRules = append([]string(nil), (*patch.RemoteRules)...)
		}
		if patch.RuleSources != nil {
			target.WAF.RuleSources = append([]config.RuleSource(nil), (*patch.RuleSources)...)
		}
		if patch.CertFile != nil {
			target.CertFile = *patch.CertFile
		}
		if patch.KeyFile != nil {
			target.KeyFile = *patch.KeyFile
		}
		if err := target.Validate(); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		sites := slices.Clone(snap.Sites)
		sites[idx] = target
		snap.Sites = sites
		if err := ctrl.ReplaceConfig(snap); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, target)

	case http.MethodDelete:
		if len(snap.Sites) <= 1 && !snap.SSHEnabled() && !snap.MinecraftEnabled() {
			writeAdminError(w, http.StatusConflict, "last_site", "cannot delete the last configured site")
			return
		}
		sites := slices.Delete(slices.Clone(snap.Sites), idx, idx+1)
		snap.Sites = sites
		if err := ctrl.ReplaceConfig(snap); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, PUT, PATCH, DELETE")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET, PUT, PATCH, or DELETE is required")
	}
}

func handleAdminSiteRulesCollection(w http.ResponseWriter, r *http.Request, ctrl ConfigController, domain string) {
	snap := ctrl.Snapshot()
	idx := findSiteIndex(snap, domain)
	if idx == -1 {
		writeAdminError(w, http.StatusNotFound, "not_found", "site not found: "+domain)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules := snap.Sites[idx].WAF.Rules
		if rules == nil {
			rules = []config.CustomRule{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": rules,
			"total": len(rules),
		})

	case http.MethodPost:
		if !requireJSONContentType(w, r) {
			return
		}
		var rule config.CustomRule
		if !decodeStrictJSON(w, r.Body, &rule) {
			return
		}
		if err := rule.Validate(); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		sites := slices.Clone(snap.Sites)
		target := sites[idx]
		target.WAF.Rules = append(slices.Clone(target.WAF.Rules), rule)
		sites[idx] = target
		snap.Sites = sites
		if err := ctrl.ReplaceConfig(snap); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		ruleIdx := len(target.WAF.Rules) - 1
		w.Header().Set("Location", "/api/v1/sites/"+domain+"/rules/"+strconv.Itoa(ruleIdx))
		writeJSON(w, http.StatusCreated, rule)

	case http.MethodPut:
		if !requireJSONContentType(w, r) {
			return
		}
		var rules []config.CustomRule
		if !decodeStrictJSON(w, r.Body, &rules) {
			return
		}
		for i := range rules {
			if err := rules[i].Validate(); err != nil {
				writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
				return
			}
		}
		sites := slices.Clone(snap.Sites)
		target := sites[idx]
		target.WAF.Rules = rules
		sites[idx] = target
		snap.Sites = sites
		if err := ctrl.ReplaceConfig(snap); err != nil {
			writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": rules,
			"total": len(rules),
		})

	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET, POST, or PUT is required")
	}
}

func handleAdminSiteRuleDelete(w http.ResponseWriter, r *http.Request, ctrl ConfigController, domain, idOrName string) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed", "DELETE is required")
		return
	}
	snap := ctrl.Snapshot()
	idx := findSiteIndex(snap, domain)
	if idx == -1 {
		writeAdminError(w, http.StatusNotFound, "not_found", "site not found: "+domain)
		return
	}
	rules := slices.Clone(snap.Sites[idx].WAF.Rules)
	removeIdx := -1
	if parsedIdx, err := strconv.Atoi(idOrName); err == nil && parsedIdx >= 0 && parsedIdx < len(rules) {
		removeIdx = parsedIdx
	} else {
		for i, cr := range rules {
			if cr.Name != "" && cr.Name == idOrName {
				removeIdx = i
				break
			}
		}
	}
	if removeIdx == -1 {
		writeAdminError(w, http.StatusNotFound, "not_found", "rule not found: "+idOrName)
		return
	}
	rules = slices.Delete(rules, removeIdx, removeIdx+1)
	sites := slices.Clone(snap.Sites)
	target := sites[idx]
	target.WAF.Rules = rules
	sites[idx] = target
	snap.Sites = sites
	if err := ctrl.ReplaceConfig(snap); err != nil {
		writeAdminError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "application/json") {
		writeAdminError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	return true
}

func decodeStrictJSON(w http.ResponseWriter, body io.Reader, dst any) bool {
	raw, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		writeAdminError(w, http.StatusBadRequest, "bad_request", "request body must be valid JSON")
		return false
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeAdminError(w, http.StatusBadRequest, "bad_request", err.Error())
		return false
	}
	return true
}

func writeAdminError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": msg,
		},
	})
}
