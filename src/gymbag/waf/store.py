import asyncio
import json
import logging
import time
from dataclasses import dataclass
from typing import Protocol

import redis.asyncio as redis

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class ChallengeRecord:
    client_ip: str
    difficulty: int
    attempts: int

    def failed(self) -> ChallengeRecord:
        return ChallengeRecord(self.client_ip, self.difficulty, self.attempts + 1)

    def to_json(self) -> str:
        return json.dumps(
            {"ip": self.client_ip, "difficulty": self.difficulty, "attempts": self.attempts},
            separators=(",", ":"),
        )

    @classmethod
    def from_json(cls, payload: str) -> ChallengeRecord:
        data = json.loads(payload)
        return cls(client_ip=data["ip"], difficulty=int(data["difficulty"]), attempts=int(data["attempts"]))


class StateStore(Protocol):
    async def save_challenge(self, challenge_id: str, record: ChallengeRecord, ttl: int | None) -> None: ...

    async def get_challenge(self, challenge_id: str) -> ChallengeRecord | None: ...

    async def delete_challenge(self, challenge_id: str) -> None: ...

    async def increment(self, key: str, window_seconds: int) -> int: ...

    async def aclose(self) -> None: ...


class MemoryStateStore:
    def __init__(self) -> None:
        self._challenges: dict[str, tuple[ChallengeRecord, float]] = {}
        self._windows: dict[str, tuple[float, int]] = {}
        self._lock = asyncio.Lock()
        self._closed = False

    async def save_challenge(self, challenge_id: str, record: ChallengeRecord, ttl: int | None) -> None:
        async with self._lock:
            if ttl is None:
                current = self._challenges.get(challenge_id)
                expires_at = current[1] if current is not None else time.monotonic() + 1
            else:
                expires_at = time.monotonic() + ttl
            self._challenges[challenge_id] = (record, expires_at)

    async def get_challenge(self, challenge_id: str) -> ChallengeRecord | None:
        async with self._lock:
            current = self._challenges.get(challenge_id)
            if current is None:
                return None
            record, expires_at = current
            if expires_at <= time.monotonic():
                self._challenges.pop(challenge_id, None)
                return None
            return record

    async def delete_challenge(self, challenge_id: str) -> None:
        async with self._lock:
            self._challenges.pop(challenge_id, None)

    async def increment(self, key: str, window_seconds: int) -> int:
        async with self._lock:
            now = time.monotonic()
            started, count = self._windows.get(key, (now, 0))
            if now - started >= window_seconds:
                started, count = now, 0
            count += 1
            self._windows[key] = (started, count)
            return count

    async def aclose(self) -> None:
        self._closed = True


class RedisStateStore:
    def __init__(self, client: redis.Redis) -> None:
        self._redis = client
        self._closed = False

    async def save_challenge(self, challenge_id: str, record: ChallengeRecord, ttl: int | None) -> None:
        key = self._challenge_key(challenge_id)
        expiry = ttl
        if expiry is None:
            remaining = await self._redis.ttl(key)
            expiry = remaining if isinstance(remaining, int) and remaining > 0 else 1
        await self._redis.set(key, record.to_json(), ex=expiry)

    async def get_challenge(self, challenge_id: str) -> ChallengeRecord | None:
        payload = await self._redis.get(self._challenge_key(challenge_id))
        if payload is None:
            return None
        return ChallengeRecord.from_json(payload)

    async def delete_challenge(self, challenge_id: str) -> None:
        await self._redis.delete(self._challenge_key(challenge_id))

    async def increment(self, key: str, window_seconds: int) -> int:
        namespaced = f"gymbag:rate:{key}"
        count = await self._redis.incr(namespaced)
        if int(count) == 1:
            await self._redis.expire(namespaced, window_seconds)
        return int(count)

    async def aclose(self) -> None:
        if self._closed:
            return
        self._closed = True
        await self._redis.aclose()

    @staticmethod
    def _challenge_key(challenge_id: str) -> str:
        return f"gymbag:challenge:{challenge_id}"


async def open_store(redis_url: str) -> StateStore:
    if not redis_url:
        return MemoryStateStore()
    client = redis.from_url(redis_url, decode_responses=True)
    try:
        await client.ping()
    except (OSError, redis.RedisError):
        logger.warning("Redis is unreachable; WAF state will stay in memory")
        await client.aclose()
        return MemoryStateStore()
    return RedisStateStore(client)
