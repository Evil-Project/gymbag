from contextlib import asynccontextmanager

import httpx
from fastapi import FastAPI, Request, Response, WebSocket
from fastapi.responses import FileResponse, JSONResponse

from gymbag.middleware import AllowedHostMiddleware, HstsMiddleware, WafMiddleware
from gymbag.runtime import ProxyRuntime
from gymbag.tls.acme import AcmeHttp01Board
from gymbag.waf.page import asset_file
from gymbag.waf.policy import incoming_from_scope
from gymbag.waf.submission import ChallengeJSON

_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"]
_INTERNAL_PREFIXES = ("/.waf/", "/.well-known/acme-challenge/")


def acme_challenge_response(board: AcmeHttp01Board, token: str) -> Response:
    authorization = board.lookup(token)
    if authorization is None:
        return Response(status_code=404)
    return Response(content=authorization, media_type="text/plain", headers={"cache-control": "no-store"})


def create_proxy_app(runtime: ProxyRuntime) -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI):
        owns_client = runtime.client is None
        client = runtime.client
        if owns_client:
            client = httpx.AsyncClient(
                timeout=httpx.Timeout(None, connect=10.0),
                follow_redirects=False,
                limits=httpx.Limits(max_connections=100, max_keepalive_connections=20),
            )
        assert client is not None
        runtime.proxy.bind(client)
        try:
            yield
        finally:
            if owns_client:
                await client.aclose()
            await runtime.acme.aclose()
            await runtime.store.aclose()

    app = FastAPI(
        lifespan=lifespan,
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
        redirect_slashes=False,
    )

    @app.post("/.waf/verify")
    async def verify(request: Request, submission: ChallengeJSON) -> JSONResponse:
        incoming = incoming_from_scope(request.scope)
        if not await runtime.challenges.verify(incoming.client_ip, submission):
            return JSONResponse({"ok": False}, status_code=403)
        response = JSONResponse({"ok": True})
        response.set_cookie(
            key=runtime.settings.cookie_name,
            value=runtime.clearance.issue(incoming.client_ip),
            max_age=runtime.settings.clearance_ttl,
            httponly=True,
            secure=runtime.settings.cookie_secure,
            samesite="lax",
            path="/",
        )
        return response

    @app.get("/.waf/assets/{name}")
    async def asset(name: str) -> Response:
        path = asset_file(name)
        if path is None:
            return Response(status_code=404)
        return FileResponse(path, media_type="image/png", headers={"cache-control": "public, max-age=86400"})

    @app.get("/.well-known/acme-challenge/{token}")
    async def acme(token: str) -> Response:
        return acme_challenge_response(runtime.acme_board, token)

    @app.api_route("/", methods=_METHODS)
    @app.api_route("/{path:path}", methods=_METHODS)
    async def proxy(request: Request, path: str = "") -> Response:
        del path
        if request.url.path.startswith(_INTERNAL_PREFIXES):
            return Response(status_code=404)
        return await runtime.proxy.forward(request)

    @app.websocket("/")
    @app.websocket("/{path:path}")
    async def websocket_proxy(websocket: WebSocket, path: str = "") -> None:
        del path
        await runtime.sockets.forward(websocket)

    app.add_middleware(
        WafMiddleware,
        gate=runtime.waf,
        pages=runtime.pages,
        challenges=runtime.challenges,
    )
    if runtime.hsts:
        app.add_middleware(HstsMiddleware)
    app.add_middleware(AllowedHostMiddleware, allowed_hosts=runtime.settings.domains)
    return app
