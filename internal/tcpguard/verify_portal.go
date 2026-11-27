package tcpguard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"gymbag/internal/store"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

var portalSecurityHeaders = map[string]string{
	"Cache-Control":          "no-store",
	"Referrer-Policy":        "no-referrer",
	"X-Content-Type-Options": "nosniff",
	"Content-Security-Policy": "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; " +
		"script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'",
}

// VerifyPortal serves the shared HTTPS proof-of-work verification web page for SSH and Minecraft.
type VerifyPortal struct {
	gateStore      store.GateStore
	challenges     waf.ChallengeManager
	acmeBoard      *tlsmanager.HTTP01Board
	trustedProxies []netip.Prefix
	solverJS       string

	mu        sync.RWMutex
	onCleared func(service, clientIP string)
	mascot    waf.MascotSources
	csp       string
	assets    *waf.AssetStore
}

// SetMascot updates the custom mascot image URLs and asset store used by the verification portal.
func (p *VerifyPortal) SetMascot(sources waf.MascotSources, assets *waf.AssetStore) {
	def := waf.DefaultMascotSources()
	if strings.TrimSpace(sources.PensiveURL) == "" {
		sources.PensiveURL = def.PensiveURL
	}
	if strings.TrimSpace(sources.HappyURL) == "" {
		sources.HappyURL = def.HappyURL
	}
	if strings.TrimSpace(sources.RejectURL) == "" {
		sources.RejectURL = def.RejectURL
	}
	p.mu.Lock()
	p.mascot = sources
	p.csp = waf.BuildPageCSP(sources)
	p.assets = assets
	p.mu.Unlock()
}

func (p *VerifyPortal) getMascot() (waf.MascotSources, string, *waf.AssetStore) {
	p.mu.RLock()
	m := p.mascot
	csp := p.csp
	assets := p.assets
	p.mu.RUnlock()
	if m.PensiveURL == "" && m.HappyURL == "" && m.RejectURL == "" {
		m = waf.DefaultMascotSources()
		csp = waf.BuildPageCSP(m)
	}
	return m, csp, assets
}

// SetOnCleared registers a callback invoked after a client IP passes verification
// and its clearance has been persisted.
func (p *VerifyPortal) SetOnCleared(fn func(service, clientIP string)) {
	p.mu.Lock()
	p.onCleared = fn
	p.mu.Unlock()
}

func (p *VerifyPortal) getOnCleared() func(service, clientIP string) {
	p.mu.RLock()
	fn := p.onCleared
	p.mu.RUnlock()
	return fn
}

// NewVerifyPortal constructs a VerifyPortal backed by gateStore and challenges.
func NewVerifyPortal(
	gateStore store.GateStore,
	challenges waf.ChallengeManager,
	acmeBoard *tlsmanager.HTTP01Board,
	trustedProxies []netip.Prefix,
) *VerifyPortal {
	def := waf.DefaultMascotSources()
	return &VerifyPortal{
		gateStore:      gateStore,
		challenges:     challenges,
		acmeBoard:      acmeBoard,
		trustedProxies: trustedProxies,
		solverJS:       waf.SolverJS(),
		mascot:         def,
		csp:            waf.BuildPageCSP(def),
	}
}

// IsGatePath reports whether path belongs to the SSH/Minecraft verification portal
// (used when sharing the HTTPS port with web reverse-proxy sites).
func IsGatePath(path string) bool {
	return strings.HasPrefix(path, "/v/") ||
		strings.HasPrefix(path, "/verify/") ||
		path == "/.waf/gate/verify"
}

func (p *VerifyPortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hdr := w.Header()
	hdr.Set("Server", "gymbag")
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	if tok, ok := strings.CutPrefix(path, "/.well-known/acme-challenge/"); ok {
		if p.acmeBoard == nil || tok == "" || strings.Contains(tok, "/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		authz, found := p.acmeBoard.Lookup(tok)
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hdr.Set("Content-Type", "text/plain; charset=utf-8")
		hdr.Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, authz)
		}
		return
	}

	if name, ok := strings.CutPrefix(path, "/.waf/assets/"); ok {
		p.servePortalAsset(w, r, name)
		return
	}
	if name, ok := strings.CutPrefix(path, "/.waf/v1/assets/"); ok {
		p.servePortalAsset(w, r, name)
		return
	}

	if path == "/.waf/gate/verify" {
		p.handleGateVerify(w, r)
		return
	}

	if token, ok := extractTicketToken(path); ok {
		p.handleVerifyPage(w, r, token)
		return
	}

	mascot, csp, _ := p.getMascot()
	if path == "/" {
		writePortalStatusHTML(w, http.StatusOK, "Gymbag Verification Portal", "Protected by Gymbag. Connect via SSH or Minecraft to receive your verification link.", mascot.HappyURL, csp)
		return
	}

	writePortalStatusHTML(w, http.StatusNotFound, "Not found", "Invalid verification link. Reconnect to obtain a fresh link.", mascot.RejectURL, csp)
}

func extractTicketToken(path string) (string, bool) {
	for _, prefix := range []string{"/v/", "/verify/"} {
		if idx := strings.Index(path, prefix); idx != -1 {
			rest := strings.TrimSpace(path[idx+len(prefix):])
			rest = strings.Trim(rest, "/")
			if rest != "" && !strings.Contains(rest, "/") {
				return rest, true
			}
		}
	}
	return "", false
}

func (p *VerifyPortal) handleVerifyPage(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mascot, csp, _ := p.getMascot()
	if p.gateStore == nil || p.challenges == nil {
		writePortalStatusHTML(w, http.StatusServiceUnavailable, "Service unavailable", "Verification store is not configured.", mascot.RejectURL, csp)
		return
	}

	ticket, err := p.gateStore.GetGateTicket(r.Context(), token)
	if (err != nil || ticket == nil) && len(token) <= 16 {
		if lower := strings.ToLower(token); lower != token {
			if t2, err2 := p.gateStore.GetGateTicket(r.Context(), lower); err2 == nil && t2 != nil {
				ticket = t2
				err = nil
				token = lower
			}
		}
	}
	if err != nil || ticket == nil {
		writePortalStatusHTML(w, http.StatusGone, "Link expired or invalid", "This verification link has expired or was already used. Reconnect to obtain a new verification link.", mascot.RejectURL, csp)
		return
	}

	browserIP := p.resolveClientIP(r)
	chalID, diff, err := p.challenges.Issue(r.Context(), "gate:"+ticket.Service, browserIP, ticket.Difficulty, 300)
	if err != nil {
		writePortalStatusHTML(w, http.StatusInternalServerError, "Unable to issue challenge", "Please reconnect and try again.", mascot.RejectURL, csp)
		return
	}

	for k, v := range portalSecurityHeaders {
		w.Header().Set(k, v)
	}
	if csp != "" {
		w.Header().Set("Content-Security-Policy", csp)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}

	_, _ = io.WriteString(w, p.renderVerifyHTML(token, chalID, diff, ticket, mascot))
}

func (p *VerifyPortal) handleGateVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writePortalJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "POST is required"})
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writePortalJSON(w, http.StatusUnsupportedMediaType, map[string]any{"ok": false, "error": "Content-Type must be application/json"})
		return
	}
	if p.gateStore == nil || p.challenges == nil {
		writePortalJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "verification store unavailable"})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		writePortalJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid request body"})
		return
	}

	var payload struct {
		Token       string `json:"token"`
		ChallengeID string `json:"challenge_id"`
		Nonce       int64  `json:"nonce"`
		Verified    bool   `json:"verified"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if payload.Token == "" || payload.ChallengeID == "" || !payload.Verified || payload.Nonce < 0 || payload.Nonce > 50_000_000 {
		writePortalJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": "invalid verification payload"})
		return
	}

	ticket, err := p.gateStore.GetGateTicket(r.Context(), payload.Token)
	if err != nil || ticket == nil {
		writePortalJSON(w, http.StatusGone, map[string]any{"ok": false, "error": "verification link expired or already used"})
		return
	}

	browserIP := p.resolveClientIP(r)
	accepted, verifyErr := p.challenges.Verify(r.Context(), "gate:"+ticket.Service, browserIP, waf.Submission{
		ChallengeID: payload.ChallengeID,
		Nonce:       payload.Nonce,
		Verified:    payload.Verified,
	})
	if verifyErr != nil || !accepted {
		writePortalJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "proof rejected"})
		return
	}

	_ = p.gateStore.DeleteGateTicket(r.Context(), payload.Token)
	clearTTL := ticket.ClearTTL
	if clearTTL <= 0 {
		clearTTL = 86400
	}
	if err := p.gateStore.SaveClearance(r.Context(), ticket.Service, ticket.ClientIP, clearTTL); err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "failed to save clearance"})
		return
	}
	if cb := p.getOnCleared(); cb != nil {
		cb(ticket.Service, ticket.ClientIP)
	}

	w.Header().Set("Cache-Control", "no-store")
	writePortalJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    ticket.Service,
		"client_ip":  ticket.ClientIP,
		"expires_in": clearTTL,
	})
}

func (p *VerifyPortal) resolveClientIP(r *http.Request) string {
	peerIP := extractRemoteIPString(r.RemoteAddr)
	if len(p.trustedProxies) > 0 && isTrustedPrefixIP(peerIP, p.trustedProxies) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				cand := strings.TrimSpace(parts[i])
				if cand != "" && !isTrustedPrefixIP(cand, p.trustedProxies) {
					return cand
				}
			}
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
	}
	return peerIP
}

func extractRemoteIPString(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.IndexByte(trimmed, ']'); end > 0 {
			return trimmed[1:end]
		}
	}
	if colon := strings.LastIndexByte(trimmed, ':'); colon > 0 {
		if strings.IndexByte(trimmed[:colon], ':') == -1 {
			return trimmed[:colon]
		}
	}
	return strings.Trim(trimmed, "[]")
}

func isTrustedPrefixIP(ipStr string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func (p *VerifyPortal) renderVerifyHTML(token, challengeID string, difficulty int, ticket *store.GateTicket, mascot waf.MascotSources) string {
	serviceLabel := "SSH"
	reconnectHint := "You may now close this tab and reconnect via SSH."
	if ticket.Service == "minecraft" {
		serviceLabel = "Minecraft"
		reconnectHint = "You may now return to Minecraft — you will be redirected to the server automatically (or click Reconnect on older versions)."
	}

	targetLine := fmt.Sprintf("Authorizing %s access for IP %s", serviceLabel, ticket.ClientIP)
	if ticket.Service == "minecraft" && ticket.Username != "" {
		targetLine = fmt.Sprintf("Authorizing Minecraft player %s (%s)", ticket.Username, ticket.ClientIP)
	}

	specObj := map[string]any{
		"token":          token,
		"challenge_id":   challengeID,
		"difficulty":     difficulty,
		"service":        serviceLabel,
		"reconnect_hint": reconnectHint,
		"images": map[string]string{
			"pass": mascot.HappyURL,
			"fail": mascot.RejectURL,
		},
	}
	specRaw, _ := json.Marshal(specObj)
	escapedSpec := strings.ReplaceAll(string(specRaw), "<", `\u003c`)

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s Verification — Gymbag</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #fdfbf7; --surface: #f4efe6; --border: #e2d9c8; --text: #1e1b18;
      --muted: #6e665e; --accent: #b81d33; --accent-soft: rgba(184, 29, 51, 0.14);
      --success: #2a7f4f; --shadow: 0 10px 30px rgba(20, 16, 12, 0.06);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #070708; --surface: #121216; --border: #26262e; --text: #f6f3ee;
        --muted: #a9a297; --accent: #d7263d; --accent-soft: rgba(215, 38, 61, 0.18);
        --success: #3ca668; --shadow: 0 14px 36px rgba(0, 0, 0, 0.45);
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0; min-height: 100vh; display: flex; flex-direction: column;
      align-items: center; justify-content: space-between; padding: 2rem 1rem 1.5rem;
      background: var(--bg); color: var(--text);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      line-height: 1.5;
    }
    main {
      width: min(30rem, 100%%); margin: auto; text-align: center;
      display: flex; flex-direction: column; align-items: center; gap: 1rem;
    }
    h1 { margin: 0; font-size: clamp(1.4rem, 4vw, 1.8rem); font-weight: 700; }
    .mascot { width: min(240px, 60vw); height: auto; filter: drop-shadow(0 8px 18px rgba(0,0,0,0.15)); }
    .status-card {
      width: 100%%; background: var(--surface); border: 1px solid var(--border);
      border-radius: 14px; padding: 1.1rem 1.25rem; box-shadow: var(--shadow);
      display: flex; flex-direction: column; gap: 0.75rem;
    }
    .status-row { display: flex; align-items: center; justify-content: center; gap: 0.6rem; font-weight: 500; }
    .spinner {
      width: 1.05rem; height: 1.05rem; border: 2.5px solid var(--accent-soft);
      border-top-color: var(--accent); border-radius: 50%%; animation: spin 0.8s linear infinite; flex-shrink: 0;
    }
    .spinner.done { border-color: var(--success); background: var(--success); animation: none; }
    .spinner.failed { border-color: var(--accent); background: var(--accent); animation: none; }
    @keyframes spin { to { transform: rotate(360deg); } }
    .progress-track { width: 100%%; height: 8px; background: var(--border); border-radius: 999px; overflow: hidden; }
    .progress-bar { height: 100%%; width: 32%%; background: var(--accent); border-radius: 999px; animation: indet 1.6s ease-in-out infinite; }
    .progress-bar.complete { width: 100%%; background: var(--success); animation: none; }
    .progress-bar.error { width: 100%%; background: var(--accent); animation: none; }
    @keyframes indet { 0%% { transform: translateX(-100%%); } 50%% { transform: translateX(150%%); } 100%% { transform: translateX(320%%); } }
    .meta { margin: 0; font-size: 0.84rem; color: var(--muted); }
    .retry-btn {
      align-self: center; background: transparent; color: var(--text); border: 1px solid var(--accent);
      border-radius: 999px; padding: 0.45rem 1.1rem; font: inherit; font-size: 0.88rem; cursor: pointer;
    }
    footer { margin-top: 2rem; font-size: 0.8rem; color: var(--muted); }
    footer strong { color: var(--text); }
  </style>
</head>
<body>
  <main>
    <h1 id="title">%s Verification</h1>
    <img id="portrait" class="mascot" alt="Gymbag verification" src="%s">
    <section class="status-card" aria-live="polite">
      <div class="status-row">
        <span id="spinner" class="spinner" aria-hidden="true"></span>
        <span id="detail">%s</span>
      </div>
      <div class="progress-track"><div id="progress-bar" class="progress-bar"></div></div>
      <p id="meta" class="meta">Running proof-of-work check · Difficulty %d</p>
      <button id="retry" class="retry-btn" hidden type="button">Try again</button>
    </section>
    <noscript><p>JavaScript is required to pass this verification check.</p></noscript>
  </main>
  <footer>Protected by <strong>Gymbag</strong></footer>
  <script type="application/json" id="gymbag-gate-challenge">%s</script>
  <script>
%s
const spec = JSON.parse(document.getElementById("gymbag-gate-challenge").textContent);
const portrait = document.getElementById("portrait");
const title = document.getElementById("title");
const detail = document.getElementById("detail");
const meta = document.getElementById("meta");
const spinner = document.getElementById("spinner");
const progressBar = document.getElementById("progress-bar");
const retry = document.getElementById("retry");

function showFailure(msg) {
  portrait.src = spec.images.fail;
  title.textContent = "Verification failed";
  detail.textContent = msg || "The proof was rejected. Reconnect to obtain a fresh link.";
  spinner.className = "spinner failed";
  progressBar.className = "progress-bar error";
  meta.textContent = "Check failed";
  retry.hidden = false;
}
retry.addEventListener("click", () => location.reload());

if (typeof solveGymbag === "function") {
  solveGymbag(spec.challenge_id, spec.difficulty, (nonce) => {
    meta.textContent = "Running proof-of-work check · Difficulty " + spec.difficulty + " · " + (nonce / 1000).toFixed(0) + "k hashes";
  }).then(async (nonce) => {
    const resp = await fetch("/.waf/gate/verify", {
      method: "POST",
      headers: { "content-type": "application/json" },
      cache: "no-store",
      body: JSON.stringify({
        token: spec.token,
        challenge_id: spec.challenge_id,
        nonce: nonce,
        verified: true
      })
    });
    if (!resp.ok) {
      showFailure();
      return;
    }
    portrait.src = spec.images.pass;
    title.textContent = spec.service + " unlocked!";
    detail.textContent = spec.reconnect_hint;
    spinner.className = "spinner done";
    progressBar.className = "progress-bar complete";
    meta.textContent = "Proof accepted · Access cleared";
  }).catch(() => showFailure());
}
  </script>
</body>
</html>`,
		html.EscapeString(serviceLabel),
		html.EscapeString(serviceLabel),
		html.EscapeString(mascot.PensiveURL),
		html.EscapeString(targetLine),
		difficulty,
		escapedSpec,
		p.solverJS,
	)
}

func writePortalStatusHTML(w http.ResponseWriter, status int, title, detail, imgURL, csp string) {
	if strings.TrimSpace(imgURL) == "" {
		imgURL = "/.waf/assets/" + waf.RejectAsset
	}
	for k, v := range portalSecurityHeaders {
		w.Header().Set(k, v)
	}
	if csp != "" {
		w.Header().Set("Content-Security-Policy", csp)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, fmt.Sprintf(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s — Gymbag</title>
<style>
:root{color-scheme:light dark;--bg:#fdfbf7;--surface:#f4efe6;--border:#e2d9c8;--text:#1e1b18;--muted:#6e665e}
@media(prefers-color-scheme:dark){:root{--bg:#070708;--surface:#121216;--border:#26262e;--text:#f6f3ee;--muted:#a9a297}}
body{margin:0;min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:2rem 1rem;background:var(--bg);color:var(--text);font-family:system-ui,-apple-system,sans-serif;text-align:center}
img{width:min(220px,60vw);margin:1rem 0}
.card{max-width:28rem;background:var(--surface);border:1px solid var(--border);border-radius:14px;padding:1.1rem 1.25rem}
p{margin:0;color:var(--muted)}
</style></head>
<body><h1>%s</h1><img alt="Gymbag" src="%s"><div class="card"><p>%s</p></div></body></html>`,
		html.EscapeString(title),
		html.EscapeString(title),
		html.EscapeString(imgURL),
		html.EscapeString(detail),
	))
}

func (p *VerifyPortal) servePortalAsset(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, _, assets := p.getMascot()
	var (
		data        []byte
		contentType string
		etag        string
		ok          bool
	)
	if assets != nil {
		data, contentType, etag, ok = assets.Lookup(name)
	} else {
		data, contentType, etag, ok = waf.AssetDataWithType(name)
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if contentType == "" {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", etag)
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func writePortalJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
