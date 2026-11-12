"""Atomic corpus replacement and cosine retrieval in pgvector, public corpus only."""
import json
import psycopg
from psycopg.types.json import Jsonb
from chunking import Chunk
from errors import AIError
from providers import validate_vector


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

    def search(self, vector):
        vector = self._vector(vector)
        try:
            with self._connection() as connection:
                rows = connection.execute("""SELECT metadata FROM ai_knowledge_chunks
                    WHERE collection = %s AND metadata->>'source_type' = 'public'
                    AND (embedding <=> %s::vector) <= %s
                    ORDER BY embedding <=> %s::vector, chunk_id LIMIT %s""",
                    [self.settings.collection, vector, self.settings.rag_max_distance,
                     vector, self.settings.rag_top_k]).fetchall()
            return [Chunk.model_validate(row[0]) for row in rows]
        except (psycopg.Error, ValueError) as exc:
            raise AIError("database_error", "База знаний недоступна или требует индексации") from exc
