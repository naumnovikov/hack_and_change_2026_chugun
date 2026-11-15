from unittest.mock import Mock

from repository import Repository, rank_lexical
from settings import Settings
from test_ai import chunks, connection_mock


def test_lexical_requires_heading_and_multiple_terms(chunks):
    metadata = chunks[0].model_dump(mode="json")
    assert rank_lexical([(metadata, ["документ", "сервис"], ["документ"])],
                        {"документ", "сервис"}, "документ сервис") == chunks[:1]
    assert not rank_lexical([(metadata, ["документ", "сервис"], [])],
                            {"документ", "сервис"}, "документ сервис")
    assert not rank_lexical([(metadata, ["сервис"], ["сервис"])], {"сервис"}, "сервис")
    assert not rank_lexical([(metadata, ["документ", "сервис"], ["документ"])],
                            {"документ", "сервис", "погод", "фильм", "суп"}, "документ сервис")


def test_exact_acronym_is_allowed_but_unknown_token_is_not(chunks):
    metadata = chunks[0].model_dump(mode="json")
    rows = [(metadata, ["spt"], ["spt"])]
    assert rank_lexical(rows, {"spt"}, "Что такое SPT?") == chunks[:1]
    assert not rank_lexical(rows, {"spt", "zbp"}, "SPT ZBP")


def test_repository_hybrid_preserves_vector_order_and_public_scope(chunks):
    connection = connection_mock()
    connection.execute.return_value.fetchall.return_value = [(chunks[0].model_dump(mode="json"),)]
    repo = Repository(Settings(database_url="test", embedding_dimensions=3), Mock(return_value=connection))
    repo.search_lexical = Mock(return_value=chunks[:2])
    assert repo.search([1, 2, 3], "SPT") == chunks[:2]


def test_lexical_sql_scopes_collection_and_public_sources(chunks):
    connection = connection_mock()
    connection.execute.return_value.fetchone.return_value = (["spt"],)
    connection.execute.return_value.fetchall.return_value = [
        (chunks[0].model_dump(mode="json"), ["spt"], ["spt"])]
    repo = Repository(Settings(database_url="test"), Mock(return_value=connection))
    assert repo.search_lexical("Что такое SPT?") == chunks[:1]
    sql, parameters = connection.execute.call_args.args
    assert "source_type'='public'" in sql and parameters == [repo.settings.collection]
