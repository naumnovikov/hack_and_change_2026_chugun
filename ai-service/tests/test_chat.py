import pytest
from fastapi.testclient import TestClient

from main import app


@pytest.fixture
def client():
    with TestClient(app) as test_client:
        yield test_client


@pytest.mark.parametrize("message", ["Привет", "  Нужен доступ к Jira  ", "a" * 8000])
def test_chat_accepts_message(client, message):
    response = client.post("/chat", json={"message": message})

    assert response.status_code == 200
    assert response.json() == {"reply": "Hello from AI"}


@pytest.mark.parametrize(
    "payload",
    [
        {},
        {"message": ""},
        {"message": " \n\t "},
        {"message": None},
        {"message": 123},
        {"message": ["Привет"]},
        {"message": "a" * 8001},
        {"message": "Привет", "user_id": "another-user"},
    ],
)
def test_chat_rejects_invalid_request(client, payload):
    response = client.post("/chat", json=payload)

    assert response.status_code == 422
    assert "detail" in response.json()
    assert "reply" not in response.json()


def test_chat_rejects_malformed_json(client):
    response = client.post(
        "/chat", content='{"message":', headers={"Content-Type": "application/json"}
    )

    assert response.status_code == 422


def test_chat_requires_post(client):
    assert client.get("/chat").status_code == 405
