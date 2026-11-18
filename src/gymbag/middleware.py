from starlette._utils import parse_host_header
from starlette.datastructures import Headers
from starlette.responses import PlainTextResponse
from starlette.types import ASGIApp, Receive, Scope, Send

from gymbag.hosts import hostname_from_header
from gymbag.runtime import AccessGate, BlockPages, ChallengeIssuer
from gymbag.waf.policy import Action, incoming_from_scope


class AllowedHostMiddleware:
    def __init__(self, app: ASGIApp, allowed_hosts: tuple[str, ...]) -> None:
        self.app = app
        self._allowed = {host.lower() for host in allowed_hosts}

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] not in {"http", "websocket"}:
            await self.app(scope, receive, send)
            return
        parsed = parse_host_header(Headers(scope=scope).get("host"))
        host = hostname_from_header(parsed.host) if parsed is not None else ""
        if host not in self._allowed:
            await PlainTextResponse("Invalid host header", status_code=400)(scope, receive, send)
            return
        await self.app(scope, receive, send)


class HstsMiddleware:
    def __init__(self, app: ASGIApp) -> None:
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        async def send_with_hsts(message):
            if message["type"] == "http.response.start":
                headers = list(message.get("headers") or [])
                headers.append((b"strict-transport-security", b"max-age=31536000"))
                message = {**message, "headers": headers}
            await send(message)

        await self.app(scope, receive, send_with_hsts)


class WafMiddleware:
    def __init__(
        self,
        app: ASGIApp,
        gate: AccessGate,
        pages: BlockPages,
        challenges: ChallengeIssuer,
    ) -> None:
        self.app = app
        self._gate = gate
        self._pages = pages
        self._challenges = challenges

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        incoming = incoming_from_scope(scope)
        decision = await self._gate.decide(incoming)
        if decision.action is Action.DENY:
            response = self._pages.reject(decision.reason, decision.status_code)
            await response(scope, receive, send)
            return
        if decision.action is Action.CHALLENGE:
            challenge_id, difficulty = await self._challenges.issue(incoming.client_ip)
            response = self._pages.challenge(challenge_id, difficulty)
            await response(scope, receive, send)
            return
        await self.app(scope, receive, send)
