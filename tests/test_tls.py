import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import httpx
from cryptography import x509
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.serialization import load_pem_private_key
from fastapi.testclient import TestClient

from gymbag.config import Settings
from gymbag.errors import CertificateError, GymbagError
from gymbag.tls.acme import (
    AcmeClient,
    AcmeHttp01Board,
    b64url_decode,
    build_jws,
    verify_jws_signature,
)
from gymbag.tls.certificates import (
    AcmeCertificateProvider,
    CertificateManager,
    FileCertificateProvider,
    SelfSignedCertificateProvider,
    TlsMaterial,
    build_self_signed,
    certificate_covers,
    certificate_expires_within,
    hsts_enabled,
    load_leaf_certificate,
    name_covers,
)
from gymbag.tls.redirect import create_redirect_app
from tests.test_waf import make_settings

ROOT = Path(__file__).resolve().parents[1]


class NameTests(unittest.TestCase):
    def test_wildcard_covers_one_label(self) -> None:
        self.assertTrue(name_covers("*.example.com", "www.example.com"))
        self.assertFalse(name_covers("*.example.com", "example.com"))
        self.assertFalse(name_covers("*.example.com", "a.b.example.com"))


class CertificateTests(unittest.IsolatedAsyncioTestCase):
    async def test_self_signed_covers_the_domain_and_reports_expiry(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            provider = SelfSignedCertificateProvider(directory, ("localhost",), days=1)
            material = await provider.issue()
            self.assertIsNotNone(material)
            assert material is not None
            cert = load_leaf_certificate(material.cert_path.read_bytes())
            self.assertTrue(certificate_covers(cert, ("localhost",)))
            self.assertTrue(certificate_expires_within(material.cert_path, 30))
            long_lived, _ = build_self_signed(("localhost",), days=90)
            path = directory / "long.pem"
            path.write_bytes(long_lived)
            self.assertFalse(certificate_expires_within(path, 30))

    async def test_file_provider_rejects_a_mismatched_key(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            cert_pem, key_pem = build_self_signed(("localhost",), days=90)
            _, other_key = build_self_signed(("localhost",), days=90)
            cert_path = directory / "cert.pem"
            key_path = directory / "key.pem"
            cert_path.write_bytes(cert_pem)
            key_path.write_bytes(other_key)
            provider = FileCertificateProvider(cert_path, key_path, ("localhost",), 30, None, required=False)
            self.assertIsNone(await provider.issue())
            key_path.write_bytes(key_pem)
            material = await provider.issue()
            self.assertIsNotNone(material)
            assert material is not None
            self.assertEqual(material.source, "file")

    async def test_manager_uses_the_first_provider_that_can_issue(self) -> None:
        class Stub:
            def __init__(self, material: TlsMaterial | None) -> None:
                self.material = material
                self.calls = 0

            async def issue(self) -> TlsMaterial | None:
                self.calls += 1
                return self.material

        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            cert_pem, key_pem = build_self_signed(("localhost",), days=90)
            cert_path = directory / "cert.pem"
            key_path = directory / "key.pem"
            cert_path.write_bytes(cert_pem)
            key_path.write_bytes(key_pem)
            material = TlsMaterial(cert_path, key_path, "file")
            skipped = Stub(None)
            selected = Stub(material)
            manager = CertificateManager([skipped, selected])
            issued = await manager.obtain()
            self.assertEqual(issued.source, "file")
            self.assertEqual(skipped.calls, 1)
            self.assertEqual(selected.calls, 1)
            self.assertFalse(hsts_enabled(make_settings(hsts=None), "file"))
            self.assertTrue(hsts_enabled(make_settings(hsts=None), "acme"))

class RedirectTests(unittest.TestCase):
    def test_http_serves_acme_and_redirects_everything_else(self) -> None:
        board = AcmeHttp01Board()
        board.publish("token", "token.thumbprint")
        app = create_redirect_app(make_settings(https_port=443), board)
        with TestClient(app) as client:
            challenge = client.get("/.well-known/acme-challenge/token", follow_redirects=False)
            self.assertEqual(challenge.status_code, 200)
            self.assertEqual(challenge.text, "token.thumbprint")
            redirected = client.post("/app?x=1", follow_redirects=False)
            self.assertEqual(redirected.status_code, 308)
            self.assertEqual(redirected.headers["location"], "https://testserver/app?x=1")
            custom = client.get("/app", headers={"host": "app.example:8080"}, follow_redirects=False)
            self.assertEqual(custom.status_code, 400)
        custom_port = create_redirect_app(make_settings(https_port=8443), board)
        with TestClient(custom_port) as client:
            response = client.get("/app", follow_redirects=False)
            self.assertEqual(response.headers["location"], "https://testserver:8443/app")


class AcmeTests(unittest.IsolatedAsyncioTestCase):
    async def test_jws_signature_verifies(self) -> None:
        key = ec.generate_private_key(ec.SECP256R1())
        document = build_jws(key, url="https://acme.test/new-acct", nonce="nonce", payload=b"{}", kid=None)
        verify_jws_signature(key, document)

    async def test_http01_order_publishes_the_token_and_saves_the_chain(self) -> None:
        board = AcmeHttp01Board()
        seen: dict[str, object] = {}

        def handler(request: httpx.Request) -> httpx.Response:
            path = request.url.path
            headers = {"Replay-Nonce": f"nonce-{path.strip('/')}"}
            if path == "/directory":
                return httpx.Response(
                    200,
                    json={
                        "newNonce": "https://acme.test/new-nonce",
                        "newAccount": "https://acme.test/new-acct",
                        "newOrder": "https://acme.test/new-order",
                    },
                )
            if path == "/new-nonce":
                return httpx.Response(204, headers=headers)
            if path == "/new-acct":
                return httpx.Response(201, json={}, headers={**headers, "Location": "https://acme.test/acct/1"})
            if path == "/new-order":
                return httpx.Response(
                    201,
                    json={
                        "status": "pending",
                        "authorizations": ["https://acme.test/authz/1"],
                        "finalize": "https://acme.test/finalize/1",
                    },
                    headers={**headers, "Location": "https://acme.test/order/1"},
                )
            if path == "/authz/1":
                if board.lookup("token-1"):
                    return httpx.Response(200, json={"status": "valid"}, headers=headers)
                return httpx.Response(
                    200,
                    json={
                        "status": "pending",
                        "challenges": [
                            {"type": "dns-01", "token": "other", "url": "https://acme.test/dns"},
                            {"type": "http-01", "token": "token-1", "url": "https://acme.test/chall/1"},
                        ],
                    },
                    headers=headers,
                )
            if path == "/chall/1":
                self.assertIsNotNone(board.lookup("token-1"))
                return httpx.Response(200, json={}, headers=headers)
            if path == "/finalize/1":
                body = json.loads(request.content)
                payload = json.loads(b64url_decode(body["payload"]))
                seen["csr"] = b64url_decode(payload["csr"])
                return httpx.Response(
                    200,
                    json={"status": "valid", "certificate": "https://acme.test/cert/1"},
                    headers=headers,
                )
            if path == "/cert/1":
                return httpx.Response(
                    200,
                    content=b"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
                    headers=headers,
                )
            raise AssertionError(path)

        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            http = httpx.AsyncClient(transport=httpx.MockTransport(handler))
            client = AcmeClient("https://acme.test/directory", directory / "account.pem", http)
            try:
                cert_pem, key_pem = await client.issue(("example.com",), "owner@example.com", board)
            finally:
                await client.aclose()
                await http.aclose()
            self.assertIn(b"BEGIN CERTIFICATE", cert_pem)
            self.assertIsInstance(load_pem_private_key(key_pem, password=None), ec.EllipticCurvePrivateKey)
            csr = x509.load_der_x509_csr(seen["csr"])
            names = csr.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
            self.assertIn("example.com", names.get_values_for_type(x509.DNSName))
            provider = AcmeCertificateProvider(
                client,
                board,
                directory,
                ("example.com",),
                "owner@example.com",
                required=True,
            )
            self.assertTrue(provider._eligible())


class SettingsTests(unittest.TestCase):
    def test_public_domains_bind_all_interfaces_unless_overridden(self) -> None:
        saved = {key: os.environ[key] for key in list(os.environ) if key.startswith("GYMBAG_")}
        try:
            for key in saved:
                del os.environ[key]
            local = Settings.from_env()
            self.assertEqual(local.domains, ("localhost",))
            self.assertEqual(local.bind_host, "127.0.0.1")
            self.assertEqual(local.tls_mode, "auto")
            os.environ["GYMBAG_DOMAINS"] = "Example.COM, www.example.com."
            os.environ["GYMBAG_ACME_EMAIL"] = "owner@example.com"
            public = Settings.from_env()
            self.assertEqual(public.domains, ("example.com", "www.example.com"))
            self.assertEqual(public.bind_host, "0.0.0.0")
            os.environ["GYMBAG_BIND"] = "127.0.0.1"
            os.environ["GYMBAG_DIFFICULTY"] = "9"
            with self.assertRaises(GymbagError):
                Settings.from_env()
        finally:
            for key in list(os.environ):
                if key.startswith("GYMBAG_"):
                    del os.environ[key]
            os.environ.update(saved)

    def test_acme_mode_rejects_a_local_name(self) -> None:
        with self.assertRaises(CertificateError):
            AcmeClient._reject_ineligible(("localhost",), "owner@example.com")

    def test_caddyfile_config_parses_single_and_multi_site_blocks(self) -> None:
        simple = Settings.from_caddyfile(
            """
            example.com {
                reverse_proxy http://localhost:5173
            }
            """
        )
        self.assertEqual(simple.domains, ("example.com",))
        self.assertEqual(simple.upstream, "http://localhost:5173")
        self.assertEqual(simple.bind_host, "0.0.0.0")

        multi = Settings.from_caddyfile(
            """
            {
                difficulty 3
                email admin@example.com
            }

            app.example.com, www.example.com {
                reverse_proxy localhost:3000
            }

            api.example.com {
                reverse_proxy :8080
            }
            """
        )
        self.assertEqual(multi.domains, ("app.example.com", "www.example.com", "api.example.com"))
        self.assertEqual(multi.upstream, "http://localhost:3000")
        self.assertEqual(
            multi.upstreams,
            (
                ("app.example.com", "http://localhost:3000"),
                ("www.example.com", "http://localhost:3000"),
                ("api.example.com", "http://localhost:8080"),
            ),
        )
        self.assertEqual(multi.difficulty, 3)
        self.assertEqual(multi.acme_email, "admin@example.com")

        flat = Settings.from_caddyfile(
            """
            example.org
            reverse_proxy http://127.0.0.1:9000
            """
        )
        self.assertEqual(flat.domains, ("example.org",))
        self.assertEqual(flat.upstream, "http://127.0.0.1:9000")

