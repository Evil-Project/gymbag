import logging
import secrets
from dataclasses import dataclass

import httpx

from gymbag.config import Settings
from gymbag.proxy.reverse import ReverseProxy, WebSocketProxy
from gymbag.tls.acme import AcmeClient, AcmeHttp01Board
from gymbag.tls.certificates import CertificateManager, build_providers
from gymbag.waf.challenge import ChallengeService
from gymbag.waf.page import PageRenderer
from gymbag.waf.policy import WafEngine, build_engine
from gymbag.waf.store import StateStore, open_store
from gymbag.waf.tokens import ClearanceTokens

logger = logging.getLogger(__name__)


@dataclass
class AppContext:
    settings: Settings
    store: StateStore
    clearance: ClearanceTokens
    waf: WafEngine
    challenges: ChallengeService
    pages: PageRenderer
    proxy: ReverseProxy
    sockets: WebSocketProxy
    certificates: CertificateManager
    acme_board: AcmeHttp01Board
    acme: AcmeClient
    client: httpx.AsyncClient | None = None
    hsts: bool = False


def resolve_secret(settings: Settings) -> bytes:
    if settings.secret:
        return settings.secret.encode()
    path = settings.cert_dir / "secret"
    if path.is_file():
        return path.read_text(encoding="utf-8").strip().encode()
    path.parent.mkdir(parents=True, exist_ok=True)
    secret = secrets.token_urlsafe(32)
    path.write_text(secret + "\n", encoding="utf-8")
    path.chmod(0o600)
    logger.info("generated a new clearance secret")
    return secret.encode()


async def build_context(
    settings: Settings,
    *,
    client: httpx.AsyncClient | None = None,
    store: StateStore | None = None,
    acme: AcmeClient | None = None,
) -> AppContext:
    if store is None:
        if settings.redis_url:
            logger.info("connecting to Redis for WAF state")
        store = await open_store(settings.redis_url)
    clearance = ClearanceTokens(resolve_secret(settings), settings.clearance_ttl)
    waf = build_engine(store, clearance, settings.cookie_name, settings.rate_limit, settings.rate_window)
    board = AcmeHttp01Board()
    acme_client = acme or AcmeClient(settings.acme_directory, settings.cert_dir / "acme-account.pem")
    return AppContext(
        settings=settings,
        store=store,
        clearance=clearance,
        waf=waf,
        challenges=ChallengeService(store, settings.difficulty, settings.challenge_ttl),
        pages=PageRenderer(),
        proxy=ReverseProxy(settings, client),
        sockets=WebSocketProxy(settings, waf),
        certificates=CertificateManager(build_providers(settings, board, acme_client)),
        acme_board=board,
        acme=acme_client,
        client=client,
    )
