"""Atomic corpus replacement and cosine retrieval in pgvector, public corpus only."""
import json
import re
import psycopg
from psycopg.types.json import Jsonb
from chunking import Chunk
from errors import AIError
from providers import validate_vector
from synonyms import term_matches


class Repository:
    def __init__(self, settings, connect=psycopg.connect):
        self.settings, self.connect = settings, connect

    def _connection(self):
        if not self.settings.database_url:
            raise AIError("database_unconfigured", "Не настроено подключение к базе знаний")
        return self.connect(self.settings.database_url, connect_timeout=5,
                            options="-c statement_timeout=5000")

    def _vector(self, values):
        validate_vector(values, self.settings.embedding_dimensions)
        return json.dumps(values, allow_nan=False)

    def replace(self, chunks, vectors):
        if not chunks or len(chunks) != len(vectors):
            raise ValueError("Non-empty aligned chunks and vectors required")
        rows = [(self.settings.collection, chunk.chunk_id,
                 Jsonb(chunk.model_dump(mode="json")), self._vector(vector))
                for chunk, vector in zip(chunks, vectors)]
        try:
            with self._connection() as connection:
                connection.execute("SELECT pg_advisory_xact_lock(hashtext(%s))", ["ai-knowledge-import"])
                connection.execute("CREATE EXTENSION IF NOT EXISTS vector")
                connection.execute("""CREATE TABLE IF NOT EXISTS ai_knowledge_chunks (
                    collection text NOT NULL, chunk_id text NOT NULL, metadata jsonb NOT NULL,
                    embedding vector NOT NULL, PRIMARY KEY (collection, chunk_id))""")
                connection.execute("DELETE FROM ai_knowledge_chunks WHERE collection = %s", [self.settings.collection])
                for row in rows:
                    connection.execute("""INSERT INTO ai_knowledge_chunks
                        (collection, chunk_id, metadata, embedding) VALUES (%s, %s, %s, %s::vector)""", row)
        except psycopg.Error as exc:
            raise AIError("database_error", "Не удалось обновить базу знаний") from exc

    def search(self, vector, question=None):
        vector = self._vector(vector)
        try:
            with self._connection() as connection:
                rows = connection.execute("""SELECT metadata FROM ai_knowledge_chunks
                    WHERE collection = %s AND metadata->>'source_type' = 'public'
                    AND (embedding <=> %s::vector) <= %s
                    ORDER BY embedding <=> %s::vector, chunk_id LIMIT %s""",
                    [self.settings.collection, vector, self.settings.rag_max_distance,
                     vector, self.settings.rag_top_k]).fetchall()
            semantic = [Chunk.model_validate(row[0]) for row in rows]
            if not question:
                return semantic
            lexical = self.search_lexical(question)
            combined = {chunk.chunk_id: chunk for chunk in semantic + lexical}
            return list(combined.values())[:self.settings.rag_top_k]
        except (psycopg.Error, ValueError) as exc:
            raise AIError("database_error", "База знаний недоступна или требует индексации") from exc

    def search_lexical(self, question):
        """Small public corpus: Russian morphology plus a required section match."""
        with self._connection() as connection:
            terms = set(connection.execute(
                "SELECT tsvector_to_array(to_tsvector('russian', %s))", [question]
            ).fetchone()[0])
            rows = connection.execute("""SELECT metadata,
                tsvector_to_array(to_tsvector('russian', metadata->>'text')),
                tsvector_to_array(to_tsvector('russian',
                    concat_ws(' ', metadata->>'title', metadata->>'section')))
                FROM ai_knowledge_chunks WHERE collection=%s
                AND metadata->>'source_type'='public'""", [self.settings.collection]).fetchall()
        return rank_lexical(rows, terms, question)[:self.settings.rag_top_k]


def rank_lexical(rows, terms, question):
    latin = re.findall(r"\b[a-zA-Z][a-zA-Z0-9_-]{1,}\b", question)
    definition = re.fullmatch(r"Что такое ([a-z][a-z0-9_]{1,63})[?]", question, re.I)
    ranked = []
    for metadata, body, section in rows:
        shared, heading = terms & set(body), terms & set(section)
        acronym = bool(definition and definition.group(1).casefold() in heading)
        if not heading or not (acronym or (len(shared) >= 2 and len(shared) / max(1, len(terms)) >= .5)):
            continue
        chunk = Chunk.model_validate(metadata)
        if not all(term_matches(term, chunk.text) for term in latin):
            continue
        ranked.append((len(heading), len(shared) / max(1, len(terms)), chunk.chunk_id, chunk))
    ranked.sort(key=lambda item: (-item[0], -item[1], item[2]))
    return [item[3] for item in ranked]
