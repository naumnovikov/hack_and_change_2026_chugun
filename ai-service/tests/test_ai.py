import json
from pathlib import Path
from unittest.mock import Mock
from fastapi.testclient import TestClient
import httpx
import psycopg
import pytest
from agent import Agent, AgentRequest
from chunking import load_corpus
from errors import AIError
from ingest import ingest
from main import app, get_services
from providers import Provider, validate_vector
from rag import NO_CONTEXT, RAG
from repository import Repository
from settings import Settings


@pytest.fixture
def chunks():
    return load_corpus(Path(__file__).resolve().parents[1] / "knowledge")


def fake_provider(response, provider="ollama", dimensions=3):
    def handler(request):
        return httpx.Response(200, json=response)
    return Provider(Settings(llm_provider=provider, embedding_dimensions=dimensions), httpx.MockTransport(handler))


@pytest.mark.parametrize("provider,data", [
    ("ollama", {"embeddings": [[1, 2, 3]]}),
    ("openai", {"data": [{"index": 0, "embedding": [1, 2, 3]}]}),
])
def test_embeddings_adapters(provider, data):
    model = fake_provider(data, provider)
    assert model.embed(["SPT"]) == [[1, 2, 3]]


@pytest.mark.parametrize("provider,base_url,expected_dimensions", [
    ("ollama", "http://ollama:11434/v1", None),
    ("openai", "https://generativelanguage.googleapis.com/v1beta/openai/", 3),
    ("openai", "https://api.openai.com/v1", None),
])
def test_embeddings_send_expected_dimensions(provider, base_url, expected_dimensions):
    seen = []

    def handler(request):
        seen.append(json.loads(request.content))
        data = {"embeddings": [[1, 2, 3]]} if provider == "ollama" else {
            "data": [{"index": 0, "embedding": [1, 2, 3]}]}
        return httpx.Response(200, json=data)

    model = Provider(
        Settings(llm_provider=provider, llm_base_url=base_url, embedding_dimensions=3),
        httpx.MockTransport(handler),
    )
    model.embed(["SPT"])

    assert seen[0].get("dimensions") == expected_dimensions


def test_openai_embeddings_preserve_order_when_provider_omits_indices():
    response = {"data": [
        {"object": "embedding", "embedding": [1, 0, 0]},
        {"object": "embedding", "embedding": [0, 1, 0]},
    ]}
    assert fake_provider(response, provider="openai").embed(["first", "second"]) == [
        [1, 0, 0], [0, 1, 0]]


@pytest.mark.parametrize("data", [{}, {"embeddings": []}, {"embeddings": [[0, 0, 0]]},
                                 {"embeddings": [[1, 2]]}, {"embeddings": None}])
def test_bad_embeddings(data):
    with pytest.raises(AIError):
        fake_provider(data).embed(["SPT"])


@pytest.mark.parametrize("vector", [[True, 1, 2], [float("inf"), 1, 2], "bad"])
def test_non_numeric_vectors(vector):
    with pytest.raises(AIError):
        validate_vector(vector, 3)


def test_provider_demo_is_explicit_and_deterministic():
    provider = Provider(Settings(llm_provider="demo", embedding_dimensions=8))
    assert provider.embed(["SPT"])[0] == provider.embed(["SPT"])[0]
    assert len(provider.embed(["SPT"])[0]) == 8


@pytest.mark.parametrize("provider,data", [
    ("ollama", {"message": {"content": "ok"}}),
    ("openai", {"choices": [{"message": {"content": "ok"}}]}),
])
@pytest.mark.parametrize("structured", [True, {"type": "object"}])
def test_chat_adapters_send_tools_and_json_format(provider, data, structured):
    seen = []
    def handler(request):
        seen.append(json.loads(request.content))
        return httpx.Response(200, json=data)
    output = Provider(Settings(llm_provider=provider, llm_api_key="test-only"), httpx.MockTransport(handler)).chat(
        [], tools=[{"type": "function"}], structured=structured)
    assert output == {"content": "ok"}
    assert seen[0]["tools"] and not seen[0]["stream"]
    assert ("format" if provider == "ollama" else "response_format") in seen[0]


def test_provider_sends_configured_api_key_as_bearer():
    seen = []

    def handler(request):
        seen.append(request.headers.get("Authorization"))
        return httpx.Response(200, json={"choices": [{"message": {"content": "ok"}}]})

    provider = Provider(
        Settings(llm_provider="openai", llm_api_key="test-key"),
        httpx.MockTransport(handler),
    )
    provider.chat([])

    assert seen == ["Bearer test-key"]


def test_google_openai_compatible_structured_chat_uses_json_object():
    seen = []

    def handler(request):
        seen.append(json.loads(request.content))
        return httpx.Response(200, json={"choices": [{"message": {"content": "{}"}}]})

    provider = Provider(
        Settings(
            llm_provider="openai",
            llm_base_url="https://generativelanguage.googleapis.com/v1beta/openai/",
        ),
        httpx.MockTransport(handler),
    )
    provider.chat([], structured={"type": "object"})

    assert seen[0]["response_format"] == {"type": "json_object"}


def test_other_openai_compatible_structured_chat_keeps_json_schema():
    seen = []

    def handler(request):
        seen.append(json.loads(request.content))
        return httpx.Response(200, json={"choices": [{"message": {"content": "{}"}}]})

    schema = {"type": "object", "properties": {"reply": {"type": "string"}}}
    provider = Provider(
        Settings(llm_provider="openai", llm_base_url="https://api.openai.com/v1"),
        httpx.MockTransport(handler),
    )
    provider.chat([], structured=schema)

    assert seen[0]["response_format"]["type"] == "json_schema"
    assert seen[0]["response_format"]["json_schema"]["schema"] == schema


@pytest.mark.parametrize("data", [{}, {"message": []}, {"choices": []}])
def test_invalid_chat_response(data):
    with pytest.raises(AIError):
        fake_provider(data).chat([])


@pytest.mark.parametrize("failure,code", [(httpx.ReadTimeout, "model_timeout"), (httpx.ConnectError, "model_unavailable")])
def test_model_network_failures(failure, code):
    def handler(request):
        raise failure("secret backend diagnostics")
    with pytest.raises(AIError) as raised:
        Provider(Settings(), httpx.MockTransport(handler)).chat([])
    assert raised.value.code == code
    assert "secret" not in raised.value.message


@pytest.mark.parametrize("status,body,code", [(500, b"secret", "model_error"), (200, b"invalid json", "invalid_model_response")])
def test_model_http_failures(status, body, code):
    def handler(request):
        return httpx.Response(status, content=body)
    with pytest.raises(AIError) as raised:
        Provider(Settings(), httpx.MockTransport(handler)).chat([])
    assert raised.value.code == code


def grounded_provider(chunk, **changes):
    provider = Mock(settings=Settings())
    provider.embed.return_value = [[1, 2, 3]]
    answer = {"answer": "SPT — работа с обращениями пользователей.",
              "citations": [{"chunk_id": "C1", "quote": chunk.text[-30:]}]}
    answer.update(changes)
    provider.chat.return_value = {"content": json.dumps(answer)}
    return provider


def test_rag_answer_and_source_are_grounded(chunks):
    provider = grounded_provider(chunks[0])
    result = RAG(provider, Mock(search=Mock(return_value=[chunks[0]]))).answer("Что такое SPT?")
    assert "SPT" in result["reply"]
    assert result["sources"][0]["url"] == str(chunks[0].source_url)
    assert "system" == provider.chat.call_args.args[0][0]["role"]
    assert "По открытой публикации AvitoTech Playbook:" in result["reply"]


def test_rag_expands_curated_synonyms_before_embedding(chunks):
    provider = grounded_provider(chunks[0])
    repo = Mock(search=Mock(return_value=[chunks[0]]))
    RAG(provider, repo).answer("Как разобраться с таской?")
    query = provider.embed.call_args.args[0][0]
    assert "задача" in query
    assert "task" in query


def test_rag_uses_prior_turn_to_resolve_short_followup(chunks):
    provider = grounded_provider(chunks[0])
    repo = Mock(search=Mock(return_value=[chunks[0]]))
    history = [{"user": "Что значит SPT?", "assistant": "SPT — процесс работы с обращениями."}]

    RAG(provider, repo).answer("подробнее", history)

    query = provider.embed.call_args.args[0][0]
    assert "Что значит SPT?" in query
    prompt = json.loads(provider.chat.call_args.args[0][1]["content"])
    assert prompt["history"] == history
    assert prompt["question"] == "подробнее"


@pytest.mark.parametrize("question", ["А как её сдавать?", "А как таску сдавать?"])
def test_rag_uses_prior_turn_for_task_followups(chunks, question):
    provider = grounded_provider(chunks[0])
    repo = Mock(search=Mock(return_value=[chunks[0]]))
    history = [{
        "user": "Давай сделаем задачу: написать новую фичу",
        "assistant": "Для запроса на новую возможность используется тип обращения Feature.",
    }]

    RAG(provider, repo).answer(question, history)

    query = provider.embed.call_args.args[0][0]
    assert "Предыдущий вопрос:" in query
    assert history[0]["user"] in query


def test_rag_does_not_apply_old_context_to_new_task_question(chunks):
    provider = grounded_provider(chunks[0])
    repo = Mock(search=Mock(return_value=[chunks[0]]))
    history = [{"user": "Что значит SPT?", "assistant": "SPT — процесс работы с обращениями."}]

    RAG(provider, repo).answer("давай сделаем задачу: написать новую фичу", history)

    query = provider.embed.call_args.args[0][0]
    assert "Предыдущий вопрос:" not in query
    assert "написать новую фичу" in query


def test_rag_accepts_known_latin_alias_instead_of_exact_source_token(chunks):
    provider = grounded_provider(chunks[0])
    repo = Mock(search=Mock(return_value=[chunks[0]]))
    result = RAG(provider, repo).answer("Что означает support?")
    assert result["sources"]


@pytest.mark.parametrize("question,found", [("Что такое SPT?", False), ("Что такое ZBP?", True)])
def test_no_context_does_not_call_llm(chunks, question, found):
    provider = grounded_provider(chunks[0])
    result = RAG(provider, Mock(search=Mock(return_value=chunks[:1] if found else []))).answer(question)
    assert result == {"reply": NO_CONTEXT, "sources": []}
    provider.chat.assert_not_called()


@pytest.mark.parametrize("citations", [[{"chunk_id": "fake", "quote": "invented quote"}],
                                      [{"chunk_id": "C1", "quote": "invented quote"}]])
def test_rag_rejects_fabricated_sources(chunks, citations):
    with pytest.raises(AIError):
        RAG(grounded_provider(chunks[0], citations=citations), Mock(search=Mock(return_value=chunks[:1]))).answer("SPT")


def test_rag_empty_or_malformed_model_answers(chunks):
    provider = grounded_provider(chunks[0], citations=[])
    rag = RAG(provider, Mock(search=Mock(return_value=chunks[:1])))
    assert rag.answer("SPT")["reply"] == NO_CONTEXT
    provider.chat.return_value = {"content": "not json"}
    with pytest.raises(AIError):
        rag.answer("SPT")


def test_rag_allows_line_wrap_changes_in_exact_quote(chunks):
    provider = grounded_provider(chunks[0], citations=[{"chunk_id": "C1", "quote": " ".join(chunks[0].text.split())}])
    assert RAG(provider, Mock(search=Mock(return_value=chunks[:1]))).answer("SPT")["sources"]


def test_rag_demo_labels_excerpt(chunks):
    result = RAG(Provider(Settings(llm_provider="demo")), Mock(search=Mock(return_value=chunks[:1]))).answer("SPT")
    assert result["reply"].startswith("Демонстрационный режим")


def agent_request(**changes):
    data = {"request_id": "req-1", "conversation_id": "conv-1", "message": "Создай заявку на Jira",
            "user_context": {"user_id": "demo-1", "role": "Intern", "team_id": "demo", "onboarding_week": 1},
            "allowed_services": ["Jira"], "explicit_access_request": True}
    return AgentRequest.model_validate({**data, **changes})


def call_provider(name="create_access_request", arguments=None):
    provider = Mock(settings=Settings())
    provider.chat.return_value = {"tool_calls": [{"function": {
        "name": name, "arguments": arguments if arguments is not None else {"service_name": "Jira"}}}]}
    return provider


def test_agent_proposes_one_tool_and_keeps_operation_identity():
    agent = Agent(call_provider(), Mock())
    result = agent.step(agent_request())
    assert result["kind"] == "tool_call"
    assert result["tool_call"]["arguments"] == {"service_name": "Jira"}
    assert result == agent.step(agent_request())
    assert agent.step(agent_request(conversation_id="conv-2"))["tool_call"]["call_id"] != result["tool_call"]["call_id"]


@pytest.mark.parametrize("name,args", [("unknown", {}),
    ("create_access_request", {"service_name": "Jira", "user_id": "victim"}),
    ("send_message", {"recipient_id": "demo-2", "text": " "}), ("find_employee", "not json")])
def test_agent_rejects_unknown_tools_and_argument_injection(name, args):
    with pytest.raises(AIError):
        Agent(call_provider(name, args), Mock()).step(agent_request())


def test_agent_rejects_parallel_tool_calls():
    provider = call_provider()
    provider.chat.return_value["tool_calls"] *= 2
    with pytest.raises(AIError):
        Agent(provider, Mock()).step(agent_request())


def test_agent_requires_access_intent_and_allowed_service():
    agent = Agent(call_provider(), Mock())
    assert agent.step(agent_request(explicit_access_request=False))["state"] == "clarification"
    assert agent.step(agent_request(allowed_services=[]))["state"] == "denied"


@pytest.mark.parametrize("message", [
    "Как сделать доступ?",
    "Как оформить заявку?",
    "Нужен доступ к Jira",
])
def test_access_help_explains_how_to_make_demo_request(message):
    provider = call_provider()
    rag = Mock()

    result = Agent(provider, rag).step(agent_request(
        message=message, explicit_access_request=False,
    ))

    assert result["state"] == "clarification"
    assert "Оформи доступ к Jira" in result["reply"]
    assert "Оформи доступ к Confluence" in result["reply"]
    assert "не выдаст реальный доступ" in result["reply"]
    provider.chat.assert_not_called()
    rag.answer.assert_not_called()


@pytest.mark.parametrize("message", [
    "Давай сделаем задачу: написать новую фичу",
    "Как сдавать PR?",
    "Что дальше делать?",
])
def test_general_work_questions_use_rag_without_action_tools(message):
    provider = call_provider()
    rag = Mock()
    rag.answer.return_value = {"reply": "Ответ по источникам.", "sources": []}

    result = Agent(provider, rag).step(agent_request(
        message=message, explicit_access_request=False, allowed_services=[],
    ))

    assert result["reply"] == "Ответ по источникам."
    rag.answer.assert_called_once_with(message, [])
    provider.chat.assert_not_called()


def test_employee_lookup_tool_requires_explicit_owner_intent_and_known_service():
    provider = call_provider("find_employee", {"service_name": "billing"})
    request = agent_request(
        message="Who owns billing?", explicit_access_request=False, allowed_services=["billing"],
    )

    result = Agent(provider, Mock()).step(request)

    assert result["kind"] == "tool_call"
    tools = provider.chat.call_args.kwargs["tools"]
    assert [tool["function"]["name"] for tool in tools] == ["find_employee"]


def test_message_requires_exact_confirmation_and_verified_recipient():
    args = {"recipient_id": "demo-2", "text": "Посмотри PR"}
    agent = Agent(call_provider("send_message", args), Mock())
    assert agent.step(agent_request())["state"] == "confirmation_required"
    confirmed = agent_request(confirmed_message=args, allowed_recipients=["demo-2"])
    assert agent.step(confirmed)["kind"] == "tool_call"
    confirmed.confirmed_message.text = "Другой текст"
    assert agent.step(confirmed)["state"] == "confirmation_required"


def test_confirmed_message_whitespace_is_preserved():
    args = {"recipient_id": "demo-2", "text": "  Посмотри PR\n"}
    request = agent_request(confirmed_message=args, allowed_recipients=["demo-2"])
    result = Agent(call_provider("send_message", args), Mock()).step(request)
    assert result["tool_call"]["arguments"]["text"] == args["text"]


@pytest.mark.parametrize("response", [{"tool_calls": "bad"}, {"content": {"bad": "format"}}])
def test_agent_rejects_malformed_model_envelopes(response):
    provider = call_provider()
    provider.chat.return_value = response
    with pytest.raises(AIError):
        Agent(provider, Mock()).step(agent_request())


@pytest.mark.parametrize("name,result,expected", [
    ("create_access_request", {"ticket_id": "DEMO-1", "status": "created"}, "DEMO-1"),
    ("send_message", {"message_id": "DEMO-MSG", "sent_at": "2026-11-12T19:00:00+03:00"}, "DEMO-MSG"),
    ("find_employee", {"employee_id": "demo-2", "name": "Демо", "role": "Lead", "mattermost_id": "@demo"}, "Демо"),
])
def test_tool_results_are_reported_without_final_model_call(name, result, expected):
    provider = call_provider()
    response = Agent(provider, Mock()).step(agent_request(tool_result={
        "call_id": "call-1", "name": name, "status": "success", "result": result}))
    assert expected in response["reply"]
    provider.chat.assert_not_called()


@pytest.mark.parametrize("status", ["unknown", "error"])
def test_tool_failure_is_not_automatically_retried(status):
    provider = call_provider()
    response = Agent(provider, Mock()).step(agent_request(tool_result={
        "call_id": "call-1", "name": "send_message", "status": status}))
    assert response["state"] == status
    provider.chat.assert_not_called()


@pytest.mark.parametrize("name,result", [("create_access_request", {}),
    ("send_message", {"message_id": "MSG", "sent_at": "2026-99-12T00:00:00Z"})])
def test_invalid_tool_result_never_claims_success(name, result):
    with pytest.raises(AIError):
        Agent(call_provider(), Mock()).step(agent_request(tool_result={
            "call_id": "call-1", "name": name, "status": "success", "result": result}))


def test_agent_limit_financial_refusal_and_untrusted_model_prose():
    provider = call_provider()
    agent = Agent(provider, Mock())
    assert agent.step(agent_request(tool_steps=3))["state"] == "limit"
    assert agent.step(agent_request(message="Переведи зарплату"))["state"] == "denied"
    provider.chat.assert_not_called()
    provider.chat.return_value = {"content": "Я уже создал заявку 123"}
    assert "123" not in agent.step(agent_request())["reply"]
    provider.chat.return_value = {"content": ""}
    rag = Mock()
    rag.answer.return_value = {"reply": NO_CONTEXT, "sources": []}
    assert Agent(provider, rag).step(agent_request())["reply"] == NO_CONTEXT


def test_agent_demo_uses_grounded_rag_only():
    rag = Mock()
    rag.answer.return_value = {"reply": NO_CONTEXT, "sources": []}
    assert Agent(Provider(Settings(llm_provider="demo")), rag).step(agent_request())["kind"] == "answer"


def test_agent_passes_recent_turn_for_followup_questions():
    rag = Mock()
    rag.answer.return_value = {"reply": "Подробный ответ", "sources": []}
    request = agent_request(
        message="подробнее",
        explicit_access_request=False,
        history=[
            {"role": "user", "content": "Что значит SPT?"},
            {"role": "assistant", "content": "SPT — процесс работы с обращениями."},
        ],
    )
    provider = Mock(settings=Settings())
    provider.chat.return_value = {"content": ""}

    response = Agent(provider, rag).step(request)

    assert response["reply"] == "Подробный ответ"
    rag.answer.assert_called_once_with("подробнее", [
        {"user": "Что значит SPT?", "assistant": "SPT — процесс работы с обращениями."}
    ])


def test_http_internal_boundary_and_safe_errors():
    config = Settings(ai_internal_token="x" * 32)
    rag, agent = Mock(), Mock()
    agent.step.return_value = {"kind": "answer", "state": "answer", "reply": "Тест", "sources": []}
    app.dependency_overrides[get_services] = lambda: (config, rag, agent)
    try:
        with TestClient(app) as client:
            assert client.get("/health").json() == {"status": "ok"}
            payload = agent_request().model_dump()
            assert client.post("/agent/step", json=payload).status_code == 403
            assert client.post("/agent/step", json=payload, headers={"X-AI-Internal-Token": "x" * 32}).status_code == 200
            config.ai_internal_token = ""
            assert client.post("/agent/step", json=payload).status_code == 503
            rag.answer.side_effect = AIError("database_error", "База недоступна")
            response = client.post("/chat", json={"message": "SPT"})
            assert response.status_code == 503 and response.json()["code"] == "database_error"
    finally:
        app.dependency_overrides.clear()


def test_service_wiring_and_environment(monkeypatch):
    monkeypatch.setenv("EMBEDDING_DIMENSIONS", "16")
    assert Settings.from_env().embedding_dimensions == 16
    get_services.cache_clear()
    assert get_services()[0].embedding_dimensions == 16
    get_services.cache_clear()


def test_ingest_is_repeatable_and_replaces_after_embedding(chunks):
    provider, repository = Mock(), Mock()
    provider.embed.return_value = [[1, 2, 3]] * len(chunks)
    assert ingest(Settings(), provider, repository) == len(chunks)
    repository.replace.assert_called_once()
    provider.embed.side_effect = AIError("model_unavailable", "Нет модели")
    repository.reset_mock()
    with pytest.raises(AIError):
        ingest(Settings(), provider, repository)
    repository.replace.assert_not_called()


def test_ingest_empty_corpus(tmp_path):
    (tmp_path / "sources.json").write_text("[]")
    with pytest.raises(ValueError):
        ingest(Settings(knowledge_dir=tmp_path), Mock(), Mock())


def test_ingest_cli_reports_safe_error_without_diagnostics(monkeypatch, capsys):
    import ingest as module
    failure = AIError("database_error", "База недоступна")
    failure.__cause__ = psycopg.OperationalError("password=secret")
    monkeypatch.setattr(module, "ingest", Mock(side_effect=failure))
    assert module.main() == 1
    assert "secret" not in capsys.readouterr().out
    monkeypatch.setattr(module, "ingest", Mock(return_value=6))
    assert module.main() == 0
    assert "6" in capsys.readouterr().out


def connection_mock():
    connection = Mock()
    connection.__enter__ = Mock(return_value=connection)
    connection.__exit__ = Mock(return_value=False)
    return connection


def test_repository_transaction_and_search(chunks):
    connection = connection_mock()
    repository = Repository(Settings(database_url="test", embedding_dimensions=3), Mock(return_value=connection))
    repository.replace(chunks[:1], [[1, 2, 3]])
    statements = [str(call.args[0]) for call in connection.execute.call_args_list]
    assert "DELETE" in statements[3] and "INSERT" in statements[4]
    connection.execute.return_value.fetchall.return_value = [(chunks[0].model_dump(mode="json"),)]
    assert repository.search([1, 2, 3]) == chunks[:1]
    assert connection.execute.call_args.args[1][0] == repository.settings.collection
    assert "source_type" in connection.execute.call_args.args[0]


def test_repository_errors_are_safe(chunks):
    repository = Repository(Settings(embedding_dimensions=3))
    with pytest.raises(AIError, match="Не настроено"):
        repository.search([1, 2, 3])
    with pytest.raises(ValueError):
        repository.replace([], [])
    failing = Mock(side_effect=psycopg.OperationalError("password=secret"))
    repository = Repository(Settings(database_url="test", embedding_dimensions=3), failing)
    with pytest.raises(AIError, match="Не удалось обновить"):
        repository.replace(chunks[:1], [[1, 2, 3]])
    with pytest.raises(AIError, match="База знаний"):
        repository.search([1, 2, 3])
