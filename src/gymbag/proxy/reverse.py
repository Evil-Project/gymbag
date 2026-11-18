import asyncio
import logging

import httpx
from fastapi import Request, WebSocket
from fastapi.responses import PlainTextResponse, Response, StreamingResponse
from websockets.asyncio.client import connect
from websockets.exceptions import WebSocketException

from gymbag.config import Settings
from gymbag.proxy.gateway import (
    UpstreamRouter,
    forward_request_headers,
    forward_response_headers,
    upstream_url,
    websocket_headers,
    websocket_subprotocols,
    websocket_upstream,
)
from gymbag.waf.policy import Action, WafEngine, incoming_from_scope

logger = logging.getLogger(__name__)
_BODYLESS = frozenset({"GET", "HEAD", "OPTIONS"})


class ReverseProxy:
    def __init__(self, settings: Settings, client: httpx.AsyncClient | None = None) -> None:
        self._router = UpstreamRouter(settings.upstream, settings.upstreams)
        self._https_port = settings.https_port
        self._client = client

    def bind(self, client: httpx.AsyncClient) -> None:
        self._client = client

    async def forward(self, request: Request) -> Response:
        if self._client is None:
            raise RuntimeError("reverse proxy client is not ready")
        upstream = self._router.resolve(request.headers.get("host", ""))
        url = upstream_url(upstream, request.url.path, request.scope.get("query_string", b""))
        headers = forward_request_headers(
            request.headers,
            request.client.host if request.client else "",
            request.url.scheme,
        )
        content = None if request.method.upper() in _BODYLESS else request.stream()
        try:
            upstream_request = self._client.build_request(request.method, url, headers=headers, content=content)
            upstream_response = await self._client.send(upstream_request, stream=True)
        except httpx.HTTPError as exc:
            logger.warning("upstream request failed for %s: %s", request.url.path, exc.__class__.__name__)
            return PlainTextResponse("upstream unavailable", status_code=502)

        response_headers = forward_response_headers(
            upstream_response.headers,
            upstream,
            request.headers.get("host", ""),
            self._https_port,
        )

        async def body():
            try:
                if upstream_response.is_stream_consumed:
                    yield upstream_response.content
                else:
                    async for chunk in upstream_response.aiter_raw():
                        yield chunk
            finally:
                await upstream_response.aclose()

        return StreamingResponse(body(), status_code=upstream_response.status_code, headers=response_headers)


class WebSocketProxy:
    def __init__(self, settings: Settings, waf: WafEngine) -> None:
        self._router = UpstreamRouter(settings.upstream, settings.upstreams)
        self._waf = waf

    async def forward(self, websocket: WebSocket) -> None:
        decision = await self._waf.decide(incoming_from_scope(websocket.scope))
        if decision.action is not Action.ALLOW:
            await websocket.close(code=1008)
            return
        upstream = self._router.resolve(websocket.headers.get("host", ""))
        query = bytes(websocket.scope.get("query_string", b"")).decode("latin-1")
        url = websocket_upstream(upstream, websocket.url.path, query)
        try:
            connection = await connect(
                url,
                additional_headers=websocket_headers(websocket.headers),
                subprotocols=websocket_subprotocols(websocket.headers),
                user_agent_header=None,
                proxy=None,
                open_timeout=10,
            )
        except (OSError, TimeoutError, WebSocketException):
            logger.warning("websocket upstream unavailable for %s", websocket.url.path)
            await websocket.close(code=1011)
            return
        await websocket.accept(subprotocol=connection.subprotocol)
        client_to_upstream = asyncio.create_task(self._client_to_upstream(websocket, connection))
        upstream_to_client = asyncio.create_task(self._upstream_to_client(websocket, connection))
        try:
            done, pending = await asyncio.wait(
                {client_to_upstream, upstream_to_client},
                return_when=asyncio.FIRST_COMPLETED,
            )
            for task in pending:
                task.cancel()
            await asyncio.gather(*pending, return_exceptions=True)
            await asyncio.gather(*done, return_exceptions=True)
        finally:
            try:
                await connection.close()
            except WebSocketException:
                pass
            try:
                await websocket.close()
            except RuntimeError:
                pass

    @staticmethod
    async def _client_to_upstream(websocket: WebSocket, connection) -> None:
        while True:
            message = await websocket.receive()
            if message["type"] == "websocket.disconnect":
                return
            if message.get("text") is not None:
                await connection.send(message["text"])
            elif message.get("bytes") is not None:
                await connection.send(message["bytes"])

    @staticmethod
    async def _upstream_to_client(websocket: WebSocket, connection) -> None:
        async for message in connection:
            if isinstance(message, str):
                await websocket.send_text(message)
            else:
                await websocket.send_bytes(message)
