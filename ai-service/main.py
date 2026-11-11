"""Day-one AI service skeleton; no LLM calls or corporate actions yet."""

from typing import Annotated

from fastapi import FastAPI
from pydantic import BaseModel, ConfigDict, StringConstraints


app = FastAPI(title="AI-buddy AI Service", version="0.1.0")


class ChatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    message: Annotated[
        str,
        StringConstraints(strict=True, strip_whitespace=True, min_length=1, max_length=8000),
    ]


class ChatResponse(BaseModel):
    reply: str


@app.post("/chat", response_model=ChatResponse)
async def chat(request: ChatRequest) -> ChatResponse:
    """Validate a message and return a fixed response for the initial smoke test."""
    return ChatResponse(reply="Hello from AI")
