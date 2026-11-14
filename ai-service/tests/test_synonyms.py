import pytest

from synonyms import expand_query, term_matches


@pytest.mark.parametrize("query,expected", [
    ("Как сделать таску?", "задача"),
    ("Подготовить презу к демо", "презентация"),
    ("Кто может сделать code review моего PR?", "код-ревью"),
    ("Когда закрывать Bug?", "bug"),
    ("Когда нужен TDR?", "tech design review"),
])
def test_expand_query_includes_curated_aliases(query, expected):
    assert expected in expand_query(query)


def test_expand_query_leaves_unrecognized_words_unchanged():
    assert expand_query("Как настроить unknown-system?") == "Как настроить unknown-system?"


@pytest.mark.parametrize("term,text", [
    ("tasks", "Порядок работы с задачами"),
    ("PR", "Попросить коллегу о code review"),
    ("TDR", "Tech Design Review с коллегами"),
])
def test_lexical_guard_accepts_curated_aliases(term, text):
    assert term_matches(term, text)


def test_lexical_guard_keeps_unknown_term_exact():
    assert not term_matches("Billing", "Порядок работы с задачами")
