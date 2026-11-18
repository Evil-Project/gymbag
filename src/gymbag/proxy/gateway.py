from collections.abc import Mapping

import httpx

from gymbag.hosts import hostname_from_header

_HOP_BY_HOP = frozenset(
    {
        "connection",
        "keep-alive",
        "proxy-authenticate",
        "proxy-authorization",
        "te",
        "trailer",
        "trailers",
        "transfer-encoding",
        "upgrade",
    }
)
_REQUEST_DROP = _HOP_BY_HOP | {"host", "content-length"}
_RESPONSE_DROP = _HOP_BY_HOP | {"alt-svc", "strict-transport-security"}
_WEBSOCKET_DROP = _REQUEST_DROP | {
    "sec-websocket-key",
    "sec-websocket-version",
    "sec-websocket-extensions",
    "sec-websocket-protocol",
}


class HeaderPairs:
    """Preserves repeated response headers such as multiple Set-Cookie values."""

    def __init__(self, pairs: list[tuple[str, str]]) -> None:
        self._pairs = pairs

    def items(self) -> list[tuple[str, str]]:
        return self._pairs


def _connection_options(headers: Mapping[str, str]) -> set[str]:
    raw = headers.get("connection", "")
    return {part.strip().lower() for part in raw.split(",") if part.strip()}


def forward_request_headers(headers: Mapping[str, str], client_ip: str, scheme: str) -> dict[str, str]:
    blocked = _REQUEST_DROP | _connection_options(headers)
    forwarded = {key: value for key, value in headers.items() if key.lower() not in blocked}
    host = headers.get("host")
    if host:
        forwarded["x-forwarded-host"] = host
    forwarded["x-forwarded-for"] = client_ip
    forwarded["x-real-ip"] = client_ip
    forwarded["x-forwarded-proto"] = scheme
    return forwarded


def websocket_headers(headers: Mapping[str, str]) -> list[tuple[str, str]]:
    blocked = _WEBSOCKET_DROP | _connection_options(headers)
    return [(key, value) for key, value in headers.items() if key.lower() not in blocked]


def websocket_subprotocols(headers: Mapping[str, str]) -> list[str] | None:
    raw = headers.get("sec-websocket-protocol")
    if not raw:
        return None
    values = [part.strip() for part in raw.split(",") if part.strip()]
    return values or None


def public_origin(host_header: str, https_port: int) -> str:
    host = hostname_from_header(host_header)
    if ":" in host:
        host = f"[{host}]"
    if https_port == 443:
        return f"https://{host}"
    return f"https://{host}:{https_port}"


def _query_text(url: httpx.URL) -> str:
    query = url.query
    if isinstance(query, bytes):
        return query.decode("latin-1")
    return str(query)


def rewrite_location(location: str, upstream: str, origin: str) -> str:
    try:
        target = httpx.URL(location)
        base = httpx.URL(upstream)
    except (httpx.InvalidURL, ValueError):
        return location
    if not target.scheme or not target.host or target.host != base.host:
        return location
    target_port = target.port or (443 if target.scheme == "https" else 80)
    base_port = base.port or (443 if base.scheme == "https" else 80)
    if target_port != base_port:
        return location
    path = target.path or "/"
    query = _query_text(target)
    suffix = f"?{query}" if query else ""
    return f"{origin}{path}{suffix}"


def forward_response_headers(
    headers: httpx.Headers,
    upstream: str,
    host_header: str,
    https_port: int,
) -> HeaderPairs:
    blocked = _RESPONSE_DROP | _connection_options(headers)
    origin = public_origin(host_header, https_port) if host_header else ""
    pairs: list[tuple[str, str]] = []
    for key, value in headers.multi_items():
        if key.lower() in blocked:
            continue
        if key.lower() == "location" and origin:
            value = rewrite_location(value, upstream, origin)
        pairs.append((key, value))
    return HeaderPairs(pairs)


def upstream_url(upstream: str, path: str, query: bytes) -> httpx.URL:
    base = httpx.URL(upstream)
    prefix = base.path.rstrip("/")
    suffix = path if path.startswith("/") else f"/{path}"
    joined = f"{prefix}{suffix}" or "/"
    return base.copy_with(path=joined, query=query or None)


def websocket_upstream(upstream: str, path: str, query: str) -> str:
    base = httpx.URL(upstream)
    scheme = "wss" if base.scheme == "https" else "ws"
    prefix = base.path.rstrip("/")
    suffix = path if path.startswith("/") else f"/{path}"
    joined = f"{prefix}{suffix}" or "/"
    host = base.host or ""
    if ":" in host:
        host = f"[{host}]"
    port = base.port
    default_port = 443 if scheme == "wss" else 80
    port_text = f":{port}" if port and port != default_port else ""
    query_text = f"?{query}" if query else ""
    return f"{scheme}://{host}{port_text}{joined}{query_text}"


class UpstreamRouter:
    def __init__(self, default: str, routes: tuple[tuple[str, str], ...]) -> None:
        self._default = default
        self._routes = {host: target for host, target in routes}

    def resolve(self, host_header: str) -> str:
        if not host_header:
            return self._default
        return self._routes.get(hostname_from_header(host_header), self._default)
