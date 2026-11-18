from __future__ import annotations

import os
import shlex
from dataclasses import dataclass, replace
from pathlib import Path

import httpx

from gymbag.errors import GymbagError
from gymbag.hosts import hostname_from_header, is_public_hostname, normalize_host

_TLS_MODES = frozenset({"auto", "acme", "selfsigned", "file"})
DEFAULT_CONFIG_FILES = ("Gymbagfile", "Caddyfile")


def _parse_int(raw: str, name: str) -> int:
    try:
        value = int(raw)
    except ValueError as exc:
        raise GymbagError(f"{name} must be an integer") from exc
    if value <= 0:
        raise GymbagError(f"{name} must be greater than zero")
    return value


def _parse_port(raw: str, name: str) -> int:
    value = _parse_int(raw, name)
    if value > 65535:
        raise GymbagError(f"{name} must be between 1 and 65535")
    return value


def _parse_bool(raw: str, name: str) -> bool:
    lowered = raw.strip().lower()
    if lowered in {"1", "true", "yes", "on"}:
        return True
    if lowered in {"0", "false", "no", "off"}:
        return False
    raise GymbagError(f"{name} must be true or false")


def _env_int(name: str, default: int) -> int:
    raw = os.environ.get(name)
    if raw is None or raw == "":
        return default
    return _parse_int(raw, name)


def _env_port(name: str, default: int) -> int:
    raw = os.environ.get(name)
    if raw is None or raw == "":
        return default
    return _parse_port(raw, name)


def _env_bool(name: str, default: bool) -> bool:
    raw = os.environ.get(name)
    if raw is None or raw == "":
        return default
    return _parse_bool(raw, name)


def _env_optional_bool(name: str, default: bool | None = None) -> bool | None:
    raw = os.environ.get(name)
    if raw is None or raw == "":
        return default
    return _parse_bool(raw, name)


def _domains_from_env(default: tuple[str, ...] = ("localhost",)) -> tuple[str, ...]:
    raw_list = os.environ.get("GYMBAG_DOMAINS", "")
    raw_single = os.environ.get("GYMBAG_DOMAIN", "")
    if raw_list.strip():
        domains = tuple(normalize_host(part) for part in raw_list.split(",") if part.strip())
    elif raw_single.strip():
        domains = (normalize_host(raw_single),)
    else:
        domains = default
    if not domains or any(not domain for domain in domains):
        raise GymbagError("at least one domain is required")
    return domains


def _upstreams_from_env(default: tuple[tuple[str, str], ...] = ()) -> tuple[tuple[str, str], ...]:
    raw = os.environ.get("GYMBAG_UPSTREAMS", "")
    if not raw.strip():
        return default
    routes: list[tuple[str, str]] = []
    for item in raw.split(";"):
        entry = item.strip()
        if not entry:
            continue
        host, separator, target = entry.partition("=")
        if not separator or not host.strip() or not target.strip():
            raise GymbagError(f"invalid GYMBAG_UPSTREAMS entry: {entry}")
        normalized_target = normalize_upstream(target.strip(), "GYMBAG_UPSTREAMS")
        routes.append((hostname_from_header(host), normalized_target))
    return tuple(routes)


def _require_http_url(value: str, name: str) -> None:
    try:
        url = httpx.URL(value)
    except httpx.InvalidURL as exc:
        raise GymbagError(f"{name} must be an absolute http or https URL") from exc
    if url.scheme not in {"http", "https"} or not url.host:
        raise GymbagError(f"{name} must be an absolute http or https URL")


def normalize_upstream(value: str, name: str = "upstream") -> str:
    target = value.strip()
    if not target:
        raise GymbagError(f"{name} must not be empty")
    if target.startswith(":"):
        target = f"http://localhost{target}"
    elif "://" not in target:
        target = f"http://{target}"
    _require_http_url(target, name)
    return target


def _default_bind(domains: tuple[str, ...]) -> str:
    if any(is_public_hostname(domain) for domain in domains):
        return "0.0.0.0"
    return "127.0.0.1"


def _strip_comment(line: str) -> str:
    in_quote: str | None = None
    escaped = False
    for index, char in enumerate(line):
        if escaped:
            escaped = False
            continue
        if char == "\\":
            escaped = True
            continue
        if in_quote is not None:
            if char == in_quote:
                in_quote = None
            continue
        if char in {'"', "'"}:
            in_quote = char
            continue
        if char == "#" and (index == 0 or line[index - 1].isspace()):
            return line[:index]
    return line


def _parse_site_address(token: str) -> tuple[str, int | None]:
    raw = token.strip().rstrip(",")
    if not raw:
        raise GymbagError("site address must not be empty")
    if "://" in raw:
        try:
            url = httpx.URL(raw)
        except httpx.InvalidURL as exc:
            raise GymbagError(f"invalid site address: {token}") from exc
        if url.scheme not in {"http", "https"} or not url.host:
            raise GymbagError(f"invalid site address: {token}")
        return normalize_host(url.host), url.port
    host = hostname_from_header(raw)
    port: int | None = None
    if raw.startswith("[") and "]:" in raw:
        _, _, port_str = raw.rpartition("]:")
        port = _parse_port(port_str, "site port")
    elif raw.count(":") == 1:
        _, _, port_str = raw.partition(":")
        port = _parse_port(port_str, "site port")
    if not host:
        raise GymbagError(f"invalid site address: {token}")
    return host, port


def _tokenize_caddyfile(text: str) -> list[str]:
    tokens: list[str] = []
    for raw_line in text.splitlines():
        cleaned = _strip_comment(raw_line).strip()
        if not cleaned:
            continue
        spaced = cleaned.replace("{", " { ").replace("}", " } ").replace(";", " ; ")
        try:
            line_tokens = shlex.split(spaced)
        except ValueError as exc:
            raise GymbagError(f"invalid config syntax: {exc}") from exc
        if line_tokens:
            tokens.extend(line_tokens)
            tokens.append("\n")
    return tokens


def _split_statements(tokens: list[str]) -> list[list[str]]:
    statements: list[list[str]] = []
    current: list[str] = []
    for token in tokens:
        if token in {"\n", ";"}:
            if current:
                statements.append(current)
                current = []
        else:
            current.append(token)
    if current:
        statements.append(current)
    return statements


def _apply_directive(stmt: list[str], options: dict[str, object]) -> str | None:
    if not stmt:
        return None
    if len(stmt) == 3 and stmt[1] == "->":
        options.setdefault("_flat_domains", []).append(stmt[0])  # type: ignore[union-attr]
        return normalize_upstream(stmt[2], "reverse_proxy")

    name = stmt[0].lower()
    args = stmt[1:]

    if name in {"reverse_proxy", "upstream", "proxy"}:
        target_args = [arg for arg in args if arg.lower() != "to"]
        if not target_args:
            raise GymbagError(f"{name} requires an upstream URL")
        return normalize_upstream(target_args[0], name)

    if len(stmt) == 1 and ("://" in stmt[0] or stmt[0].startswith(":")):
        return normalize_upstream(stmt[0], "reverse_proxy")

    if name in {"domain", "domains", "host", "hosts"}:
        if not args:
            raise GymbagError(f"{name} requires at least one domain")
        flat: list[str] = []
        for arg in args:
            for part in arg.split(","):
                if part.strip():
                    host, _ = _parse_site_address(part)
                    flat.append(host)
        if not flat:
            raise GymbagError(f"{name} requires at least one domain")
        options.setdefault("_flat_domains", []).extend(flat)  # type: ignore[union-attr]
        return None

    if name in {"email", "acme_email"}:
        if len(args) != 1:
            raise GymbagError(f"{name} requires an email address")
        options["acme_email"] = args[0].strip()
        return None

    if name == "acme_directory":
        if len(args) != 1:
            raise GymbagError("acme_directory requires a URL")
        _require_http_url(args[0], "acme_directory")
        options["acme_directory"] = args[0]
        return None

    if name == "tls":
        if not args:
            raise GymbagError("tls requires arguments")
        if len(args) == 1:
            val = args[0].strip()
            lowered = val.lower()
            if lowered in {"internal", "selfsigned", "self_signed"}:
                options["tls_mode"] = "selfsigned"
            elif lowered in _TLS_MODES:
                options["tls_mode"] = lowered
            elif "@" in val:
                options["acme_email"] = val
            else:
                raise GymbagError(f"unsupported tls option: {val}")
        elif len(args) == 2:
            options["tls_mode"] = "file"
            options["cert_file"] = Path(args[0])
            options["key_file"] = Path(args[1])
        else:
            raise GymbagError("tls accepts a mode, an email, or cert and key file paths")
        return None

    if name in {"bind", "bind_host"}:
        if len(args) != 1:
            raise GymbagError("bind requires a host address")
        options["bind_host"] = args[0].strip()
        return None

    if name == "http_port":
        if len(args) != 1:
            raise GymbagError("http_port requires a port number")
        options["http_port"] = _parse_port(args[0], "http_port")
        return None

    if name == "https_port":
        if len(args) != 1:
            raise GymbagError("https_port requires a port number")
        options["https_port"] = _parse_port(args[0], "https_port")
        return None

    if name == "difficulty":
        if len(args) != 1:
            raise GymbagError("difficulty requires an integer from 1 to 5")
        diff = _parse_int(args[0], "difficulty")
        if not 1 <= diff <= 5:
            raise GymbagError("difficulty must be from 1 to 5")
        options["difficulty"] = diff
        return None

    if name == "rate_limit":
        if len(args) not in {1, 2}:
            raise GymbagError("rate_limit requires a limit and optional window in seconds")
        options["rate_limit"] = _parse_int(args[0], "rate_limit")
        if len(args) == 2:
            options["rate_window"] = _parse_int(args[1], "rate_window")
        return None

    if name == "rate_window":
        if len(args) != 1:
            raise GymbagError("rate_window requires seconds")
        options["rate_window"] = _parse_int(args[0], "rate_window")
        return None

    if name == "challenge_ttl":
        if len(args) != 1:
            raise GymbagError("challenge_ttl requires seconds")
        options["challenge_ttl"] = _parse_int(args[0], "challenge_ttl")
        return None

    if name == "clearance_ttl":
        if len(args) != 1:
            raise GymbagError("clearance_ttl requires seconds")
        options["clearance_ttl"] = _parse_int(args[0], "clearance_ttl")
        return None

    if name == "cookie_name":
        if len(args) != 1 or not args[0].strip():
            raise GymbagError("cookie_name requires a value")
        options["cookie_name"] = args[0].strip()
        return None

    if name == "cookie_secure":
        if len(args) != 1:
            raise GymbagError("cookie_secure requires true or false")
        options["cookie_secure"] = _parse_bool(args[0], "cookie_secure")
        return None

    if name in {"redis", "redis_url"}:
        if len(args) != 1:
            raise GymbagError("redis requires a Redis URL")
        options["redis_url"] = args[0].strip()
        return None

    if name == "secret":
        if len(args) != 1:
            raise GymbagError("secret requires a value")
        options["secret"] = args[0]
        return None

    if name == "cert_dir":
        if len(args) != 1:
            raise GymbagError("cert_dir requires a directory path")
        options["cert_dir"] = Path(args[0])
        return None

    if name == "hsts":
        if len(args) != 1:
            raise GymbagError("hsts requires true or false")
        options["hsts"] = _parse_bool(args[0], "hsts")
        return None

    raise GymbagError(f"unknown directive: {stmt[0]}")


@dataclass(frozen=True)
class Settings:
    upstream: str = "http://localhost:5173"
    upstreams: tuple[tuple[str, str], ...] = ()
    domains: tuple[str, ...] = ("localhost",)
    bind_host: str = "127.0.0.1"
    http_port: int = 80
    https_port: int = 443
    cert_file: Path = Path("cert.pem")
    key_file: Path = Path("key.pem")
    key_password: str | None = None
    cert_dir: Path = Path("certs")
    tls_mode: str = "auto"
    acme_email: str = ""
    acme_directory: str = "https://acme-v02.api.letsencrypt.org/directory"
    redis_url: str = ""
    secret: str = ""
    difficulty: int = 4
    challenge_ttl: int = 300
    clearance_ttl: int = 86_400
    cookie_name: str = "waf_clearance"
    cookie_secure: bool = True
    rate_limit: int = 240
    rate_window: int = 60
    renew_interval: int = 12 * 60 * 60
    renew_within_days: int = 30
    hsts: bool | None = None

    @classmethod
    def from_caddyfile(cls, text: str) -> Settings:
        tokens = _tokenize_caddyfile(text)
        options: dict[str, object] = {}
        routes: list[tuple[str, str]] = []
        domains_order: list[str] = []

        if "{" in tokens or "}" in tokens:
            index = 0
            length = len(tokens)
            while index < length:
                while index < length and tokens[index] in {"\n", ";"}:
                    index += 1
                if index >= length:
                    break
                header: list[str] = []
                while index < length and tokens[index] not in {"{", "}"}:
                    if tokens[index] not in {"\n", ";"}:
                        header.append(tokens[index])
                    index += 1
                if index >= length or tokens[index] != "{":
                    raise GymbagError("unexpected token or missing '{' in config file")
                index += 1  # consume '{'
                depth = 1
                body_tokens: list[str] = []
                while index < length and depth > 0:
                    tok = tokens[index]
                    if tok == "{":
                        depth += 1
                    elif tok == "}":
                        depth -= 1
                        if depth == 0:
                            index += 1
                            break
                    if depth == 1:
                        body_tokens.append(tok)
                    index += 1
                if depth != 0:
                    raise GymbagError("unclosed '{' in config file")

                statements = _split_statements(body_tokens)
                if not header:
                    for stmt in statements:
                        upstream = _apply_directive(stmt, options)
                        if upstream is not None:
                            options["upstream"] = upstream
                else:
                    site_hosts: list[str] = []
                    for raw_addr in header:
                        for part in raw_addr.split(","):
                            if not part.strip():
                                continue
                            host, port = _parse_site_address(part)
                            site_hosts.append(host)
                            if port is not None and "https_port" not in options:
                                options["https_port"] = port
                    if not site_hosts:
                        raise GymbagError("site block requires at least one domain")
                    site_upstream: str | None = None
                    for stmt in statements:
                        target = _apply_directive(stmt, options)
                        if target is not None:
                            site_upstream = target
                    if site_upstream is None:
                        raise GymbagError(f"site block for {', '.join(site_hosts)} requires reverse_proxy <upstream>")
                    for host in site_hosts:
                        if host not in domains_order:
                            domains_order.append(host)
                        routes.append((host, site_upstream))
        else:
            statements = _split_statements(tokens)
            flat_upstream: str | None = None
            for idx, stmt in enumerate(statements):
                if idx == 0 and len(stmt) >= 1 and stmt[0].lower() not in {
                    "reverse_proxy",
                    "upstream",
                    "proxy",
                    "domain",
                    "domains",
                    "host",
                    "hosts",
                    "email",
                    "acme_email",
                    "acme_directory",
                    "tls",
                    "bind",
                    "bind_host",
                    "http_port",
                    "https_port",
                    "difficulty",
                    "rate_limit",
                    "rate_window",
                    "challenge_ttl",
                    "clearance_ttl",
                    "cookie_name",
                    "cookie_secure",
                    "redis",
                    "redis_url",
                    "secret",
                    "cert_dir",
                    "hsts",
                } and (len(stmt) != 3 or stmt[1] != "->"):
                    for token in stmt:
                        for part in token.split(","):
                            if part.strip():
                                host, port = _parse_site_address(part)
                                options.setdefault("_flat_domains", []).append(host)  # type: ignore[union-attr]
                                if port is not None and "https_port" not in options:
                                    options["https_port"] = port
                    continue
                target = _apply_directive(stmt, options)
                if target is not None:
                    flat_upstream = target

            flat_domains: list[str] = options.pop("_flat_domains", [])  # type: ignore[assignment]
            if not flat_domains or flat_upstream is None:
                raise GymbagError("config file must specify both a domain and an upstream URL")
            for host in flat_domains:
                if host not in domains_order:
                    domains_order.append(host)
                routes.append((host, flat_upstream))

        options.pop("_flat_domains", None)
        if not domains_order or not routes:
            raise GymbagError("config file must define at least one site with a domain and upstream URL")

        domains = tuple(domains_order)
        default_upstream = routes[0][1]
        bind_host = str(options.pop("bind_host", _default_bind(domains)))
        return cls(
            upstream=default_upstream,
            upstreams=tuple(routes),
            domains=domains,
            bind_host=bind_host,
            **options,  # type: ignore[arg-type]
        )

    @classmethod
    def from_file(cls, path: Path | str) -> Settings:
        file_path = Path(path)
        if not file_path.is_file():
            raise GymbagError(f"config file not found: {file_path}")
        return cls.from_caddyfile(file_path.read_text(encoding="utf-8"))

    @classmethod
    def from_env(cls, base: Settings | None = None) -> Settings:
        has_base = base is not None
        if base is None:
            config_env = os.environ.get("GYMBAG_CONFIG", "").strip()
            if config_env:
                base = cls.from_file(Path(config_env))
                has_base = True
            else:
                base = cls()

        domains = _domains_from_env(base.domains)
        raw_upstream = os.environ.get("GYMBAG_UPSTREAM", "").strip()
        if raw_upstream:
            upstream = normalize_upstream(raw_upstream, "GYMBAG_UPSTREAM")
        else:
            upstream = base.upstream
            _require_http_url(upstream, "GYMBAG_UPSTREAM")

        raw_mode = os.environ.get("GYMBAG_TLS_MODE", "").strip()
        mode = raw_mode.lower() if raw_mode else base.tls_mode
        if mode not in _TLS_MODES:
            raise GymbagError("GYMBAG_TLS_MODE must be auto, acme, selfsigned, or file")

        difficulty = _env_int("GYMBAG_DIFFICULTY", base.difficulty)
        if not 1 <= difficulty <= 5:
            raise GymbagError("GYMBAG_DIFFICULTY must be from 1 to 5")

        raw_bind = os.environ.get("GYMBAG_BIND", "").strip()
        if raw_bind:
            bind_host = raw_bind
        elif has_base and domains == base.domains:
            bind_host = base.bind_host
        else:
            bind_host = _default_bind(domains)

        return cls(
            upstream=upstream,
            upstreams=_upstreams_from_env(base.upstreams),
            domains=domains,
            bind_host=bind_host,
            http_port=_env_port("GYMBAG_HTTP_PORT", base.http_port),
            https_port=_env_port("GYMBAG_HTTPS_PORT", base.https_port),
            cert_file=Path(os.environ.get("GYMBAG_CERT_FILE") or base.cert_file),
            key_file=Path(os.environ.get("GYMBAG_KEY_FILE") or base.key_file),
            key_password=os.environ.get("GYMBAG_KEY_PASSWORD") or base.key_password,
            cert_dir=Path(os.environ.get("GYMBAG_CERT_DIR") or base.cert_dir),
            tls_mode=mode,
            acme_email=os.environ.get("GYMBAG_ACME_EMAIL", base.acme_email).strip(),
            acme_directory=os.environ.get("GYMBAG_ACME_DIRECTORY") or base.acme_directory,
            redis_url=os.environ.get("GYMBAG_REDIS_URL", base.redis_url).strip(),
            secret=os.environ.get("GYMBAG_SECRET") or base.secret,
            difficulty=difficulty,
            challenge_ttl=_env_int("GYMBAG_CHALLENGE_TTL", base.challenge_ttl),
            clearance_ttl=_env_int("GYMBAG_CLEARANCE_TTL", base.clearance_ttl),
            cookie_name=os.environ.get("GYMBAG_COOKIE_NAME") or base.cookie_name,
            cookie_secure=_env_bool("GYMBAG_COOKIE_SECURE", base.cookie_secure),
            rate_limit=_env_int("GYMBAG_RATE_LIMIT", base.rate_limit),
            rate_window=_env_int("GYMBAG_RATE_WINDOW", base.rate_window),
            renew_interval=_env_int("GYMBAG_RENEW_INTERVAL", base.renew_interval),
            renew_within_days=_env_int("GYMBAG_RENEW_WITHIN_DAYS", base.renew_within_days),
            hsts=_env_optional_bool("GYMBAG_HSTS", base.hsts),
        )

    @classmethod
    def load(
        cls,
        config_path: Path | str | None = None,
        *,
        domain: str | None = None,
        upstream: str | None = None,
    ) -> Settings:
        base: Settings | None = None
        resolved_path: Path | None = None
        if config_path is not None:
            resolved_path = Path(config_path)
        else:
            env_config = os.environ.get("GYMBAG_CONFIG", "").strip()
            if env_config:
                resolved_path = Path(env_config)
            else:
                for candidate_name in DEFAULT_CONFIG_FILES:
                    candidate = Path(candidate_name)
                    if candidate.is_file():
                        resolved_path = candidate
                        break

        if resolved_path is not None:
            base = cls.from_file(resolved_path)

        settings = cls.from_env(base=base)
        if domain or upstream:
            new_domains = settings.domains
            if domain:
                parsed = tuple(normalize_host(part) for part in domain.split(",") if part.strip())
                if not parsed or any(not d for d in parsed):
                    raise GymbagError("at least one domain is required")
                new_domains = parsed
            new_upstream = normalize_upstream(upstream, "upstream") if upstream else settings.upstream
            new_bind = (
                os.environ.get("GYMBAG_BIND", "").strip()
                or _default_bind(new_domains)
            )
            settings = replace(
                settings,
                domains=new_domains,
                upstream=new_upstream,
                upstreams=tuple((d, new_upstream) for d in new_domains),
                bind_host=new_bind,
            )
        return settings
