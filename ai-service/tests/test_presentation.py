import json
from types import SimpleNamespace

import httpx
import pytest
from fastapi.testclient import TestClient

from main import app, get_services
from presentation import PROPOSAL, TITLES
from providers import Provider
from settings import Settings


TASKS = {"tasks": [{"key": "DEMO-101", "summary": "Добавить валидацию заявки",
                    "description": "Добавлена серверная проверка названия сервиса."}]}
HEADERS = {"X-AI-Internal-Token": "t" * 32}


def valid_draft():
    return {"slides": [
        {"kind": kind, "title": title, "bullets": (
            [] if kind in ("title", "next_steps") else
            [{"text": TASKS["tasks"][0]["description"], "source_keys": ["DEMO-101"]}]),
         **({"is_proposal": True} if kind == "next_steps" else {})}
        for kind, title in TITLES.items()]}


@pytest.fixture
def client_factory():
    def create(handler, token="t" * 32, provider="ollama"):
        settings = Settings(ai_internal_token=token, llm_provider=provider)
        model = Provider(settings, httpx.MockTransport(handler))
        app.dependency_overrides[get_services] = lambda: (settings, SimpleNamespace(provider=model), None)
        return TestClient(app)
    yield create
    app.dependency_overrides.clear()


def model_response(draft):
    return lambda request: httpx.Response(200, json={"message": {"content": json.dumps(draft)}})


def post(client, payload=None, headers=None):
    return client.post("/presentation/draft", json=TASKS if payload is None else payload,
                       headers=HEADERS if headers is None else headers)


@pytest.mark.parametrize("provider", ["ollama", "openai"])
def test_draft_and_provider_request(client_factory, provider):
    seen = []
    def handler(request):
        seen.append(json.loads(request.content))
        message = {"content": json.dumps(valid_draft())}
        return httpx.Response(200, json={"message": message} if provider == "ollama" else
                              {"choices": [{"message": message}]})
    result = post(client_factory(handler, provider=provider))
    assert result.status_code == 200
    assert result.json() == valid_draft()
    assert json.loads(seen[0]["messages"][1]["content"]) == TASKS
    assert (seen[0]["options"]["num_predict"] if provider == "ollama" else seen[0]["max_tokens"]) == 4096
    assert "tools" not in seen[0]


@pytest.mark.parametrize("variant", ["unknown_key", "invented_fact", "count", "order", "extra",
                                    "empty_content", "no_sources", "duplicate_sources", "bad_title",
                                    "title_fact", "missing_proposal", "invented_proposal", "proposal_source"])
def test_reject_untrusted_model_output(client_factory, variant):
    draft = valid_draft()
    bullet = draft["slides"][1]["bullets"][0]
    if variant == "unknown_key":
        bullet["source_keys"] = ["DEMO-999"]
    elif variant == "invented_fact":
        bullet["text"] = "Ускорил сервис на 80 процентов"
    elif variant == "count":
        draft["slides"].pop()
    elif variant == "order":
        draft["slides"].reverse()
    elif variant == "extra":
        draft["attachment"] = {"url": "https://example.org"}
    elif variant == "empty_content":
        draft["slides"][1]["bullets"] = []
    elif variant == "no_sources":
        bullet["source_keys"] = []
    elif variant == "duplicate_sources":
        bullet["source_keys"] *= 2
    elif variant == "bad_title":
        draft["slides"][0]["title"] = "Заработал миллион"
    elif variant == "title_fact":
        draft["slides"][0]["bullets"] = [bullet]
    elif variant == "missing_proposal":
        draft["slides"][-1].pop("is_proposal")
    else:
        draft["slides"][-1]["bullets"] = [{"text": PROPOSAL if variant == "proposal_source" else "Релиз завтра",
                                           "source_keys": ["DEMO-101"] if variant == "proposal_source" else []}]
    result = post(client_factory(model_response(draft)))
    assert result.status_code == 502
    assert result.json()["code"] in ("invalid_model_response", "ungrounded_presentation")


def test_excerpts_and_generic_proposal(client_factory):
    draft = valid_draft()
    draft["slides"][1]["bullets"][0]["text"] = "Добавлена   серверная проверка"
    draft["slides"][2]["bullets"][0]["text"] = "DEMO-101 — Добавить валидацию заявки"
    draft["slides"][3]["bullets"][0]["text"] = "Добавить валидацию заявки"
    draft["slides"][-1]["bullets"] = [{"text": PROPOSAL, "source_keys": []}]
    assert post(client_factory(model_response(draft))).status_code == 200


def test_learning_can_be_empty_without_inventing_skills(client_factory):
    draft = valid_draft()
    draft["slides"][3]["bullets"] = []
    result = post(client_factory(model_response(draft)))
    assert result.status_code == 200
    assert result.json()["slides"][3]["bullets"] == []


def test_learning_still_requires_grounded_facts(client_factory):
    draft = valid_draft()
    draft["slides"][3]["bullets"] = [
        {"text": "Освоил Kubernetes", "source_keys": ["DEMO-101"]}]
    result = post(client_factory(model_response(draft)))
    assert result.status_code == 502
    assert result.json()["code"] == "ungrounded_presentation"


@pytest.mark.parametrize("payload", [{"tasks": []}, {"tasks": TASKS["tasks"] * 2},
                                    {"tasks": TASKS["tasks"], "user_id": "other"},
                                    {"tasks": [{"key": "X", "summary": " ", "description": ""}]},
                                    {"tasks": [{**TASKS["tasks"][0], "status": "done"}]},
                                    {"tasks": [{**TASKS["tasks"][0], "key": 123}]},
                                    {"tasks": [{**TASKS["tasks"][0], "description": "x" * 8001}]}])
def test_bad_input_never_calls_model(client_factory, payload):
    def unexpected(request):
        pytest.fail("Model must not be called")
    assert post(client_factory(unexpected), payload=payload).status_code == 422


@pytest.mark.parametrize("token,headers,status", [("t" * 32, {}, 403), ("", HEADERS, 503),
                                                ("t" * 32, {"X-AI-Internal-Token": "wrong"}, 403)])
def test_internal_credential(client_factory, token, headers, status):
    def unexpected(request):
        pytest.fail("Model must not be called")
    assert post(client_factory(unexpected, token=token), headers=headers).status_code == status


@pytest.mark.parametrize("failure,status,code", [(httpx.ReadTimeout, 504, "model_timeout"),
                                               (httpx.ConnectError, 503, "model_unavailable")])
def test_network_errors(client_factory, failure, status, code):
    def handler(request):
        raise failure("secret upstream details")
    result = post(client_factory(handler))
    assert result.status_code == status
    assert result.json()["code"] == code
    assert "secret" not in result.text


@pytest.mark.parametrize("response,status,code", [
    (httpx.Response(500, text="secret"), 503, "model_error"),
    (httpx.Response(200, text="secret"), 502, "invalid_model_response"),
    (httpx.Response(200, json={"message": {"content": "not json"}}), 502, "invalid_model_response"),
    (httpx.Response(200, json={"message": {"content": None}}), 502, "invalid_model_response"),
])
def test_model_errors(client_factory, response, status, code):
    result = post(client_factory(lambda request: response))
    assert result.status_code == status
    assert result.json()["code"] == code
    assert "secret" not in result.text
