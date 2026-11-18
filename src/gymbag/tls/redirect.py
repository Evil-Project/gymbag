from fastapi import FastAPI, Request, Response
from fastapi.responses import PlainTextResponse

from gymbag.config import Settings
from gymbag.hosts import hostname_from_header
from gymbag.http import acme_challenge_response
from gymbag.middleware import AllowedHostMiddleware
from gymbag.tls.acme import AcmeHttp01Board


def create_redirect_app(settings: Settings, board: AcmeHttp01Board) -> FastAPI:
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None, redirect_slashes=False)

    @app.get("/.well-known/acme-challenge/{token}")
    async def acme(token: str) -> Response:
        return acme_challenge_response(board, token)

    @app.api_route("/", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])
    @app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])
    async def redirect(request: Request, path: str = "") -> Response:
        del path
        host = hostname_from_header(request.headers.get("host", ""))
        if ":" in host:
            host = f"[{host}]"
        port = "" if settings.https_port == 443 else f":{settings.https_port}"
        query = f"?{request.url.query}" if request.url.query else ""
        location = f"https://{host}{port}{request.url.path}{query}"
        return PlainTextResponse(
            "Redirecting to HTTPS",
            status_code=308,
            headers={"location": location, "cache-control": "no-store"},
        )

    app.add_middleware(AllowedHostMiddleware, allowed_hosts=settings.domains)
    return app
