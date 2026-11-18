import asyncio
import json
import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import httpx
from fastapi.testclient import TestClient

from gymbag.config import Settings
from gymbag.context import build_context
from gymbag.http import create_proxy_app
from gymbag.waf.challenge import ChallengeService
from gymbag.waf.page import PageRenderer
from gymbag.waf.policy import Action, IncomingRequest, build_engine
from gymbag.waf.proof import accepts, proof_hex
from gymbag.waf.store import MemoryStateStore
from gymbag.waf.submission import ChallengeJSON
from gymbag.waf.tokens import ClearanceTokens

ROOT = Path(__file__).resolve().parents[1]


def make_settings(**overrides) -> Settings:
    values = {
        "domains": ("testserver",),
        "secret": "test-secret",
        "difficulty": 1,
        "cookie_secure": False,
        "rate_limit": 30,
        "tls_mode": "selfsigned",
        "cert_file": Path("missing-cert.pem"),
        "key_file": Path("missing-key.pem"),
    }
    values.update(overrides)
    return Settings(**values)


class ProofTests(unittest.TestCase):
    def test_digest_matches_hashlib_shape(self) -> None:
        digest = proof_hex("gymbag", 0)
        self.assertEqual(len(digest), 64)
        self.assertNotIn(digest[0], "g")

    def test_difficulty_one_is_a_real_prefix(self) -> None:
        nonce = 0
        while not accepts("gymbag", nonce, 1):
            nonce += 1
        self.assertTrue(proof_hex("gymbag", nonce).startswith("0"))
        self.assertFalse(accepts("gymbag", -1, 1))
        self.assertFalse(accepts("gymbag", nonce, 0))


class TokenTests(unittest.TestCase):
    def test_token_is_bound_to_the_client_ip(self) -> None:
        tokens = ClearanceTokens(b"secret", 60)
        token = tokens.issue("203.0.113.5")
        self.assertTrue(tokens.valid(token, "203.0.113.5"))
        self.assertFalse(tokens.valid(token, "203.0.113.6"))
        self.assertFalse(tokens.valid("not-a-token", "203.0.113.5"))
        self.assertFalse(tokens.valid(None, "203.0.113.5"))


class PolicyTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.store = MemoryStateStore()
        self.tokens = ClearanceTokens(b"secret", 60)
        self.engine = build_engine(self.store, self.tokens, "waf_clearance", rate_limit=10, rate_window=60)

    def request(self, path: str, method: str = "GET", cookies: dict[str, str] | None = None) -> IncomingRequest:
        return IncomingRequest(method, path, "", {}, "203.0.113.8", cookies or {})

    async def test_rules_short_circuit_in_order(self) -> None:
        denied = await self.engine.decide(self.request("/../secret"))
        self.assertIs(denied.action, Action.DENY)
        self.assertEqual(denied.reason, "shape")

        options = await self.engine.decide(self.request("/", method="OPTIONS"))
        self.assertIs(options.action, Action.ALLOW)

        acme = await self.engine.decide(self.request("/.well-known/acme-challenge/token"))
        self.assertIs(acme.action, Action.ALLOW)

        missing = await self.engine.decide(self.request("/"))
        self.assertIs(missing.action, Action.CHALLENGE)

        cleared = self.request("/", cookies={"waf_clearance": self.tokens.issue("203.0.113.8")})
        allowed = await self.engine.decide(cleared)
        self.assertIs(allowed.action, Action.ALLOW)

    async def test_rate_limit_denies_before_another_challenge(self) -> None:
        engine = build_engine(self.store, self.tokens, "waf_clearance", rate_limit=2, rate_window=60)
        await engine.decide(self.request("/"))
        await engine.decide(self.request("/"))
        limited = await engine.decide(self.request("/again"))
        self.assertIs(limited.action, Action.DENY)
        self.assertEqual(limited.status_code, 429)


class ChallengeServiceTests(unittest.IsolatedAsyncioTestCase):
    async def test_proof_is_single_use_and_bound_to_the_issuer(self) -> None:
        store = MemoryStateStore()
        service = ChallengeService(store, difficulty=1, ttl_seconds=60)
        challenge_id, difficulty = await service.issue("203.0.113.9")
        nonce = 0
        while not accepts(challenge_id, nonce, difficulty):
            nonce += 1
        wrong = ChallengeJSON(challenge_id=challenge_id, nonce=nonce, verified=True)
        self.assertFalse(await service.verify("198.51.100.1", wrong))
        self.assertTrue(await service.verify("203.0.113.9", wrong))
        self.assertFalse(await service.verify("203.0.113.9", wrong))


class PageTests(unittest.TestCase):
    def test_challenge_page_includes_a_browser_solver(self) -> None:
        page = PageRenderer().challenge("challenge-1", 1)
        html = page.body.decode()
        self.assertIn("pensive_Evil_v3_corrected.png", html)
        self.assertIn("happy_Evil_v3_corrected.png", html)
        self.assertIn("reject_Evil_v3_corrected.png", html)
        match = re.search(r'<script type="application/json" id="gymbag-challenge">(.*?)</script>', html)
        self.assertIsNotNone(match)
        spec = json.loads(match.group(1))
        self.assertEqual(spec["challenge_id"], "challenge-1")
        script = re.search(r"<script>\n(.*)</script>", html, re.DOTALL)
        self.assertIsNotNone(script)
        with tempfile.NamedTemporaryFile("w", suffix=".js", delete=False) as handle:
            handle.write(script.group(1))
            path = handle.name
        try:
            subprocess.run(["node", "--check", path], check=True, capture_output=True)
        finally:
            os.unlink(path)


class SolverTests(unittest.TestCase):
    def test_browser_sha256_matches_hashlib(self) -> None:
        script = r"""
const { createHash } = require("node:crypto");
const { readFileSync } = require("node:fs");
const source = readFileSync(process.argv[1], "utf8");
const { sha256 } = new Function(`${source}; return { sha256 };`)();
for (const sample of ["", "abc", "challenge:0", "Gymbag:42"]) {
  const got = sha256(sample);
  const want = createHash("sha256").update(sample).digest("hex");
  if (got !== want) {
    console.error(sample, got, want);
    process.exit(1);
  }
}
"""
        result = subprocess.run(
            ["node", "-e", script, str(ROOT / "src" / "gymbag" / "waf" / "solver.js")],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)


class AppFlowTests(unittest.TestCase):
    def setUp(self) -> None:
        self.calls = 0

        def handler(request: httpx.Request) -> httpx.Response:
            self.calls += 1
            return httpx.Response(200, text=f"proxied {request.url.path}")

        self.upstream = httpx.AsyncClient(transport=httpx.MockTransport(handler))
        self.settings = make_settings()
        self.ctx = asyncio.run(build_context(self.settings, client=self.upstream))
        self.app = create_proxy_app(self.ctx)

    def tearDown(self) -> None:
        asyncio.run(self.upstream.aclose())

    def test_challenge_blocks_the_proxy_until_the_proof_is_valid(self) -> None:
        with TestClient(self.app) as client:
            blocked = client.get("/")
            self.assertEqual(blocked.status_code, 403)
            self.assertEqual(self.calls, 0)
            self.assertIn("Checking your browser", blocked.text)
            spec = json.loads(
                re.search(
                    r'<script type="application/json" id="gymbag-challenge">(.*?)</script>',
                    blocked.text,
                ).group(1)
            )
            nonce = 0
            while not accepts(spec["challenge_id"], nonce, spec["difficulty"]):
                nonce += 1
            bad = 0
            while accepts(spec["challenge_id"], bad, spec["difficulty"]):
                bad += 1
            rejected = client.post(
                "/.waf/verify",
                json={"challenge_id": spec["challenge_id"], "nonce": bad, "verified": True},
            )
            self.assertEqual(rejected.status_code, 403)
            verified = client.post(
                "/.waf/verify",
                json={"challenge_id": spec["challenge_id"], "nonce": nonce, "verified": True},
            )
            self.assertEqual(verified.status_code, 200)
            self.assertEqual(verified.json(), {"ok": True})
            cleared = client.get("/")
            self.assertEqual(cleared.status_code, 200)
            self.assertEqual(cleared.text, "proxied /")
            self.assertEqual(self.calls, 1)
            replay = client.post(
                "/.waf/verify",
                json={"challenge_id": spec["challenge_id"], "nonce": nonce, "verified": True},
            )
            self.assertEqual(replay.status_code, 403)

    def test_assets_and_docs_follow_the_firewall(self) -> None:
        with TestClient(self.app) as client:
            asset = client.get("/.waf/assets/pensive_Evil_v3_corrected.png")
            self.assertEqual(asset.status_code, 200)
            self.assertEqual(asset.headers["content-type"], "image/png")
            self.assertEqual(self.calls, 0)
            missing = client.get("/.waf/nope")
            self.assertEqual(missing.status_code, 403)
            token = self.ctx.clearance.issue("testclient")
            client.cookies.set("waf_clearance", token)
            docs = client.get("/docs")
            self.assertEqual(docs.text, "proxied /docs")
