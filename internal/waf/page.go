package waf

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

var allowedAssets = map[string]struct{}{
	PensiveAsset: {},
	HappyAsset:   {},
	RejectAsset:  {},
}

var challengeTagRE = regexp.MustCompile(`(?s)(<script type="application/json" id="gymbag-challenge">).*?(</script>)`)

var pageSecurityHeaders = map[string]string{
	"Cache-Control":          "no-store",
	"Referrer-Policy":        "no-referrer",
	"X-Content-Type-Options": "nosniff",
	"Content-Security-Policy": "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; " +
		"script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'",
}

// AssetData retrieves an embedded mascot PNG asset and its ETag if allowed.
func AssetData(name string) (data []byte, etag string, ok bool) {
	if _, allowed := allowedAssets[name]; !allowed {
		return nil, "", false
	}
	raw, err := embeddedAssets.ReadFile("assets/" + name)
	if err != nil {
		return nil, "", false
	}
	sum := sha256.Sum256(raw)
	return raw, `"` + hex.EncodeToString(sum[:12]) + `"`, true
}

// Renderer renders HTML challenge and rejection responses using embedded templates and solver JS.
type Renderer struct {
	template string
	solverJS string
}

// NewRenderer constructs a Renderer using the embedded HTML template (or customTemplatePath if non-empty).
func NewRenderer(customTemplatePath string) *Renderer {
	solverBytes, _ := embeddedAssets.ReadFile("assets/solver.js")
	tplBytes, _ := embeddedAssets.ReadFile("assets/index.html")
	tpl := string(tplBytes)
	if customTemplatePath != "" {
		if diskBytes, err := os.ReadFile(customTemplatePath); err == nil {
			tpl = string(diskBytes)
		}
	}
	return &Renderer{
		template: tpl,
		solverJS: string(solverBytes),
	}
}

// RenderChallengeHTML returns the rendered proof-of-work HTML document.
func (r *Renderer) RenderChallengeHTML(challengeID string, difficulty int) string {
	spec := map[string]any{
		"challenge_id": challengeID,
		"difficulty":   difficulty,
		"images": map[string]string{
			"pass": "/.waf/assets/" + HappyAsset,
			"fail": "/.waf/assets/" + RejectAsset,
		},
	}
	rawJSON, _ := json.Marshal(spec)
	escapedJSON := strings.ReplaceAll(string(rawJSON), "<", `\u003c`)

	replaced := false
	html := challengeTagRE.ReplaceAllStringFunc(r.template, func(match string) string {
		if replaced {
			return match
		}
		replaced = true
		sub := challengeTagRE.FindStringSubmatch(match)
		if len(sub) == 3 {
			return sub[1] + escapedJSON + sub[2]
		}
		return match
	})
	html = strings.ReplaceAll(html, "/* SOLVER_PLACEHOLDER */", r.solverJS)
	html = strings.ReplaceAll(html, "SOLVER_PLACEHOLDER", r.solverJS)
	return html
}

// WriteChallenge writes the 403 HTML proof-of-work page to w.
func (r *Renderer) WriteChallenge(w http.ResponseWriter, challengeID string, difficulty int) {
	html := r.RenderChallengeHTML(challengeID, difficulty)
	for k, v := range pageSecurityHeaders {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(html))
}

// RenderRejectHTML returns the rendered block or rate-limit HTML document.
func (r *Renderer) RenderRejectHTML(reason string, statusCode int) string {
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
    <img alt="Gymbag blocked this request" src="/.waf/assets/%s">
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
`, title, title, RejectAsset, detail, retry)
}

// WriteReject writes the HTML block/rate-limit page to w.
func (r *Renderer) WriteReject(w http.ResponseWriter, reason string, statusCode int) {
	if statusCode == 0 {
		statusCode = http.StatusForbidden
	}
	html := r.RenderRejectHTML(reason, statusCode)
	for k, v := range pageSecurityHeaders {
		w.Header().Set(k, v)
	}
	if statusCode == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(html))
}
