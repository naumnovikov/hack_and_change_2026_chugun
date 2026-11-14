import json
from pathlib import Path

import pytest
from pydantic import ValidationError

from chunking import Source, chunk_markdown, load_corpus


KNOWLEDGE = Path(__file__).resolve().parents[1] / "knowledge"


@pytest.fixture
def source():
    return Source.model_validate(json.loads((KNOWLEDGE / "sources.json").read_text("utf-8"))[0])


def test_real_corpus_has_curated_sections_and_pinned_sources():
    chunks = load_corpus(KNOWLEDGE)
    assert len(chunks) >= 15
    assert {chunk.document_id for chunk in chunks} == {
        "avito-spt", "avito-tdr", "avito-glossary",
        "avito-developer-principles", "avito-team-processes"}
    assert len({chunk.chunk_id for chunk in chunks}) == len(chunks)
    for chunk in chunks:
        assert chunk.source_revision in str(chunk.source_url)
        assert chunk.source_type == "public"
        assert chunk.content_kind == "editorial_summary"
        assert chunk.text.startswith(f"{chunk.title}\n{chunk.section}\n\n")
        assert len(chunk.text) <= 1200
        assert "Проверено:" not in chunk.text
        assert "Ручная приёмка" not in chunk.text
        assert "ZBP" not in chunk.text


@pytest.mark.parametrize("body", [" ".join(f"слово{i}" for i in range(80)), "я" * 180])
def test_long_section_is_bounded_and_preserves_body(source, body):
    chunks = chunk_markdown(f"# Термин\n\n## Определение\n{body}", source, max_chars=70)
    assert len(chunks) > 1
    actual = [chunk.text.split("\n\n", 1)[1] for chunk in chunks]
    assert "".join("".join(part.split()) for part in actual) == "".join(body.split())
    assert all(len(chunk.text) <= 70 for chunk in chunks)
    assert [chunk.position for chunk in chunks] == list(range(len(chunks)))


def test_empty_sections_and_provenance_are_not_indexed(source):
    chunks = chunk_markdown("# SPT\nПроверено: сегодня\n## Пустой\n \n## Смысл\nТекст", source)
    assert len(chunks) == 1
    assert chunks[0].text == "SPT\nСмысл\n\nТекст"
    assert chunk_markdown("# SPT\n## Пустой\n", source) == []


def test_ids_are_reproducible_and_track_content_and_source_changes(source):
    text = "# SPT\n## Смысл\nТекст"
    original = chunk_markdown(text, source)
    assert original == chunk_markdown(text.replace("\n", "\r\n"), source)
    changed = chunk_markdown(text + " изменён", source)
    assert changed[0].chunk_id != original[0].chunk_id
    assert changed[0].document_revision != original[0].document_revision
    updated = source.model_copy(update={"source_revision": "a" * 40})
    assert chunk_markdown(text, updated)[0].chunk_id != original[0].chunk_id


@pytest.mark.parametrize("text", ["", "SPT", "# \n## Раздел\nТекст", "# SPT\nНет разделов"])
def test_invalid_document_is_rejected(source, text):
    with pytest.raises(ValueError):
        chunk_markdown(text, source)


def test_impossible_size_is_rejected(source):
    with pytest.raises(ValueError, match="max_chars"):
        chunk_markdown("# SPT\n## Смысл\nТекст", source, max_chars=3)


@pytest.mark.parametrize("field,value", [
    ("filename", "../README.md"), ("source_revision", "master"),
    ("source_url", "not-a-url"), ("source_type", "private"),
])
def test_invalid_source_metadata_is_rejected(source, field, value):
    with pytest.raises(ValidationError):
        Source.model_validate({**source.model_dump(), field: value})


def test_duplicate_document_ids_are_rejected(tmp_path, source):
    (tmp_path / "sources.json").write_text(json.dumps([source.model_dump(mode="json")] * 2))
    with pytest.raises(ValueError, match="Duplicate"):
        load_corpus(tmp_path)


def test_missing_material_is_not_silently_skipped(tmp_path, source):
    (tmp_path / "sources.json").write_text(json.dumps([source.model_dump(mode="json")]))
    with pytest.raises(FileNotFoundError):
        load_corpus(tmp_path)
