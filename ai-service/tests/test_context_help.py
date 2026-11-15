import json
from unittest.mock import Mock

import pytest

from agent import Agent
from rag import RAG
from errors import AIError
from test_ai import agent_request, grounded_provider, chunks


@pytest.mark.parametrize("question", ["а какие типы?", "объясни проще", "приведи пример", "а зачем?"])
def test_contextual_followup_keeps_topic(chunks, question):
    provider = grounded_provider(chunks[0])
    history = [{"user": "Что такое SPT?", "assistant": "SPT — работа с обращениями."}]
    RAG(provider, Mock(search=Mock(return_value=chunks[:1]))).answer(question, history)
    assert "Что такое SPT?" in provider.embed.call_args.args[0][0]


def test_two_turns_are_used_without_promoting_history_to_sources(chunks):
    provider = grounded_provider(chunks[0])
    history = [{"user": "Что такое SPT?", "assistant": "SPT — обращения."},
               {"user": "подробнее", "assistant": "Ответ с описанием процесса."}]
    RAG(provider, Mock(search=Mock(return_value=chunks[:1]))).answer("объясни проще", history)
    assert "Что такое SPT?" in provider.embed.call_args.args[0][0]
    prompt = json.loads(provider.chat.call_args.args[0][1]["content"])
    assert prompt["history"] == history
    assert all("chunk_id" in item for item in prompt["context"])


def test_new_explicit_topic_does_not_use_previous_topic_for_retrieval(chunks):
    provider = grounded_provider(chunks[0])
    RAG(provider, Mock(search=Mock(return_value=[]))).answer("А что такое TDR?", [
        {"user": "Что такое SPT?", "assistant": "SPT — обращения."}])
    query = provider.embed.call_args.args[0][0]
    assert "TDR" in query and "SPT" not in query


def test_no_history_followup_asks_topic_without_model_or_database():
    provider, repo = Mock(), Mock()
    result = RAG(provider, repo).answer("объясни проще")
    assert "Уточни" in result["reply"] and result["sources"] == []
    provider.embed.assert_not_called()
    repo.search.assert_not_called()


@pytest.mark.parametrize("question", ["Собери презу по моим таскам", "Как сделать презентацию?",
                                    "Можешь подготовить слайды?"])
def test_presentation_help_never_claims_pdf_or_invokes_tools(question):
    provider, rag = Mock(), Mock()
    result = Agent(provider, rag).step(agent_request(message=question, explicit_access_request=False))
    assert result["state"] == "clarification"
    assert "Создай презентацию" in result["reply"] and "ещё не создан" in result["reply"]
    assert "tool_call" not in result
    provider.chat.assert_not_called()
    rag.answer.assert_not_called()


def test_agent_passes_both_trusted_pairs():
    rag = Mock()
    rag.answer.return_value = {"reply": "ответ", "sources": []}
    turns = [{"role": role, "content": text} for role, text in [
        ("user", "SPT"), ("assistant", "Обращения"),
        ("user", "подробнее"), ("assistant", "Описание")]]
    Agent(Mock(), rag).step(agent_request(message="попроще", explicit_access_request=False, history=turns))
    assert len(rag.answer.call_args.args[1]) == 2


def test_rag_presents_whitespace_normalized_source_without_escape_ambiguity(chunks):
    provider = grounded_provider(chunks[0])
    def respond(messages, **kwargs):
        text = json.loads(messages[1]["content"])["context"][0]["text"]
        assert text == " ".join(chunks[0].text.split())
        assert "\n" not in text and "\\n" not in text
        return {"content": json.dumps({"answer": "Ответ по источнику", "citations": [
            {"chunk_id": "C1", "quote": text}]})}
    provider.chat.side_effect = respond
    assert RAG(provider, Mock(search=Mock(return_value=chunks[:1]))).answer("SPT")["sources"]


def test_literal_escape_sequences_in_quote_are_still_rejected(chunks):
    provider = grounded_provider(chunks[0], citations=[{
        "chunk_id": "C1", "quote": chunks[0].text.replace("\n", "\\n")}])
    with pytest.raises(AIError, match="не подтвердила"):
        RAG(provider, Mock(search=Mock(return_value=chunks[:1]))).answer("SPT")
