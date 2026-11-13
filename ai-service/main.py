"""Public-corpus chat and an internal AI-only tool proposal protocol."""
from functools import lru_cache
import hmac
from fastapi import Depends, FastAPI, Header
from fastapi.responses import JSONResponse
from pydantic import HttpUrl
from agent import Agent, AgentAnswer, AgentRequest, AgentToolCall, StrictModel, Text
from errors import AIError
from providers import Provider
from presentation import Presentation, PresentationDraft, PresentationRequest
from rag import RAG
from repository import Repository
from settings import Settings


app = FastAPI(title="AI-buddy AI Service", version="1.0.0")


class ChatRequest(StrictModel):
    message: Text


class Source(StrictModel):
    title: Text
    url: HttpUrl


class ChatResponse(StrictModel):
    reply: Text
    sources: list[Source]


class ErrorResponse(StrictModel):
    code: str
    error: str


ERRORS = {status: {"model": ErrorResponse} for status in (403, 502, 503, 504)}


@lru_cache
def get_services():
    config = Settings.from_env()
    provider = Provider(config)
    rag = RAG(provider, Repository(config))
    return config, rag, Agent(provider, rag)


@app.exception_handler(AIError)
async def ai_error_handler(request, exc):
    return JSONResponse(status_code=exc.status, content={"code": exc.code, "error": exc.message})


@app.get("/health")
def health():
    """Liveness only, deliberately does not claim model/database readiness."""
    return {"status": "ok"}


@app.post("/chat", response_model=ChatResponse, responses=ERRORS)
def chat(request: ChatRequest, services=Depends(get_services)):
    return services[1].answer(request.message)


def require_internal(x_ai_internal_token: str = Header(default=""), services=Depends(get_services)):
    expected = services[0].ai_internal_token
    if len(expected) < 32:
        raise AIError("agent_unconfigured", "Внутренний агентский API не настроен")
    if not hmac.compare_digest(x_ai_internal_token.encode(), expected.encode()):
        raise AIError("forbidden", "Доступ запрещён", 403)


@app.post("/agent/step", response_model=AgentAnswer | AgentToolCall, responses=ERRORS,
          dependencies=[Depends(require_internal)])
def agent_step(request: AgentRequest, services=Depends(get_services)):
    return services[2].step(request)


@app.post("/presentation/draft", response_model=PresentationDraft, responses=ERRORS,
          response_model_exclude_unset=True, dependencies=[Depends(require_internal)])
def presentation_draft(request: PresentationRequest, services=Depends(get_services)):
    return Presentation(services[1].provider).draft(request)
