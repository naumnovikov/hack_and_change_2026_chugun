"""Validated environment configuration; never logs credentials."""
import os
from pathlib import Path
from typing import Literal
from pydantic import BaseModel, Field


class Settings(BaseModel):
    database_url: str = ""
    llm_provider: Literal["ollama", "openai", "demo"] = "ollama"
    llm_base_url: str = "http://ollama:11434/v1"
    llm_model: str = "qwen3.5:4b"
    llm_api_key: str = ""
    embedding_model: str = "embeddinggemma"
    embedding_dimensions: int = Field(default=768, ge=1, le=2000)
    timeout_seconds: float = Field(default=180, gt=0, le=300)
    rag_max_distance: float = Field(default=0.35, ge=0, le=2)
    rag_top_k: int = Field(default=3, ge=1, le=8)
    ai_internal_token: str = ""
    knowledge_dir: Path = Path(__file__).parent / "knowledge"

    @classmethod
    def from_env(cls):
        return cls.model_validate({key: os.environ[key.upper()]
                                   for key in cls.model_fields if key.upper() in os.environ})

    @property
    def collection(self) -> str:
        return f"v1:{self.llm_provider}:{self.embedding_model}:{self.embedding_dimensions}"
