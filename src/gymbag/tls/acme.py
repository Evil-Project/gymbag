import asyncio
import base64
import hashlib
import json
import time
from pathlib import Path
from urllib.parse import urljoin

import httpx
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.asymmetric.utils import (
    decode_dss_signature,
    encode_dss_signature,
)
from cryptography.x509.oid import NameOID

from gymbag.errors import CertificateError
from gymbag.hosts import is_public_hostname, normalize_host

_TOKEN_ALPHABET = frozenset("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def b64url_decode(data: str) -> bytes:
    return base64.urlsafe_b64decode(data + "=" * (-len(data) % 4))


class AcmeHttp01Board:
    """Tokens the HTTP listener serves for an in-progress ACME order."""

    def __init__(self) -> None:
        self._tokens: dict[str, str] = {}

    def publish(self, token: str, authorization: str) -> None:
        self._tokens[token] = authorization

    def lookup(self, token: str) -> str | None:
        if not token or any(character not in _TOKEN_ALPHABET for character in token):
            return None
        return self._tokens.get(token)

    def clear(self) -> None:
        self._tokens.clear()


def ec_jwk(key: ec.EllipticCurvePrivateKey) -> dict[str, str]:
    numbers = key.public_key().public_numbers()
    return {
        "crv": "P-256",
        "kty": "EC",
        "x": b64url(numbers.x.to_bytes(32, "big")),
        "y": b64url(numbers.y.to_bytes(32, "big")),
    }


def jwk_thumbprint(key: ec.EllipticCurvePrivateKey) -> str:
    canonical = json.dumps(ec_jwk(key), separators=(",", ":"), sort_keys=True).encode()
    return b64url(hashlib.sha256(canonical).digest())


def key_authorization(token: str, key: ec.EllipticCurvePrivateKey) -> str:
    return f"{token}.{jwk_thumbprint(key)}"


def build_jws(
    key: ec.EllipticCurvePrivateKey,
    *,
    url: str,
    nonce: str,
    payload: bytes | None,
    kid: str | None,
) -> dict[str, str]:
    header: dict[str, object] = {"alg": "ES256", "nonce": nonce, "url": url}
    if kid is None:
        header["jwk"] = ec_jwk(key)
    else:
        header["kid"] = kid
    protected = b64url(json.dumps(header, separators=(",", ":")).encode())
    encoded_payload = "" if payload is None else b64url(payload)
    signing_input = f"{protected}.{encoded_payload}".encode()
    der = key.sign(signing_input, ec.ECDSA(hashes.SHA256()))
    raw_r, raw_s = decode_dss_signature(der)
    signature = b64url(raw_r.to_bytes(32, "big") + raw_s.to_bytes(32, "big"))
    return {"protected": protected, "payload": encoded_payload, "signature": signature}


def verify_jws_signature(key: ec.EllipticCurvePrivateKey, document: dict[str, str]) -> None:
    signing_input = f"{document['protected']}.{document['payload']}".encode()
    raw = b64url_decode(document["signature"])
    der = encode_dss_signature(int.from_bytes(raw[:32], "big"), int.from_bytes(raw[32:], "big"))
    key.public_key().verify(der, signing_input, ec.ECDSA(hashes.SHA256()))


def load_account_key(path: Path) -> ec.EllipticCurvePrivateKey:
    if path.exists():
        key = serialization.load_pem_private_key(path.read_bytes(), password=None)
        if not isinstance(key, ec.EllipticCurvePrivateKey):
            raise CertificateError("ACME account key must be an EC private key")
        return key
    path.parent.mkdir(parents=True, exist_ok=True)
    key = ec.generate_private_key(ec.SECP256R1())
    path.write_bytes(
        key.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        )
    )
    path.chmod(0o600)
    return key


def _problem(response: httpx.Response) -> str:
    detail = response.text[:300]
    try:
        payload = response.json()
    except json.JSONDecodeError:
        payload = None
    if isinstance(payload, dict):
        detail = str(payload.get("detail") or payload.get("type") or detail)
    return f"ACME request failed ({response.status_code}): {detail}"


class AcmeClient:
    """Requests a certificate with the ACME HTTP-01 challenge."""

    def __init__(
        self,
        directory_url: str,
        account_key_path: Path,
        http: httpx.AsyncClient | None = None,
    ) -> None:
        self._directory_url = directory_url
        self._account_key_path = account_key_path
        self._owns_http = http is None
        self._http = http or httpx.AsyncClient(
            timeout=30,
            follow_redirects=False,
            headers={"User-Agent": "gymbag/0.1.0"},
        )
        self._directory: dict[str, str] | None = None
        self._nonce: str | None = None
        self._kid: str | None = None
        self._key: ec.EllipticCurvePrivateKey | None = None
        self._closed = False

    async def issue(
        self,
        domains: tuple[str, ...],
        email: str,
        board: AcmeHttp01Board,
    ) -> tuple[bytes, bytes]:
        self._reject_ineligible(domains, email)
        self._key = load_account_key(self._account_key_path)
        await self._load_directory()
        await self._register(email)
        order, order_url = await self._new_order(domains)
        for authorization_url in order["authorizations"]:
            await self._satisfy_http01(authorization_url, board)
        key = ec.generate_private_key(ec.SECP256R1())
        csr = _certificate_request(key, domains)
        finalized = await self._signed_json(
            order["finalize"],
            json.dumps({"csr": b64url(csr)}, separators=(",", ":")).encode(),
        )
        if finalized.get("status") != "valid":
            finalized = await self._poll(order_url)
        if finalized.get("status") != "valid":
            raise CertificateError("ACME order failed")
        certificate_url = finalized.get("certificate")
        if not certificate_url:
            raise CertificateError("ACME order did not include a certificate")
        downloaded = await self._signed(certificate_url, None)
        if downloaded.status_code >= 400 or b"BEGIN CERTIFICATE" not in downloaded.content:
            raise CertificateError(_problem(downloaded))
        key_pem = key.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        )
        return downloaded.content, key_pem

    async def aclose(self) -> None:
        if self._closed:
            return
        self._closed = True
        if self._owns_http:
            await self._http.aclose()

    async def _new_order(self, domains: tuple[str, ...]) -> tuple[dict, str]:
        directory = await self._load_directory()
        response = await self._signed(
            directory["newOrder"],
            json.dumps(
                {"identifiers": [{"type": "dns", "value": domain} for domain in domains]},
                separators=(",", ":"),
            ).encode(),
        )
        if response.status_code >= 400:
            raise CertificateError(_problem(response))
        order = response.json()
        if not isinstance(order, dict):
            raise CertificateError("ACME order response was not an object")
        location = response.headers.get("Location")
        if not location:
            raise CertificateError("ACME order response did not include a location")
        return order, urljoin(directory["newOrder"], location)

    async def _register(self, email: str) -> None:
        directory = await self._load_directory()
        account_payload: dict[str, object] = {"termsOfServiceAgreed": True}
        if email:
            account_payload["contact"] = [f"mailto:{email}"]
        response = await self._signed(
            directory["newAccount"],
            json.dumps(
                account_payload,
                separators=(",", ":"),
            ).encode(),
        )
        if response.status_code not in {200, 201}:
            raise CertificateError(_problem(response))
        location = response.headers.get("Location")
        if not location:
            raise CertificateError("ACME account response did not include a location")
        self._kid = urljoin(directory["newAccount"], location)

    async def _satisfy_http01(self, authorization_url: str, board: AcmeHttp01Board) -> None:
        authorization = await self._signed_json(authorization_url, None)
        if authorization.get("status") == "valid":
            return
        challenge = next(
            (item for item in authorization.get("challenges", []) if item.get("type") == "http-01"),
            None,
        )
        if challenge is None or "token" not in challenge or "url" not in challenge:
            raise CertificateError("ACME authorization did not offer an HTTP-01 challenge")
        if self._key is None:
            raise CertificateError("ACME account key is not loaded")
        board.publish(challenge["token"], key_authorization(challenge["token"], self._key))
        await self._signed_json(challenge["url"], b"{}")
        completed = await self._poll(authorization_url)
        if completed.get("status") != "valid":
            raise CertificateError("ACME HTTP-01 challenge failed")

    async def _poll(self, url: str) -> dict:
        deadline = time.monotonic() + 60
        while True:
            payload = await self._signed_json(url, None)
            if payload.get("status") in {"valid", "invalid"}:
                return payload
            if time.monotonic() >= deadline:
                raise CertificateError("timed out waiting for the ACME order")
            await asyncio.sleep(1)

    async def _signed_json(self, url: str, payload: bytes | None) -> dict:
        response = await self._signed(url, payload)
        if response.status_code >= 400:
            raise CertificateError(_problem(response))
        if not response.content:
            return {}
        data = response.json()
        if not isinstance(data, dict):
            raise CertificateError("ACME response was not an object")
        return data

    async def _signed(self, url: str, payload: bytes | None) -> httpx.Response:
        if self._key is None:
            raise CertificateError("ACME account key is not loaded")
        last: httpx.Response | None = None
        for _ in range(2):
            if not self._nonce:
                await self._refresh_nonce()
            document = build_jws(
                self._key,
                url=url,
                nonce=self._nonce or "",
                payload=payload,
                kid=self._kid,
            )
            try:
                last = await self._http.post(
                    url,
                    content=json.dumps(document, separators=(",", ":")),
                    headers={"Content-Type": "application/jose+json"},
                )
            except httpx.HTTPError as exc:
                raise CertificateError(f"ACME request failed: {exc}") from exc
            self._nonce = last.headers.get("Replay-Nonce")
            if last.status_code == 400 and "badNonce" in last.text:
                continue
            return last
        if last is None:
            raise CertificateError("ACME request was not sent")
        return last

    async def _refresh_nonce(self) -> None:
        directory = await self._load_directory()
        try:
            response = await self._http.get(directory["newNonce"])
        except httpx.HTTPError as exc:
            raise CertificateError(f"ACME request failed: {exc}") from exc
        nonce = response.headers.get("Replay-Nonce")
        if not nonce:
            raise CertificateError("ACME directory did not return a nonce")
        self._nonce = nonce

    async def _load_directory(self) -> dict[str, str]:
        if self._directory is None:
            try:
                response = await self._http.get(self._directory_url)
            except httpx.HTTPError as exc:
                raise CertificateError(f"ACME request failed: {exc}") from exc
            if response.status_code >= 400:
                raise CertificateError(_problem(response))
            data = response.json()
            if not isinstance(data, dict):
                raise CertificateError("ACME directory was not an object")
            self._directory = {str(key): str(value) for key, value in data.items()}
        return self._directory

    @staticmethod
    def _reject_ineligible(domains: tuple[str, ...], email: str) -> None:
        if email and ("@" not in email or email.startswith("@") or email.endswith("@")):
            raise CertificateError("GYMBAG_ACME_EMAIL must be a valid email address")
        reasons = []
        for domain in domains:
            if domain.startswith("*."):
                reasons.append(f"{domain} needs DNS-01; Gymbag completes HTTP-01 only")
            elif not is_public_hostname(domain):
                reasons.append(f"{domain} is not a public hostname")
        if reasons:
            raise CertificateError("; ".join(reasons))


def _certificate_request(key: ec.EllipticCurvePrivateKey, domains: tuple[str, ...]) -> bytes:
    names = [x509.DNSName(normalize_host(domain)) for domain in domains]
    csr = (
        x509.CertificateSigningRequestBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, names[0].value[:64])]))
        .add_extension(x509.SubjectAlternativeName(names), critical=False)
        .sign(key, hashes.SHA256())
    )
    return csr.public_bytes(serialization.Encoding.DER)
