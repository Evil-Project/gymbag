import hashlib
import hmac
import time


def _b64(data: bytes) -> str:
    import base64

    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def _b64decode(data: str) -> bytes:
    import base64

    padding = "=" * (-len(data) % 4)
    return base64.urlsafe_b64decode(data + padding)


class ClearanceTokens:
    """Issues and checks the cookie that marks a solved challenge."""

    def __init__(self, secret: bytes, ttl_seconds: int) -> None:
        self._secret = secret
        self._ttl = ttl_seconds

    def issue(self, client_ip: str) -> str:
        expires = int(time.time()) + self._ttl
        body = _b64(f"{expires}:{self._binding(client_ip)}".encode())
        return f"{body}.{self._sign(body)}"

    def valid(self, token: str | None, client_ip: str) -> bool:
        if not token or token.count(".") != 1:
            return False
        body, signature = token.split(".", 1)
        expected = self._sign(body)
        if len(signature) != len(expected) or not hmac.compare_digest(signature, expected):
            return False
        try:
            expires_text, binding = _b64decode(body).decode().split(":", 1)
            expires = int(expires_text)
        except (ValueError, UnicodeError):
            return False
        if expires < int(time.time()):
            return False
        expected_binding = self._binding(client_ip)
        return len(binding) == len(expected_binding) and hmac.compare_digest(binding, expected_binding)

    def _binding(self, client_ip: str) -> str:
        return hashlib.sha256(client_ip.encode()).hexdigest()

    def _sign(self, body: str) -> str:
        digest = hmac.new(self._secret, body.encode(), hashlib.sha256).digest()
        return _b64(digest)
