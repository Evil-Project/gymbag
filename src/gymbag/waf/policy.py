from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Protocol

from gymbag.waf.store import StateStore
from gymbag.waf.tokens import ClearanceTokens


class Action(Enum):
    ALLOW = "allow"
    CHALLENGE = "challenge"
    DENY = "deny"


@dataclass(frozen=True)
class Decision:
    action: Action
    reason: str = ""
    status_code: int = 403

    @staticmethod
    def allow() -> Decision:
        return Decision(Action.ALLOW)

    @staticmethod
    def challenge() -> Decision:
        return Decision(Action.CHALLENGE)

    @staticmethod
    def deny(reason: str, status_code: int = 403) -> Decision:
        return Decision(Action.DENY, reason, status_code)


@dataclass(frozen=True)
class IncomingRequest:
    method: str
    path: str
    query: str
    headers: Mapping[str, str]
    client_ip: str
    cookies: Mapping[str, str]


class WafRule(Protocol):
    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        """Return a decision, or None when the next rule should decide."""


_METHODS = frozenset({"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"})


class RequestShapeRule:
    def __init__(self, max_target_length: int = 8192) -> None:
        self._max_target_length = max_target_length

    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        if request.method.upper() not in _METHODS:
            return Decision.deny("shape")
        target = f"{request.path}?{request.query}" if request.query else request.path
        folded = target.lower()
        if len(target) > self._max_target_length or "\x00" in target:
            return Decision.deny("shape")
        if ".." in request.path.split("/") or "%2e%2e" in folded or "..\\" in target or "\\.." in target:
            return Decision.deny("shape")
        return None


class PrefixAllowRule:
    def __init__(self, prefixes: tuple[str, ...]) -> None:
        self._prefixes = prefixes

    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        if any(request.path.startswith(prefix) for prefix in self._prefixes):
            return Decision.allow()
        return None


class ExactAllowRule:
    def __init__(self, paths: frozenset[str]) -> None:
        self._paths = paths

    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        if request.path in self._paths:
            return Decision.allow()
        return None


class MethodAllowRule:
    def __init__(self, methods: frozenset[str]) -> None:
        self._methods = methods

    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        if request.method.upper() in self._methods:
            return Decision.allow()
        return None


class RateLimitRule:
    def __init__(self, store: StateStore, limit: int, window_seconds: int) -> None:
        self._store = store
        self._limit = limit
        self._window = window_seconds

    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        count = await self._store.increment(f"ip:{request.client_ip}", self._window)
        if count > self._limit:
            return Decision.deny("rate", status_code=429)
        return None


class ClearanceRule:
    def __init__(self, tokens: ClearanceTokens, cookie_name: str) -> None:
        self._tokens = tokens
        self._cookie_name = cookie_name

    async def evaluate(self, request: IncomingRequest) -> Decision | None:
        if self._tokens.valid(request.cookies.get(self._cookie_name), request.client_ip):
            return Decision.allow()
        return Decision.challenge()


class WafEngine:
    def __init__(self, rules: Sequence[WafRule]) -> None:
        self._rules = tuple(rules)

    async def decide(self, request: IncomingRequest) -> Decision:
        for rule in self._rules:
            decision = await rule.evaluate(request)
            if decision is not None:
                return decision
        return Decision.allow()


def build_engine(store: StateStore, tokens: ClearanceTokens, cookie_name: str, rate_limit: int, rate_window: int) -> WafEngine:
    return WafEngine(
        (
            RequestShapeRule(),
            PrefixAllowRule(("/.well-known/acme-challenge/",)),
            RateLimitRule(store, rate_limit, rate_window),
            MethodAllowRule(frozenset({"OPTIONS"})),
            PrefixAllowRule(("/.waf/assets/",)),
            ExactAllowRule(frozenset({"/.waf/verify"})),
            ClearanceRule(tokens, cookie_name),
        )
    )


def incoming_from_scope(scope: Mapping[str, object]) -> IncomingRequest:
    raw_headers = scope.get("headers") or []
    headers: dict[str, str] = {}
    for name, value in raw_headers:  # type: ignore[union-attr]
        headers[name.decode("latin-1")] = value.decode("latin-1")
    cookies: dict[str, str] = {}
    for part in headers.get("cookie", "").split(";"):
        if "=" not in part:
            continue
        name, value = part.split("=", 1)
        cookies[name.strip()] = value.strip()
    client = scope.get("client")
    client_ip = client[0] if isinstance(client, tuple) and client else ""
    query = scope.get("query_string") or b""
    path = scope.get("path") or "/"
    return IncomingRequest(
        method=str(scope.get("method") or "GET"),
        path=str(path),
        query=bytes(query).decode("latin-1") if isinstance(query, (bytes, bytearray)) else str(query),
        headers=headers,
        client_ip=str(client_ip),
        cookies=cookies,
    )
