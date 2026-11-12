"""Run python -m ingest explicitly after configuring the DB and embedding provider."""
from chunking import load_corpus
from providers import Provider
from repository import Repository
from settings import Settings
from errors import AIError


def ingest(settings=None, provider=None, repository=None):
    settings = settings or Settings.from_env()
    provider = provider or Provider(settings)
    repository = repository or Repository(settings)
    chunks = load_corpus(settings.knowledge_dir)
    if not chunks:
        raise ValueError("Knowledge corpus is empty")
    vectors = []
    for start in range(0, len(chunks), 8):
        vectors.extend(provider.embed([chunk.text for chunk in chunks[start:start + 8]]))
    repository.replace(chunks, vectors)
    return len(chunks)


def main():
    try:
        print(f"Indexed chunks: {ingest()}")
        return 0
    except AIError as exc:
        print(f"{exc.code}: {exc.message}")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
