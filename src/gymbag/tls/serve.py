import asyncio
import logging
import os

import uvicorn

from gymbag.context import AppContext
from gymbag.errors import CertificateError, GymbagError
from gymbag.http import create_proxy_app
from gymbag.tls.certificates import hsts_enabled
from gymbag.tls.redirect import create_redirect_app

logger = logging.getLogger(__name__)


async def _wait_until_listening(server: uvicorn.Server, task: asyncio.Task[None]) -> None:
    while not server.started:
        if task.done():
            exception = task.exception()
            if exception is not None:
                raise exception
            raise GymbagError("HTTP server stopped before it started listening")
        await asyncio.sleep(0.05)


async def _renew_certificates(server: uvicorn.Server, ctx: AppContext) -> None:
    elapsed = 0
    while not server.should_exit:
        await asyncio.sleep(1)
        elapsed += 1
        if server.should_exit or elapsed < ctx.settings.renew_interval:
            continue
        elapsed = 0
        if server.config.ssl is None:
            continue
        try:
            updated = await ctx.certificates.renew_if_needed(ctx.settings.renew_within_days)
        except CertificateError:
            logger.exception("certificate renewal failed")
            continue
        if updated is None:
            continue
        password = ctx.settings.key_password
        server.config.ssl.load_cert_chain(
            str(updated.cert_path),
            str(updated.key_path),
            password=password if password else None,
        )
        logger.info("reloaded TLS certificate from %s", updated.cert_path)


async def serve(ctx: AppContext) -> None:
    settings = ctx.settings
    logger.info(
        "HTTP :%s redirects to HTTPS :%s for %s",
        settings.http_port,
        settings.https_port,
        ", ".join(settings.domains),
    )
    logger.info("reverse proxy upstream %s", settings.upstream)
    if (settings.http_port < 1024 or settings.https_port < 1024) and hasattr(os, "geteuid") and os.geteuid() != 0:
        logger.warning(
            "ports below 1024 usually need administrator rights; "
            "set GYMBAG_HTTP_PORT and GYMBAG_HTTPS_PORT for local use"
        )
    redirect = create_redirect_app(settings, ctx.acme_board)
    http_server = uvicorn.Server(
        uvicorn.Config(
            redirect,
            host=settings.bind_host,
            port=settings.http_port,
            log_level="info",
            server_header=False,
            headers=[("server", "gymbag")],
        )
    )
    # The HTTPS server owns signal handling. This listener only has to shut down with it.
    http_task = asyncio.create_task(http_server._serve())
    renew_task: asyncio.Task[None] | None = None
    try:
        await _wait_until_listening(http_server, http_task)
        material = await ctx.certificates.obtain()
        ctx.hsts = hsts_enabled(settings, material.source)
        logger.info("TLS certificate source=%s file=%s", material.source, material.cert_path)
        https_server = uvicorn.Server(
            uvicorn.Config(
                create_proxy_app(ctx),
                host=settings.bind_host,
                port=settings.https_port,
                ssl_certfile=str(material.cert_path),
                ssl_keyfile=str(material.key_path),
                ssl_keyfile_password=settings.key_password,
                log_level="info",
                server_header=False,
                headers=[("server", "gymbag")],
            )
        )
        renew_task = asyncio.create_task(_renew_certificates(https_server, ctx))
        await https_server.serve()
    finally:
        if renew_task is not None:
            renew_task.cancel()
            try:
                await renew_task
            except asyncio.CancelledError:
                pass
        http_server.should_exit = True
        if not http_task.done():
            await http_task
        await ctx.acme.aclose()
        await ctx.store.aclose()
