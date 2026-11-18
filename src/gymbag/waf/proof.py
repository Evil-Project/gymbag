import hashlib


def proof_hex(challenge_id: str, nonce: int) -> str:
    return hashlib.sha256(f"{challenge_id}:{nonce}".encode()).hexdigest()


def accepts(challenge_id: str, nonce: int, difficulty: int) -> bool:
    if difficulty < 1 or nonce < 0:
        return False
    return proof_hex(challenge_id, nonce).startswith("0" * difficulty)
