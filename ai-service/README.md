# AI-buddy: Python AI service

Python отвечает за RAG, генерацию и предложения tool calls. Go отвечает за IAM,
историю, подтверждения, исполнение tools, идемпотентность, хранение результатов
и публичный SSE. Python не обращается к mock endpoints напрямую.

## Реализовано

- Чанки по разделам, pinned sources, публичный корпус SPT/TDR.
- Реальные embeddings Ollama embeddinggemma и cosine retrieval в pgvector.
- Атомарная повторяемая индексация с разделением коллекций по embedding-модели.
- POST /chat: {message} -> {reply, sources:[{title,url}]}; без источников — честный отказ.
- Генерация JSON с проверяемыми цитатами; URL берётся из metadata.
- POST /agent/step: три инструмента, проверка аргументов и разрешённого каталога,
  точное подтверждение сообщения, лимит шагов, обработка результатов без новой LLM.
- Ошибки модели/БД, ограниченные таймауты, отсутствие скрытых retry.
- Ollama и OpenAI-compatible adapters; явный demo без настоящей модели.

Формат: ../specs/ai-runtime.spec.md. Машиночитаемый контракт: openapi.yaml.
JSON внутри .yaml валиден как YAML. Перегенерация из этой папки:

```powershell
python -c "import json,pathlib; from main import app; pathlib.Path('openapi.yaml').write_text(json.dumps(app.openapi(),ensure_ascii=False,indent=2)+'\n',encoding='utf-8')"
```

## Docker: загрузка и запуск

Из корня репозитория, с существующим .env (не коммитить):

```powershell
docker compose up -d postgres ollama
docker compose exec ollama ollama list
# Отдельная embedding-модель, загрузить один раз:
docker compose exec ollama ollama pull embeddinggemma
# Убедиться, что qwen3.5:4b уже есть именно в этой Ollama.
docker compose run --build --rm --no-deps ai-service python -m ingest
docker compose up -d --build ai-service
```

Нативная и контейнерная Ollama имеют разные хранилища. Проверенные Windows-модели
лежат в .ollama/models, не в Docker volume. Выберите один сервер и задайте
доступный AI-сервису LLM_BASE_URL; localhost в контейнере — сам контейнер.
Qwen повторно не скачивать без причины. Не удалять volumes для переиндексации.

POST http://localhost:8000/chat с JSON {"message":"Что такое SPT?"} должен
вернуть ответ и ссылку на pinned Playbook. Вопрос о ZBP — отказ без источников.
GET /health — liveness, не проверка готовности БД и модели.

## Настройки

Параметры читаются из env; код не читает локальный .env самостоятельно.
Compose передаёт настройки из корневого .env. Секреты не выводятся.

| Переменная | По умолчанию / назначение |
|---|---|
| DATABASE_URL | обязательно: Postgres с расширением vector |
| LLM_PROVIDER | ollama; также openai или явно помеченный demo |
| LLM_BASE_URL | http://ollama:11434/v1 |
| LLM_MODEL | qwen3.5:4b |
| LLM_API_KEY | пусто; для облачного совместимого провайдера задать ключ |
| EMBEDDING_MODEL | embeddinggemma; у облачного провайдера — его embedding-модель |
| EMBEDDING_DIMENSIONS | 768; должна совпадать с фактической размерностью |
| TIMEOUT_SECONDS | 180 на внешний вызов, с запасом на холодный CPU |
| RAG_MAX_DISTANCE | 0.35, cosine distance; калибровать по контрольным вопросам |
| RAG_TOP_K | 3 |
| AI_INTERNAL_TOKEN | 32+ символа; иначе /agent/step отключён |
| KNOWLEDGE_DIR | knowledge рядом с кодом |

Cloud: LLM_PROVIDER=openai, LLM_BASE_URL=https://<provider>/v1, ключ,
LLM_MODEL, EMBEDDING_MODEL провайдера, правильная размерность; затем ingest.
Fallback без ключа: LLM_PROVIDER=demo и повторный ingest. Demo использует
hash-embeddings и выдаёт помеченный фрагмент, не нейросетевой ответ.
Postgres нужен в обоих режимах. Сбой production не включает demo автоматически.

## Тесты

```powershell
docker compose run --build --rm --no-deps ai-service python -m pytest
```

Unit-тесты не требуют сети или модели. Все прикладные модули включены в coverage,
порог 81%. coverage.xml не коммитится. Для настоящего Postgres задать
TEST_DATABASE_URL (тестовая БД). CI содержит pgvector service и этот тест.
Проверяются cosine retrieval, замена без дублей и rollback транзакции.
AI_LIVE_TESTS=1 включает настоящий RAG и три tool calls; модели автоматически
не скачиваются. Это не полный Go/Frontend E2E.

Локальная отладка, из ai-service:

```powershell
$env:LLM_BASE_URL = 'http://127.0.0.1:11434/v1'
# DATABASE_URL задаётся для вашего тестового Postgres, без публикации пароля.
.\.venv\Scripts\python.exe -m ingest
.\.venv\Scripts\python.exe -m uvicorn main:app --host 127.0.0.1 --port 8000
```

## Зависимости от Backend / границы готовности

Go chat-прокси подготовлен в ветке Backend. /chat совместим с предложенным JSON.
Для tools Go подключает /agent/step, передаёт доверенный контекст и credential,
хранит request_id/call_id/счётчик шагов и подтверждения, проверяет права,
безопасно выполняет/восстанавливает tools и сохраняет результаты. Контракт:
../specs/ai-runtime.spec.md. Общий timeout Go должен учитывать время embeddings,
генерации и cold start; 30 секунд могут быть недостаточны.

Без этих частей нельзя объявлять три CJM готовыми end-to-end. Python индексирует
только публичные источники; закрытый командный RAG не включён. Проверка цитат
не заменяет смысловую оценку ответа на контрольных вопросах.

## Результат проверки 13.11.2026

AI-образ собран и проверен в Docker/Linux/Python 3.11.18: 88 тестов прошли,
покрытие 99,78%, включая настоящий pgvector в отдельной тестовой БД.
Четыре проверки настоящей модели прошли отдельным прогоном: RAG с источником
и предложения всех трёх tools. Модельные тесты не входят в обязательный CI-прогон.

Локальный Postgres с vector 0.8.7 работает на порту 55432; загружено шесть чанков.
Связка свежего Go Backend -> AI -> pgvector/Ollama проверена настоящим HTTP:
SPT с источником, неизвестный ZBP без источников и недоступная модель.
Подробности и контракт для Backend: BACKEND_HANDOFF.md.
Из-за ошибки CUDA локальная Ollama запущена на CPU; исходники для этого не менялись.
Свежий удалённый Actions run для новой AI-реализации ещё не запускался.
