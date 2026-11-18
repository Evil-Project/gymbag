from typing import Protocol

import httpx
from fastapi.responses import HTMLResponse

from gymbag.config import Settings
from gymbag.proxy.reverse import ReverseProxy, WebSocketProxy
from gymbag.tls.acme import AcmeClient, AcmeHttp01Board
from gymbag.waf.policy import Decision, IncomingRequest
from gymbag.waf.store import StateStore
from gymbag.waf.submission import ChallengeJSON
class AccessGate(Protocol):
    async def decide(self, request: IncomingRequest) -> Decision: ...


class ChallengeIssuer(Protocol):
    async def issue(self, client_ip: str) -> tuple[str, int]: ...

    async def verify(self, client_ip: str, submission: ChallengeJSON) -> bool: ...


class BlockPages(Protocol):
    def reject(self, reason: str, status_code: int) -> HTMLResponse: ...

    def challenge(self, challenge_id: str, difficulty: int) -> HTMLResponse: ...


class ClearanceIssuer(Protocol):
    def issue(self, client_ip: str) -> str: ...


class ProxyRuntime(Protocol):
    """What the HTTP gateway needs from the composition root."""

    settings: Settings
    store: StateStore
    clearance: ClearanceIssuer
    waf: AccessGate
    challenges: ChallengeIssuer
    pages: BlockPages
    proxy: ReverseProxy
    sockets: WebSocketProxy
    acme_board: AcmeHttp01Board
    acme: AcmeClient
    client: httpx.AsyncClient | None
    hsts: bool
