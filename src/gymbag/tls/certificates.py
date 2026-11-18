import ipaddress
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Protocol

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

from gymbag.config import Settings
from gymbag.errors import CertificateError
from gymbag.hosts import is_public_hostname, normalize_host
from gymbag.tls.acme import AcmeClient, AcmeHttp01Board


@dataclass(frozen=True)
class TlsMaterial:
    cert_path: Path
    key_path: Path
    source: str


class CertificateProvider(Protocol):
    async def issue(self) -> TlsMaterial | None: ...


def load_leaf_certificate(pem: bytes) -> x509.Certificate:
    marker = b"-----END CERTIFICATE-----"
    end = pem.find(marker)
    if end == -1:
        raise CertificateError("certificate file does not contain a PEM certificate")
    leaf = pem[: end + len(marker)]
    return x509.load_pem_x509_certificate(leaf)


def certificate_names(cert: x509.Certificate) -> set[str]:
    names: set[str] = set()
    try:
        san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    except x509.ExtensionNotFound:
        san = None
    if san is not None:
        names.update(name.lower() for name in san.get_values_for_type(x509.DNSName))
        names.update(str(ip) for ip in san.get_values_for_type(x509.IPAddress))
    for attribute in cert.subject.get_attributes_for_oid(NameOID.COMMON_NAME):
        if isinstance(attribute.value, str):
            names.add(attribute.value.lower())
    return names


def name_covers(pattern: str, domain: str) -> bool:
    candidate = pattern.lower().rstrip(".")
    host = normalize_host(domain)
    if candidate == host:
        return True
    if not candidate.startswith("*."):
        return False
    suffix = candidate[1:]
    if not host.endswith(suffix):
        return False
    label = host[: -len(suffix)]
    return bool(label) and "." not in label


def certificate_covers(cert: x509.Certificate, domains: tuple[str, ...]) -> bool:
    names = certificate_names(cert)
    return all(any(name_covers(pattern, domain) for pattern in names) for domain in domains)


def certificate_expires_within(cert_path: Path, days: int) -> bool:
    cert = load_leaf_certificate(cert_path.read_bytes())
    remaining = cert.not_valid_after_utc - datetime.now(UTC)
    return remaining <= timedelta(days=days)


def _subject_alt_name(domain: str) -> x509.GeneralName:
    normalized = normalize_host(domain)
    try:
        return x509.IPAddress(ipaddress.ip_address(normalized))
    except ValueError:
        return x509.DNSName(normalized)


def build_self_signed(domains: tuple[str, ...], days: int = 90) -> tuple[bytes, bytes]:
    if not domains:
        raise CertificateError("a self-signed certificate needs at least one domain")
    key = ec.generate_private_key(ec.SECP256R1())
    names = [_subject_alt_name(domain) for domain in domains]
    common_name = normalize_host(domains[0])[:64]
    now = datetime.now(UTC)
    cert = (
        x509.CertificateBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)]))
        .issuer_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)]))
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=5))
        .not_valid_after(now + timedelta(days=days))
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .add_extension(x509.SubjectAlternativeName(names), critical=False)
        .add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False)
        .sign(key, hashes.SHA256())
    )
    cert_pem = cert.public_bytes(serialization.Encoding.PEM)
    key_pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    )
    return cert_pem, key_pem


def _write_private(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    path.chmod(0o600)


def _public_bytes(key: object) -> bytes:
    public_key = key.public_key()  # type: ignore[attr-defined]
    return public_key.public_bytes(
        serialization.Encoding.DER,
        serialization.PublicFormat.SubjectPublicKeyInfo,
    )


class FileCertificateProvider:
    def __init__(
        self,
        cert_file: Path,
        key_file: Path,
        domains: tuple[str, ...],
        renew_within_days: int,
        password: str | None,
        required: bool,
    ) -> None:
        self._cert_file = cert_file
        self._key_file = key_file
        self._domains = domains
        self._renew_within_days = renew_within_days
        self._password = password.encode() if password else None
        self._required = required

    async def issue(self) -> TlsMaterial | None:
        material = self._load()
        if material is None and self._required:
            raise CertificateError("certificate file is missing, expired, or does not cover the configured domains")
        return material

    def _load(self) -> TlsMaterial | None:
        if not self._cert_file.is_file() or not self._key_file.is_file():
            return None
        try:
            cert = load_leaf_certificate(self._cert_file.read_bytes())
            key = serialization.load_pem_private_key(self._key_file.read_bytes(), password=self._password)
        except (ValueError, CertificateError):
            return None
        now = datetime.now(UTC)
        if cert.not_valid_before_utc - timedelta(minutes=5) > now:
            return None
        if certificate_expires_within(self._cert_file, self._renew_within_days):
            return None
        if not certificate_covers(cert, self._domains):
            return None
        if _public_bytes(cert) != _public_bytes(key):
            return None
        return TlsMaterial(self._cert_file, self._key_file, "file")


class SelfSignedCertificateProvider:
    def __init__(self, directory: Path, domains: tuple[str, ...], days: int = 90) -> None:
        self._directory = directory
        self._domains = domains
        self._days = days

    async def issue(self) -> TlsMaterial | None:
        cert_pem, key_pem = build_self_signed(self._domains, self._days)
        cert_path = self._directory / "selfsigned.pem"
        key_path = self._directory / "selfsigned.key"
        cert_path.parent.mkdir(parents=True, exist_ok=True)
        cert_path.write_bytes(cert_pem)
        _write_private(key_path, key_pem)
        return TlsMaterial(cert_path, key_path, "selfsigned")


class AcmeCertificateProvider:
    def __init__(
        self,
        client: AcmeClient,
        board: AcmeHttp01Board,
        directory: Path,
        domains: tuple[str, ...],
        email: str,
        required: bool,
    ) -> None:
        self._client = client
        self._board = board
        self._directory = directory
        self._domains = domains
        self._email = email
        self._required = required

    async def issue(self) -> TlsMaterial | None:
        if not self._eligible():
            if self._required:
                raise CertificateError("ACME needs a public domain and a valid email if specified")
            return None
        cert_path = self._directory / "acme-fullchain.pem"
        key_path = self._directory / "acme-privkey.pem"
        cached = FileCertificateProvider(
            cert_path,
            key_path,
            self._domains,
            30,
            None,
            required=False,
        )._load()
        if cached is not None:
            return TlsMaterial(cert_path, key_path, "acme")
        try:
            cert_pem, key_pem = await self._client.issue(self._domains, self._email, self._board)
        finally:
            self._board.clear()
        cert_path.parent.mkdir(parents=True, exist_ok=True)
        cert_path.write_bytes(cert_pem)
        _write_private(key_path, key_pem)
        return TlsMaterial(cert_path, key_path, "acme")

    def _eligible(self) -> bool:
        if self._email and ("@" not in self._email or self._email.startswith("@") or self._email.endswith("@")):
            return False
        return all(is_public_hostname(domain) for domain in self._domains)


class CertificateManager:
    def __init__(self, providers: list[CertificateProvider]) -> None:
        self._providers = providers
        self.current: TlsMaterial | None = None

    async def obtain(self) -> TlsMaterial:
        errors: list[str] = []
        for provider in self._providers:
            try:
                material = await provider.issue()
            except CertificateError as exc:
                errors.append(str(exc))
                continue
            if material is not None:
                self.current = material
                return material
        detail = "; ".join(errors) or "no certificate provider could issue a certificate"
        raise CertificateError(detail)

    async def renew_if_needed(self, within_days: int) -> TlsMaterial | None:
        current = self.current
        if current is None or not certificate_expires_within(current.cert_path, within_days):
            return None
        fresh = await self.obtain()
        if fresh.cert_path == current.cert_path and fresh.cert_path.read_bytes() == current.cert_path.read_bytes():
            return None
        return fresh


def build_providers(settings: Settings, board: AcmeHttp01Board, client: AcmeClient) -> list[CertificateProvider]:
    file_provider = FileCertificateProvider(
        settings.cert_file,
        settings.key_file,
        settings.domains,
        settings.renew_within_days,
        settings.key_password,
        required=settings.tls_mode == "file",
    )
    acme_provider = AcmeCertificateProvider(
        client,
        board,
        settings.cert_dir,
        settings.domains,
        settings.acme_email,
        required=settings.tls_mode == "acme",
    )
    self_signed = SelfSignedCertificateProvider(settings.cert_dir, settings.domains)
    if settings.tls_mode == "file":
        return [file_provider]
    if settings.tls_mode == "acme":
        return [acme_provider]
    if settings.tls_mode == "selfsigned":
        return [self_signed]
    return [file_provider, acme_provider, self_signed]


def hsts_enabled(settings: Settings, source: str) -> bool:
    if settings.hsts is not None:
        return settings.hsts
    return source == "acme"
