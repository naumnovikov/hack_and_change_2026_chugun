"""Ollama / OpenAI-compatible adapters with bounded calls and no write retries."""
import hashlib
import math
import re
from urllib.parse import urlparse
import httpx
from errors import AIError
from settings import Settings


def validate_vector(vector, dimensions):
    if (not isinstance(vector, list) or len(vector) != dimensions
            or any(type(x) not in (int, float) or not math.isfinite(x) for x in vector)
            or not any(vector)):
        raise AIError("invalid_embedding", "Модель вернула некорректный вектор", 502)
    return vector


class Provider:
    def __init__(self, settings: Settings, transport=None):
        self.settings, self.transport = settings, transport

    def _post(self, path, payload):
        config = self.settings
        base = config.llm_base_url.rstrip("/")
        if config.llm_provider == "ollama":
            base = base.removesuffix("/v1")
        headers = {"Authorization": f"Bearer {config.llm_api_key}"} if config.llm_api_key else {}
        try:
            with httpx.Client(timeout=config.timeout_seconds, transport=self.transport,
                              trust_env=False) as client:
                response = client.post(base + path, json=payload, headers=headers)
                response.raise_for_status()
                return response.json()
        except httpx.TimeoutException as exc:
            raise AIError("model_timeout", "Модель не ответила вовремя", 504) from exc
        except httpx.HTTPStatusError as exc:
            raise AIError("model_error", "Модель недоступна или запрос отклонён") from exc
        except httpx.RequestError as exc:
            raise AIError("model_unavailable", "Не удалось подключиться к модели") from exc
        except ValueError as exc:
            raise AIError("invalid_model_response", "Модель вернула некорректный ответ", 502) from exc

    def embed(self, texts: list[str]) -> list[list[float]]:
        config = self.settings
        if config.llm_provider == "demo":
            vectors = []
            for text in texts:
                vector = [0.0] * config.embedding_dimensions
                for word in re.findall(r"\w+", text.casefold()):
                    index = int.from_bytes(hashlib.sha256(word.encode()).digest()[:4], "big")
                    vector[index % len(vector)] += 1
                vectors.append(validate_vector(vector, config.embedding_dimensions))
            return vectors
        native = config.llm_provider == "ollama"
        google_compatible = urlparse(config.llm_base_url).hostname == "generativelanguage.googleapis.com"
        data = self._post("/api/embed" if native else "/embeddings", {
            "model": config.embedding_model, "input": texts,
            **({"dimensions": config.embedding_dimensions} if google_compatible else {}),
        })
        try:
            if native:
                vectors = data["embeddings"]
            else:
                items = data["data"]
                if items and all("index" in item for item in items):
                    items = sorted(items, key=lambda item: item["index"])
                vectors = [item["embedding"] for item in items]
            if len(vectors) != len(texts):
                raise ValueError("batch length")
            return [validate_vector(vector, config.embedding_dimensions) for vector in vectors]
        except (KeyError, TypeError, ValueError) as exc:
            raise AIError("invalid_embedding", "Модель вернула некорректные embeddings", 502) from exc

    def chat(self, messages, tools=None, structured=False, max_tokens=512):
        config = self.settings
        native = config.llm_provider == "ollama"
        google_compatible = urlparse(config.llm_base_url).hostname == "generativelanguage.googleapis.com"
        payload = {"model": config.llm_model, "messages": messages, "stream": False}
        if native:
            payload.update(think=False, options={"temperature": 0, "num_predict": max_tokens})
            if structured:
                payload["format"] = structured if isinstance(structured, dict) else "json"
        else:
            payload.update(temperature=0, max_tokens=max_tokens)
            if structured:
                payload["response_format"] = (
                    {"type": "json_object"} if google_compatible or not isinstance(structured, dict)
                    else {"type": "json_schema", "json_schema": {
                        "name": "grounded_answer", "strict": True, "schema": structured}}
                )
        if tools:
            payload["tools"] = tools
        data = self._post("/api/chat" if native else "/chat/completions", payload)
        try:
            message = data["message"] if native else data["choices"][0]["message"]
            if not isinstance(message, dict):
                raise ValueError("message")
            return message
        except (KeyError, IndexError, TypeError, ValueError) as exc:
            raise AIError("invalid_model_response", "Модель вернула некорректный ответ", 502) from exc
