"""Section-based chunking for the curated public Markdown corpus (format v1)."""

import hashlib
import json
from pathlib import Path
import re
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, HttpUrl


class Source(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    document_id: str = Field(pattern=r"^[a-z0-9-]+$")
    filename: str = Field(pattern=r"^[a-z0-9-]+\.md$")
    source_url: HttpUrl
    source_revision: str = Field(pattern=r"^[a-f0-9]{40}$")
    source_type: Literal["public"]
    content_kind: Literal["editorial_summary"]


class Chunk(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: Literal[1] = 1
    chunk_id: str
    document_id: str
    document_revision: str
    text: str
    title: str
    section: str
    position: int
    source_url: HttpUrl
    source_revision: str
    source_type: Literal["public"]
    content_kind: Literal["editorial_summary"]


def chunk_markdown(markdown: str, source: Source, max_chars: int = 1200) -> list[Chunk]:
    """Ignore provenance preamble; split H2 sections without losing body text."""
    markdown = markdown.replace("\r\n", "\n").strip()
    lines = markdown.splitlines()
    if not lines or not lines[0].startswith("# ") or not lines[0][2:].strip():
        raise ValueError("A non-empty H1 title is required")
    title = lines[0][2:].strip()
    # Curated format v1: preamble is provenance, every content section starts H2.
    parts = re.split(r"^##[ \t]+(.+?)[ \t]*$", markdown, flags=re.MULTILINE)
    if len(parts) == 1:
        raise ValueError("At least one H2 section is required")
    revision = hashlib.sha256(markdown.encode("utf-8")).hexdigest()
    chunks = []
    for section, body in zip(parts[1::2], parts[2::2]):
        prefix = f"{title}\n{section}\n\n"
        budget = max_chars - len(prefix)
        if budget < 1:
            raise ValueError("max_chars must leave room for section content")
        body = body.strip()
        while body:
            end = min(len(body), budget)
            if end < len(body):
                boundaries = list(re.finditer(r"\s+", body[:end + 1]))
                if boundaries and boundaries[-1].start() > 0:
                    end = boundaries[-1].start()
            text = prefix + body[:end]
            position = len(chunks)
            identity = json.dumps([source.model_dump(mode="json"), revision,
                                   max_chars, position, text], ensure_ascii=False, sort_keys=True)
            chunks.append(Chunk(
                chunk_id=hashlib.sha256(identity.encode("utf-8")).hexdigest(),
                document_id=source.document_id, document_revision=revision,
                text=text, title=title, section=section, position=position,
                source_url=source.source_url, source_revision=source.source_revision,
                source_type=source.source_type, content_kind=source.content_kind,
            ))
            body = body[end:].strip()
    return chunks


def load_corpus(directory: Path, max_chars: int = 1200) -> list[Chunk]:
    """Load only files explicitly listed in sources.json; never scan all Markdown."""
    sources = [Source.model_validate(item) for item in
               json.loads((directory / "sources.json").read_text(encoding="utf-8"))]
    if len({source.document_id for source in sources}) != len(sources):
        raise ValueError("Duplicate document_id")
    return [chunk for source in sources for chunk in chunk_markdown(
        (directory / source.filename).read_text(encoding="utf-8"), source, max_chars
    )]
