import asyncio
import sys
import threading
import unittest
from pathlib import Path
from typing import Self

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import httpx
from fastapi.testclient import TestClient
from starlette.websockets import WebSocketDisconnect
from websockets.asyncio.server import serve

from gymbag.context import build_context
from gymbag.http import create_proxy_app
from gymbag.proxy.gateway import (
    forward_request_headers,
    rewrite_location,
    upstream_url,
    websocket_upstream,
)
from tests.test_waf import make_settings


class GatewayTests(unittest.TestCase):
    def test_request_headers_drop_hop_by_hop_and_set_forwarding(self) -> None:
        headers = forward_request_headers(
            {
                "host": "app.example",
                "connection": "close, x-secret",
                "x-secret": "no",
                "x-request": "yes",
                "content-length": "4",
            },
            "203.0.113.10",
            "https",
        )
        self.assertNotIn("connection", {key.lower() for key in headers})
        self.assertNotIn("x-secret", {key.lower() for key in headers})
        self.assertEqual(headers["x-request"], "yes")
        self.assertEqual(headers["x-forwarded-for"], "203.0.113.10")
        self.assertEqual(headers["x-forwarded-proto"], "https")
        self.assertEqual(headers["x-forwarded-host"], "app.example")
        self.assertNotIn("host", {key.lower() for key in headers})

    def test_upstream_url_keeps_the_base_path_and_query(self) -> None:
        url = upstream_url("http://127.0.0.1:5173/app", "/assets/main.js", b"v=1")
        self.assertEqual(str(url), "http://127.0.0.1:5173/app/assets/main.js?v=1")

    def test_location_is_rewritten_to_the_public_origin(self) -> None:
        rewritten = rewrite_location(
            "http://127.0.0.1:5173/moved?ok=1",
            "http://127.0.0.1:5173",
            "https://app.example",
        )
        self.assertEqual(rewritten, "https://app.example/moved?ok=1")
        self.assertEqual(
            rewrite_location("https://other.example/stay", "http://127.0.0.1:5173", "https://app.example"),
            "https://other.example/stay",
        )

    def test_websocket_url_changes_only_the_scheme(self) -> None:
        self.assertEqual(
            websocket_upstream("http://127.0.0.1:5173", "/hmr", "token=1"),
            "ws://127.0.0.1:5173/hmr?token=1",
        )
        self.assertEqual(websocket_upstream("https://upstream.internal", "/hmr", ""), "wss://upstream.internal/hmr")


class ProxyHttpTests(unittest.TestCase):
    def setUp(self) -> None:
        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                201,
                content=request.content,
                headers={
                    "x-echo-method": request.method,
                    "x-echo-proto": request.headers.get("x-forwarded-proto", ""),
                    "x-echo-secret": request.headers.get("x-secret", ""),
                    "location": "http://localhost:5173/moved",
                    "connection": "close",
                },
            )

        self.upstream = httpx.AsyncClient(transport=httpx.MockTransport(handler))
        self.ctx = asyncio.run(
            build_context(
                make_settings(upstream="http://localhost:5173", https_port=443),
                client=self.upstream,
            )
        )
        self.app = create_proxy_app(self.ctx)

    def tearDown(self) -> None:
        asyncio.run(self.upstream.aclose())

    def test_cleared_request_is_streamed_and_locations_are_rewritten(self) -> None:
        with TestClient(self.app) as client:
            client.cookies.set("waf_clearance", self.ctx.clearance.issue("testclient"))
            response = client.post("/", content=b"abc", headers={"connection": "close, x-secret", "x-secret": "no"})
        self.assertEqual(response.status_code, 201)
        self.assertEqual(response.content, b"abc")
        self.assertEqual(response.headers["x-echo-method"], "POST")
        self.assertEqual(response.headers["x-echo-proto"], "http")
        self.assertEqual(response.headers["x-echo-secret"], "")
        self.assertEqual(response.headers["location"], "https://testserver/moved")
        self.assertNotIn("connection", response.headers)

    def test_upstream_outage_is_a_bad_gateway(self) -> None:
        def explode(request: httpx.Request) -> httpx.Response:
            raise httpx.ConnectError("down")

        upstream = httpx.AsyncClient(transport=httpx.MockTransport(explode))
        ctx = asyncio.run(build_context(make_settings(), client=upstream))
        try:
            with TestClient(create_proxy_app(ctx)) as client:
                client.cookies.set("waf_clearance", ctx.clearance.issue("testclient"))
                response = client.get("/health")
            self.assertEqual(response.status_code, 502)
            self.assertNotIn("down", response.text)
        finally:
            asyncio.run(upstream.aclose())

    def test_options_preflight_is_not_challenged(self) -> None:
        with TestClient(self.app) as client:
            response = client.options("/api")
        self.assertEqual(response.status_code, 201)

    def test_trace_is_blocked_before_the_upstream(self) -> None:
        with TestClient(self.app) as client:
            response = client.request("TRACE", "/")
        self.assertEqual(response.status_code, 403)
        self.assertIn("blocked", response.text.lower())


class EchoServer:
    def __init__(self) -> None:
        self.ready = threading.Event()
        self.stop = threading.Event()
        self.port = 0
        self.thread = threading.Thread(target=self._run, daemon=True)

    async def _echo(self, connection) -> None:
        async for message in connection:
            await connection.send(message)

    async def _main(self) -> None:
        server = await serve(self._echo, "127.0.0.1", 0)
        sockets = list(server.sockets)
        self.port = sockets[0].getsockname()[1]
        self.ready.set()
        while not self.stop.is_set():
            await asyncio.sleep(0.02)
        server.close()
        await server.wait_closed()

    def _run(self) -> None:
        asyncio.run(self._main())

    def __enter__(self) -> Self:
        self.thread.start()
        if not self.ready.wait(5):
            raise RuntimeError("websocket test server did not start")
        return self

    def __exit__(self, *args) -> None:
        self.stop.set()
        self.thread.join(5)


class WebSocketProxyTests(unittest.TestCase):
    def test_websocket_requires_clearance_and_then_proxies(self) -> None:
        with EchoServer() as echo:
            upstream = httpx.AsyncClient(transport=httpx.MockTransport(lambda request: httpx.Response(200)))
            ctx = asyncio.run(
                build_context(make_settings(upstream=f"http://127.0.0.1:{echo.port}"), client=upstream)
            )
            try:
                with TestClient(create_proxy_app(ctx)) as client:
                    denied = client.websocket_connect("/chat")
                    with self.assertRaises(WebSocketDisconnect), denied as socket:
                        socket.receive_text()
                    client.cookies.set("waf_clearance", ctx.clearance.issue("testclient"))
                    with client.websocket_connect("/chat") as socket:
                        socket.send_text("ping")
                        self.assertEqual(socket.receive_text(), "ping")
            finally:
                asyncio.run(upstream.aclose())
