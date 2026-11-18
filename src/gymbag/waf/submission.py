from pydantic import BaseModel, ConfigDict, Field


class ChallengeJSON(BaseModel):
    model_config = ConfigDict(strict=True, extra="forbid")
    challenge_id: str = Field(min_length=1, max_length=128)
    nonce: int = Field(ge=0, le=50_000_000)
    verified: bool
