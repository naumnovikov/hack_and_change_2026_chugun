"""Explicit retrieval-only evaluation; uses the configured embedding provider."""
import json
from pathlib import Path
import re
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from providers import Provider
from repository import Repository
from rag import definition_query
from settings import Settings
from synonyms import expand_query, term_matches


def evaluate(path):
    cases = json.loads(path.read_text(encoding="utf-8"))
    provider = Provider(Settings.from_env())
    repository = Repository(provider.settings)
    results = []
    for case in cases:
        question = case["question"]
        query = definition_query(question) or question
        chunks = repository.search(provider.embed([expand_query(query)])[0], query)
        if not chunks and expand_query(query) != query:
            chunks = repository.search(provider.embed([query])[0], query)
        terms = re.findall(r"\b[a-zA-Z][a-zA-Z0-9_-]{1,}\b", question)
        chunks = [x for x in chunks if all(term_matches(term, x.text) for term in terms)]
        result = {"question": question, "hits": [
            {"document": x.document_id, "section": x.section} for x in chunks]}
        if "document" in case:
            result["expected_section_found"] = any(
                x.document_id == case["document"] and x.section == case["section"] for x in chunks)
        else:
            result["expect"] = case["expect"]
            result["empty_retrieval"] = not chunks
        results.append(result)
    return {"section_hits": sum(x.get("expected_section_found", False) for x in results),
            "positive_cases": sum("document" in x for x in cases), "results": results}


if __name__ == "__main__":
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).with_name("knowledge.json")
    print(json.dumps(evaluate(path), ensure_ascii=False, indent=2))
