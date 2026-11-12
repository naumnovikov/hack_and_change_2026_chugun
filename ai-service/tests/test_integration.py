"""Opt-in live checks. Unit tests never download a model or require a DB."""
import os
from pathlib import Path
import uuid
import psycopg
import pytest
from agent import Agent, AgentRequest
from chunking import load_corpus
from providers import Provider
from rag import RAG
from repository import Repository
from errors import AIError
from settings import Settings


@pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL is not set")
def test_pgvector_atomic_replace_and_cosine_search():
    config = Settings(database_url=os.environ["TEST_DATABASE_URL"], embedding_dimensions=3,
                      embedding_model="test-" + uuid.uuid4().hex, rag_max_distance=0.1)
    chunks = load_corpus(Path(__file__).resolve().parents[1] / "knowledge")[:2]
    repository = Repository(config)
    try:
        repository.replace(chunks, [[1, 0, 0], [0, 1, 0]])
        assert repository.search([1, 0, 0]) == chunks[:1]
        repository.replace(chunks[:1], [[1, 0, 0]])
        assert repository.search([0, 1, 0]) == []
        with pytest.raises(AIError):
            repository.replace([chunks[0], chunks[0]], [[1, 0, 0], [1, 0, 0]])
        assert repository.search([1, 0, 0]) == chunks[:1]
    finally:
        with psycopg.connect(config.database_url) as connection:
            connection.execute("DELETE FROM ai_knowledge_chunks WHERE collection = %s", [config.collection])


@pytest.mark.skipif(os.getenv("AI_LIVE_TESTS") != "1", reason="AI_LIVE_TESTS is not enabled")
def test_live_embeddings_and_grounded_generation():
    config = Settings.from_env()
    provider = Provider(config)
    chunks = load_corpus(config.knowledge_dir)
    vectors = provider.embed([chunk.text for chunk in chunks])
    assert len(vectors) == 6
    class Corpus:
        def search(self, vector):
            import math
            scored = []
            for chunk, embedding in zip(chunks, vectors):
                cosine = sum(a*b for a, b in zip(vector, embedding)) / (
                    math.sqrt(sum(a*a for a in vector)) * math.sqrt(sum(b*b for b in embedding)))
                if 1 - cosine <= config.rag_max_distance:
                    scored.append((cosine, chunk))
            return [chunk for _, chunk in sorted(scored, key=lambda pair: pair[0], reverse=True)[:config.rag_top_k]]
    result = RAG(provider, Corpus()).answer("Что такое SPT?")
    print(result)
    assert result["sources"], "No source: evaluate RAG_MAX_DISTANCE for this embedding model"
    assert "SPT" in result["reply"]


@pytest.mark.skipif(os.getenv("AI_LIVE_TESTS") != "1", reason="AI_LIVE_TESTS is not enabled")
@pytest.mark.parametrize("name,message,extras", [
    ("create_access_request", "Создай демо-заявку на доступ к Jira.", {"explicit_access_request": True}),
    ("find_employee", "Найди владельца сервиса billing для ревью.", {}),
    ("send_message", "Отправь подтверждённое сообщение проверенному получателю.", {
        "allowed_recipients": ["demo-employee-1"],
        "confirmed_message": {"recipient_id": "demo-employee-1", "text": "Посмотри, пожалуйста, PR DEMO-42."}}),
])
def test_live_tool_proposals(name, message, extras):
    class NoRAG:
        def answer(self, question):
            raise AssertionError("Expected a tool proposal")
    request = AgentRequest.model_validate({
        "request_id": "live-" + name, "conversation_id": "live-conversation", "message": message,
        "user_context": {"user_id": "demo-user", "role": "Intern", "team_id": "demo", "onboarding_week": 1},
        "allowed_services": ["Jira", "billing"], **extras,
    })
    result = Agent(Provider(Settings.from_env()), NoRAG()).step(request)
    print(result)
    assert result["kind"] == "tool_call"
    assert result["tool_call"]["name"] == name
