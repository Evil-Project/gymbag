import secrets

from gymbag.waf.proof import accepts
from gymbag.waf.store import ChallengeRecord, StateStore
from gymbag.waf.submission import ChallengeJSON

MAX_ATTEMPTS = 8


class ChallengeService:
    """Issues a single-use proof and checks the nonce the browser submits."""

    def __init__(self, store: StateStore, difficulty: int, ttl_seconds: int) -> None:
        self._store = store
        self._difficulty = difficulty
        self._ttl = ttl_seconds

    async def issue(self, client_ip: str) -> tuple[str, int]:
        challenge_id = secrets.token_urlsafe(32)
        record = ChallengeRecord(client_ip=client_ip, difficulty=self._difficulty, attempts=0)
        await self._store.save_challenge(challenge_id, record, self._ttl)
        return challenge_id, self._difficulty

    async def verify(self, client_ip: str, submission: ChallengeJSON) -> bool:
        record = await self._store.get_challenge(submission.challenge_id)
        if record is None or record.client_ip != client_ip:
            return False
        if submission.verified and accepts(submission.challenge_id, submission.nonce, record.difficulty):
            await self._store.delete_challenge(submission.challenge_id)
            return True
        updated = record.failed()
        if updated.attempts >= MAX_ATTEMPTS:
            await self._store.delete_challenge(submission.challenge_id)
        else:
            await self._store.save_challenge(submission.challenge_id, updated, None)
        return False
