import json
import re
from pathlib import Path

from fastapi.responses import HTMLResponse

ASSETS_DIR = Path(__file__).resolve().parents[1] / "assets"
PENSIVE_ASSET = "pensive_Evil_v3_corrected.png"
HAPPY_ASSET = "happy_Evil_v3_corrected.png"
REJECT_ASSET = "reject_Evil_v3_corrected.png"
ALLOWED_ASSETS = frozenset({PENSIVE_ASSET, HAPPY_ASSET, REJECT_ASSET})

_SOLVER = Path(__file__).with_name("solver.js").read_text(encoding="utf-8")
_FRONTEND_CANDIDATES = (
    Path(__file__).resolve().parents[3] / "frontend" / "index.html",
    Path(__file__).resolve().parents[1] / "frontend" / "index.html",
    Path.cwd() / "frontend" / "index.html",
)
_CHALLENGE_TAG_RE = re.compile(
    r'(<script type="application/json" id="gymbag-challenge">).*?(</script>)',
    re.DOTALL,
)
_PAGE_HEADERS = {
    "cache-control": "no-store",
    "referrer-policy": "no-referrer",
    "x-content-type-options": "nosniff",
    "content-security-policy": (
        "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; "
        "script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'"
    ),
}
_COPY = {
    "rate": ("Slow down", "Too many requests. Wait a moment and try again."),
    "shape": ("Request blocked", "Gymbag blocked this request."),
}

_FALLBACK_CHALLENGE_HTML = """<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Making sure you're not a bot! — Gymbag</title>
</head>
<body>
  <main>
    <img id="portrait" alt="Gymbag is checking this browser" src="/.waf/assets/pensive_Evil_v3_corrected.png">
    <h1 id="title">Making sure you're not a bot!</h1>
    <p id="detail">Checking your browser before accessing the site.</p>
    <p id="meta">Running proof-of-work check</p>
    <span id="spinner"></span>
    <div id="progress-bar"></div>
    <button id="retry" hidden type="button">Try again</button>
  </main>
  <script type="application/json" id="gymbag-challenge">{}</script>
  <script>
/* SOLVER_PLACEHOLDER */
const spec = JSON.parse(document.getElementById("gymbag-challenge").textContent);
const portrait = document.getElementById("portrait");
const title = document.getElementById("title");
const detail = document.getElementById("detail");
const retry = document.getElementById("retry");
function showFailure() {
  portrait.src = spec.images.fail;
  title.textContent = "Check failed";
  detail.textContent = "The proof was rejected. Try the check again.";
  retry.hidden = false;
}
retry.addEventListener("click", () => location.reload());
solveGymbag(spec.challenge_id, spec.difficulty, () => {
  detail.textContent = "Still confirming this browser.";
}).then(async (nonce) => {
  const response = await fetch("/.waf/verify", {
    method: "POST",
    headers: { "content-type": "application/json" },
    credentials: "same-origin",
    cache: "no-store",
    body: JSON.stringify({ challenge_id: spec.challenge_id, nonce: nonce, verified: true })
  });
  if (!response.ok) {
    showFailure();
    return;
  }
  portrait.src = spec.images.pass;
  title.textContent = "You're in";
  detail.textContent = "Continuing to the site.";
  setTimeout(() => location.reload(), 350);
}).catch(() => showFailure());
  </script>
</body>
</html>
"""


def _load_challenge_template() -> str:
    for candidate in _FRONTEND_CANDIDATES:
        if candidate.is_file():
            return candidate.read_text(encoding="utf-8")
    return _FALLBACK_CHALLENGE_HTML


def asset_file(name: str) -> Path | None:
    if name not in ALLOWED_ASSETS:
        return None
    path = ASSETS_DIR / name
    if not path.is_file():
        return None
    return path


class PageRenderer:
    def __init__(self, template_path: Path | None = None) -> None:
        if template_path is not None and template_path.is_file():
            self._template = template_path.read_text(encoding="utf-8")
        else:
            self._template = _load_challenge_template()

    def challenge(self, challenge_id: str, difficulty: int) -> HTMLResponse:
        payload = json.dumps(
            {
                "challenge_id": challenge_id,
                "difficulty": difficulty,
                "images": {
                    "pass": f"/.waf/assets/{HAPPY_ASSET}",
                    "fail": f"/.waf/assets/{REJECT_ASSET}",
                },
            },
            separators=(",", ":"),
        ).replace("<", "\\u003c")
        html = _CHALLENGE_TAG_RE.sub(
            lambda match: f"{match.group(1)}{payload}{match.group(2)}",
            self._template,
            count=1,
        )
        html = html.replace("/* SOLVER_PLACEHOLDER */", _SOLVER).replace("SOLVER_PLACEHOLDER", _SOLVER)
        return HTMLResponse(html, status_code=403, headers=_PAGE_HEADERS)

    def reject(self, reason: str, status_code: int) -> HTMLResponse:
        title, detail = _COPY.get(reason, ("Request blocked", "Gymbag blocked this request."))
        retry = ""
        if status_code == 429:
            retry = '<a class="retry-btn" href="">Try again</a>'
        html = f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{title} — Gymbag</title>
  <style>
    :root {{
      color-scheme: light dark;
      --bg: #fdfbf7;
      --surface: #f4efe6;
      --border: #e2d9c8;
      --text: #1e1b18;
      --muted: #6e665e;
      --accent: #b81d33;
      --shadow: 0 10px 30px rgba(20, 16, 12, 0.06);
    }}
    @media (prefers-color-scheme: dark) {{
      :root {{
        --bg: #070708;
        --surface: #121216;
        --border: #26262e;
        --text: #f6f3ee;
        --muted: #a9a297;
        --accent: #d7263d;
        --shadow: 0 14px 36px rgba(0, 0, 0, 0.45);
      }}
    }}
    * {{ box-sizing: border-box; }}
    body {{
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
    }}
    main {{
      width: min(30rem, 100%);
      margin: auto;
      text-align: center;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 1rem;
    }}
    img {{
      width: min(250px, 65vw);
      height: auto;
      filter: drop-shadow(0 8px 18px rgba(0, 0, 0, 0.15));
    }}
    h1 {{ margin: 0; font-size: clamp(1.45rem, 4vw, 1.85rem); font-weight: 700; }}
    .status-card {{
      width: 100%;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 1.1rem 1.25rem;
      box-shadow: var(--shadow);
    }}
    p {{ margin: 0; color: var(--muted); }}
    .retry-btn {{
      display: inline-block;
      margin-top: 0.9rem;
      color: var(--text);
      text-decoration: none;
      border: 1px solid var(--accent);
      border-radius: 999px;
      padding: 0.45rem 1.1rem;
      font-size: 0.88rem;
      font-weight: 500;
    }}
    footer {{
      margin-top: 2rem;
      font-size: 0.8rem;
      color: var(--muted);
    }}
    footer strong {{ color: var(--text); }}
  </style>
</head>
<body>
  <main>
    <h1>{title}</h1>
    <img alt="Gymbag blocked this request" src="/.waf/assets/{REJECT_ASSET}">
    <section class="status-card">
      <p>{detail}</p>
      {retry}
    </section>
  </main>
  <footer>
    Protected by <strong>Gymbag</strong>
  </footer>
</body>
</html>
"""
        headers = dict(_PAGE_HEADERS)
        if status_code == 429:
            headers["retry-after"] = "60"
        return HTMLResponse(html, status_code=status_code, headers=headers)
