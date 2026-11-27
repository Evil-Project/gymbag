package waf

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"regexp"
	"strings"
)

//go:embed assets/*
var embeddedAssets embed.FS

const (
	PensiveAsset = "pensive_Evil_v3_corrected.png"
	HappyAsset   = "happy_Evil_v3_corrected.png"
	RejectAsset  = "reject_Evil_v3_corrected.png"
)

type cachedAsset struct {
	data []byte
	etag string
}

var preloadedAssets map[string]cachedAsset

func init() {
	preloadedAssets = make(map[string]cachedAsset, 3)
	for _, name := range []string{PensiveAsset, HappyAsset, RejectAsset} {
		if raw, err := embeddedAssets.ReadFile("assets/" + name); err == nil {
			sum := sha256.Sum256(raw)
			preloadedAssets[name] = cachedAsset{
				data: raw,
				etag: `"` + hex.EncodeToString(sum[:12]) + `"`,
			}
		}
	}
}

var challengeTagRE = regexp.MustCompile(`(?s)(<script type="application/json" id="gymbag-challenge">).*?(</script>)`)

var pageSecurityHeaders = map[string]string{
	"Cache-Control":          "no-store",
	"Referrer-Policy":        "no-referrer",
	"X-Content-Type-Options": "nosniff",
	"Content-Security-Policy": "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; " +
		"script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'",
}

// AssetData retrieves a pre-cached embedded mascot PNG asset and its ETag if allowed.
func AssetData(name string) (data []byte, etag string, ok bool) {
	d, _, e, found := AssetDataWithType(name)
	return d, e, found
}

// AssetDataWithType retrieves a pre-cached embedded mascot asset with its MIME Content-Type and ETag.
func AssetDataWithType(name string) (data []byte, contentType, etag string, ok bool) {
	a, exists := preloadedAssets[name]
	if !exists {
		return nil, "", "", false
	}
	return a.data, "image/png", a.etag, true
}

// SolverJS returns the embedded browser proof-of-work SHA-256 solver JavaScript source.
func SolverJS() string {
	b, _ := embeddedAssets.ReadFile("assets/solver.js")
	return string(b)
}

// Renderer renders HTML challenge and rejection responses using embedded templates and solver JS.
type Renderer struct {
	prefix       string
	suffix       string
	hasTag       bool
	fullFallback string

	mascot MascotSources
	csp    string

	rejectForbidden []byte
	rejectRateLimit []byte
}

// NewRenderer constructs a Renderer using the embedded HTML template and default mascot images.
func NewRenderer(customTemplatePath string) *Renderer {
	return NewRendererWithMascot(customTemplatePath, DefaultMascotSources())
}

// NewRendererWithMascot constructs a Renderer using the embedded HTML template (or customTemplatePath if non-empty)
// and custom mascot image URLs.
func NewRendererWithMascot(customTemplatePath string, mascot MascotSources) *Renderer {
	mascot = mascot.withDefaults()
	solverBytes, _ := embeddedAssets.ReadFile("assets/solver.js")
	tplBytes, _ := embeddedAssets.ReadFile("assets/index.html")
	tpl := string(tplBytes)
	if customTemplatePath != "" {
		if diskBytes, err := os.ReadFile(customTemplatePath); err == nil {
			tpl = string(diskBytes)
		}
	}
	solverJS := string(solverBytes)
	tpl = strings.ReplaceAll(tpl, "/* SOLVER_PLACEHOLDER */", solverJS)
	tpl = strings.ReplaceAll(tpl, "SOLVER_PLACEHOLDER", solverJS)
	if defSrc := "/.waf/assets/" + PensiveAsset; mascot.PensiveURL != defSrc {
		tpl = strings.ReplaceAll(tpl, defSrc, html.EscapeString(mascot.PensiveURL))
	}

	r := &Renderer{
		fullFallback:    tpl,
		mascot:          mascot,
		csp:             BuildPageCSP(mascot),
		rejectForbidden: []byte(buildRejectHTML("rule", http.StatusForbidden, mascot.RejectURL)),
		rejectRateLimit: []byte(buildRejectHTML("rate", http.StatusTooManyRequests, mascot.RejectURL)),
	}
	if loc := challengeTagRE.FindStringSubmatchIndex(tpl); len(loc) == 6 {
		// loc[2]:loc[3] is opening <script ...>, loc[4]:loc[5] is closing </script>
		r.prefix = tpl[:loc[3]]
		r.suffix = tpl[loc[4]:]
		r.hasTag = true
	}
	return r
}

func (r *Renderer) effectiveCSP() string {
	if r.csp != "" {
		return r.csp
	}
	return BuildPageCSP(r.mascot.withDefaults())
}

type challengeSpecJSON struct {
	ChallengeID string            `json:"challenge_id"`
	Difficulty  int               `json:"difficulty"`
	Images      map[string]string `json:"images"`
}

// RenderChallengeHTML returns the rendered proof-of-work HTML document.
func (r *Renderer) RenderChallengeHTML(challengeID string, difficulty int) string {
	if !r.hasTag {
		return r.fullFallback
	}
	m := r.mascot.withDefaults()
	rawJSON, _ := json.Marshal(challengeSpecJSON{
		ChallengeID: challengeID,
		Difficulty:  difficulty,
		Images: map[string]string{
			"pass":            m.HappyURL,
			"fail":            m.RejectURL,
			"default_pensive": "/.waf/assets/" + PensiveAsset,
			"default_pass":    "/.waf/assets/" + HappyAsset,
			"default_fail":    "/.waf/assets/" + RejectAsset,
		},
	})
	escapedJSON := strings.ReplaceAll(string(rawJSON), "<", `\u003c`)

	var b strings.Builder
	b.Grow(len(r.prefix) + len(escapedJSON) + len(r.suffix))
	b.WriteString(r.prefix)
	b.WriteString(escapedJSON)
	b.WriteString(r.suffix)
	return b.String()
}

// WriteChallenge writes the 403 HTML proof-of-work page to w.
func (r *Renderer) WriteChallenge(w http.ResponseWriter, challengeID string, difficulty int) {
	bodyHTML := r.RenderChallengeHTML(challengeID, difficulty)
	hdr := w.Header()
	for k, v := range pageSecurityHeaders {
		hdr.Set(k, v)
	}
	hdr.Set("Content-Security-Policy", r.effectiveCSP())
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(bodyHTML))
}

// RenderRejectHTML returns the rendered block or rate-limit HTML document.
func (r *Renderer) RenderRejectHTML(reason string, statusCode int) string {
	return buildRejectHTML(reason, statusCode, r.mascot.withDefaults().RejectURL)
}

func buildRejectHTML(reason string, statusCode int, rejectImgURL string) string {
	if strings.TrimSpace(rejectImgURL) == "" {
		rejectImgURL = "/.waf/assets/" + RejectAsset
	}
	title := "Request blocked"
	detail := "Gymbag blocked this request."
	if reason == "rate" {
		title = "Slow down"
		detail = "Too many requests. Wait a moment and try again."
	}
	retry := ""
	if statusCode == http.StatusTooManyRequests {
		retry = `<a class="retry-btn" href="">Try again</a>`
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s — Gymbag</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #fdfbf7;
      --surface: #f4efe6;
      --border: #e2d9c8;
      --text: #1e1b18;
      --muted: #6e665e;
      --accent: #b81d33;
      --shadow: 0 10px 30px rgba(20, 16, 12, 0.06);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #070708;
        --surface: #121216;
        --border: #26262e;
        --text: #f6f3ee;
        --muted: #a9a297;
        --accent: #d7263d;
        --shadow: 0 14px 36px rgba(0, 0, 0, 0.45);
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: space-between;
      padding: 2rem 1rem 1.5rem;
      background: var(--bg);
      color: var(--text);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      line-height: 1.5;
    }
    main {
      width: min(30rem, 100%%);
      margin: auto;
      text-align: center;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 1rem;
    }
    img {
      width: min(250px, 65vw);
      height: auto;
      filter: drop-shadow(0 8px 18px rgba(0, 0, 0, 0.15));
    }
    h1 { margin: 0; font-size: clamp(1.45rem, 4vw, 1.85rem); font-weight: 700; }
    .status-card {
      width: 100%%;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 1.1rem 1.25rem;
      box-shadow: var(--shadow);
    }
    p { margin: 0; color: var(--muted); }
    .retry-btn {
      display: inline-block;
      margin-top: 0.9rem;
      color: var(--text);
      text-decoration: none;
      border: 1px solid var(--accent);
      border-radius: 999px;
      padding: 0.45rem 1.1rem;
      font-size: 0.88rem;
      font-weight: 500;
    }
    footer {
      margin-top: 2rem;
      font-size: 0.8rem;
      color: var(--muted);
    }
    footer strong { color: var(--text); }
  </style>
</head>
<body>
  <main>
    <h1>%s</h1>
    <img alt="Gymbag blocked this request" src="%s">
    <section class="status-card">
      <p>%s</p>
      %s
    </section>
  </main>
  <footer>
    Protected by <strong>Gymbag</strong>
  </footer>
</body>
</html>
`, title, title, html.EscapeString(rejectImgURL), detail, retry)
}

// WriteReject writes the HTML block/rate-limit page to w using pre-rendered static HTML bytes.
func (r *Renderer) WriteReject(w http.ResponseWriter, reason string, statusCode int) {
	if statusCode == 0 {
		statusCode = http.StatusForbidden
	}
	hdr := w.Header()
	for k, v := range pageSecurityHeaders {
		hdr.Set(k, v)
	}
	hdr.Set("Content-Security-Policy", r.effectiveCSP())
	rejectImgURL := r.mascot.withDefaults().RejectURL
	var body []byte
	if statusCode == http.StatusTooManyRequests {
		hdr.Set("Retry-After", "60")
		if reason == "rate" && len(r.rejectRateLimit) > 0 {
			body = r.rejectRateLimit
		} else {
			body = []byte(buildRejectHTML(reason, statusCode, rejectImgURL))
		}
	} else if reason != "rate" && len(r.rejectForbidden) > 0 {
		body = r.rejectForbidden
	} else {
		body = []byte(buildRejectHTML(reason, statusCode, rejectImgURL))
	}
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
