"""Curated query aliases for common Russian/English onboarding terminology."""
import re
import unicodedata


ALIAS_GROUPS = (
    ("задача", ("задача", "задачи", "задачу", "задачей", "задаче", "задачами", "задачах", "задач",
                "таска", "таски", "таску", "таской", "таске", "тасками", "тасках", "тасок",
                "task", "tasks", "issue", "ticket")),
    ("функция", ("фича", "фичи", "фичу", "фичей", "feature", "features", "functionality",
                 "новая функция", "новая возможность")),
    ("презентация", ("презентация", "презентации", "презентацию", "презой", "преза", "презы",
                     "презу", "presentation", "presentations", "slides", "slide deck")),
    ("код-ревью", ("код-ревью", "код ревью", "ревью кода", "code review", "pr",
                   "pull request", "pull-request", "пул реквест", "пул-реквест")),
    ("bug", ("bug", "баг", "ошибка")),
    ("crash", ("crash", "краш", "аварийное завершение")),
    ("spt", ("spt", "support", "обращения пользователей")),
    ("tdr", ("tdr", "tech design review", "дизайн-ревью", "техническое дизайн-ревью")),
)


def _normalize(text: str) -> str:
    text = unicodedata.normalize("NFKC", text).casefold().replace("ё", "е")
    return re.sub(r"[^a-z0-9а-я]+", " ", text).strip()


def _contains_alias(text: str, alias: str) -> bool:
    normalized_text = f" {_normalize(text)} "
    normalized_alias = _normalize(alias)
    return bool(normalized_alias) and f" {normalized_alias} " in normalized_text


def expand_query(question: str) -> str:
    """Append only curated variants of aliases present in the user's question."""
    variants = []
    for _, aliases in ALIAS_GROUPS:
        if any(_contains_alias(question, alias) for alias in aliases):
            variants.extend(aliases)
    if not variants:
        return question
    unique = list(dict.fromkeys(variants))
    return f"{question}\nВарианты терминов: {', '.join(unique)}"


def term_matches(term: str, text: str) -> bool:
    """Keep lexical safeguards while allowing known acronyms and synonyms."""
    if _contains_alias(text, term):
        return True
    return any(term.casefold() in aliases and any(_contains_alias(text, alias) for alias in aliases)
               for _, aliases in ALIAS_GROUPS)
