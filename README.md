# AI-buddy — прототип для адаптации стажёров

Проект команды **Чугун** для Hack&Change 2026: React-чат, Go API с демонстрационными действиями, Python AI/RAG и Docker Compose-инфраструктура.

> **Текущее состояние:** Frontend отправляет все сообщения в Go `/api/chat`. Backend хранит подтверждения и повторы демо-действий в Postgres, остальные вопросы передаёт Python AI. AI отвечает по найденному RAG-контексту с источниками и умеет предлагать tools через внутренний `/agent/step`. Связка Go → AI → pgvector/Ollama проверена настоящим HTTP. Подключение Python tools к Go, полноценный IAM и SSE ещё требуется; полный трёхсценарный AI-agent E2E пока не заявляется готовым. Свежий удалённый CI для объединённой реализации ещё не запускался.

## Документация

- [Требования к кейсу](./docs/REQUIREMENTS.md)
- [Hackathon FAQ](./docs/HACKATHON_FAQ.md)
- [Контекст и ограничения проекта](./docs/PROJECT_CONTEXT.md)
- [Правила команды](./docs/TEAM_RULES.md)
- [CJM](./docs/CJM.md)
- [Метрики](./docs/METRICS.md)
- [OpenAPI Backend](./specs/openapi.yaml)
- [Спецификация агента](./specs/agent.spec.md)
- [AI: запуск, настройки и тесты](./ai-service/README.md)
- [Контракт AI для Backend и результаты интеграции](./ai-service/BACKEND_HANDOFF.md)
- [Внутренний AI API](./ai-service/openapi.yaml)

## Требования

- Docker Desktop с Docker Compose.
- Свободные локальные порты или их переопределение в `.env`.

## Frontend

Реализован единый адаптивный чат для трёх MVP-сценариев: вопрос по материалам, демо-заявка на доступ, поиск синтетического владельца и подтверждаемая отправка mock-сообщения. Подсказки и ответы действий отображаются в общей истории сообщений — отдельных вкладок сценариев нет. Интерфейс запускается через Compose и проксирует `/api` через Nginx к Go Backend. История чата хранится в памяти страницы.

Локальная разработка, команды проверок и границы текущих интеграций описаны в [`frontend/README.md`](./frontend/README.md).

## Запуск через Docker Compose

Из корня репозитория:

Если `.env` ещё нет, создайте его из `.env.example`. Существующий файл не перезаписывайте. Локальный `.env` не коммитится.

Перед запуском выберите сервер Ollama. Два режима используют разные хранилища моделей; повторно скачивать уже имеющиеся веса без причины не нужно.

**Ollama в Compose:** остановите нативный сервер на порту `11434`, если он запущен. Выполните:

```bash
docker compose config -q
docker compose up -d postgres ollama
docker compose exec ollama ollama list
```

В выбранном сервере нужны `qwen3.5:4b` и `embeddinggemma`. Если модели отсутствуют именно в этом хранилище, подготовьте их до индексации. Затем:

```bash
docker compose run --build --rm --no-deps ai-service python -m ingest
docker compose up -d --build
docker compose ps
```

**Уже работающая Windows Ollama:** вместо отдельного контейнера модели можно использовать проверенный нативный сервер. В PowerShell задайте адрес только для текущего сеанса и запустите остальные сервисы без зависимости `ollama`:

```powershell
$env:LLM_BASE_URL = 'http://host.docker.internal:11434/v1'
# Если стандартные порты заняты, выберите свободные, например:
$env:POSTGRES_PORT = '55432'
$env:PORT = '18080'
docker compose config -q
docker compose up -d postgres
docker compose run --build --rm --no-deps ai-service python -m ingest
docker compose up -d --build --no-deps backend ai-service frontend
```

Не закрывайте Ollama при использовании нативного сервера. В локальной проверке из-за ошибки CUDA она запущена на CPU. Подробности AI и переменных окружения — в [AI README](./ai-service/README.md).

Compose описывает пять сервисов:

| Сервис | Порт на хосте по умолчанию | Примечание |
|---|---:|---|
| Backend | `8080` | Go чат-прокси и сохраняемые mock-действия |
| Frontend | `3000` | Nginx со статической React-сборкой; `/api` проксируется к Backend |
| AI service | `8000` | RAG-ответы и внутренний API предложений tools |
| Ollama | `11434` | Запущенный контейнер не означает, что модель загружена |
| PostgreSQL | `5432` | Образ pgvector; расширение `vector` создаётся из `init.sql` |

В локальном окружении порты `5432` и `8080` были заняты. Для такого случая измените соответствующие значения в `.env`, например:

```dotenv
POSTGRES_PORT=55432
PORT=18080
```

В Compose `PORT` задаёт опубликованный порт хоста; Backend внутри контейнера слушает `8080`. Frontend использует same-origin `/api` proxy, поэтому `VITE_API_URL` не требуется. Для локального Vite dev-сервера при нестандартном порте Backend можно задать `BACKEND_PROXY_URL`, например `BACKEND_PROXY_URL=http://localhost:18080 npm run dev`. Не коммитьте `.env`.

Проверить состояние и логи:

```bash
docker compose ps -a
docker compose logs --tail=80 postgres backend ai-service ollama frontend
docker compose exec postgres pg_isready -U avito -d aibuddy
docker compose exec postgres psql -U avito -d aibuddy \
  -c 'SELECT extname FROM pg_extension WHERE extname = '\''vector'\'';'
```

Успешная проверка БД: `pg_isready` сообщает, что соединения принимаются, а SQL возвращает строку `vector`.

## Smoke-тесты API

Если используются порты по умолчанию:

```bash
curl -i http://localhost:8080/health
curl -i "http://localhost:8080/api/mock/employee?service_name=billing"
curl -i -X POST http://localhost:8080/api/mock/access-request \
  -H 'Content-Type: application/json' \
  -d '{"service_name":"Jira"}'
curl -i -X POST http://localhost:8080/api/mock/message \
  -H 'Content-Type: application/json' \
  -d '{"recipient_id":"emp-001","text":"Тестовый запрос на ревью"}'
curl -i -X POST http://localhost:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"Что такое SPT?"}'
curl -i -X POST http://localhost:8080/api/chat \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: smoke-spt-001' \
  -d '{"message":"Что такое SPT?"}'
```

При `PORT=18080` замените `localhost:8080` на `localhost:18080`. Для Go `/api/chat` обязателен `X-Request-ID`: новый ключ для нового сообщения, тот же ключ и текст для повтора. Python `/chat` принимает только `message`. После индексации вопрос о SPT возвращает объяснение с Playbook-ссылкой; вопрос о ZBP — честный отказ без источников. Go может опускать пустое поле `sources`.

Backend использует `AI_REQUEST_TIMEOUT=180s` по умолчанию; он настраивается от `1s` до `10m`. Nginx ждёт до `610s`. AI ограничивает каждый вызов модели через `TIMEOUT_SECONDS=180`, поэтому общий timeout Go подбирается с учётом embeddings и генерации, особенно при холодном запуске CPU.

## Проверки

```bash
(cd backend && go test -cover ./...)
docker compose run --build --rm --no-deps ai-service python -m pytest
(cd frontend && npm ci && npm run lint && npm run build && npm run test:coverage)
```

Frontend CI запускает lint, build, тесты с порогом покрытия строк выше 80% и загружает `frontend-coverage`. Backend и AI workflows загружают собственные отчёты покрытия.

AI проверен в Docker/Python 3.11 с настоящим pgvector: 88 тестов прошли, покрытие 99,78%. Четыре проверки настоящих моделей выполнены отдельно. Тестовая БД включается через `TEST_DATABASE_URL`; обычные unit-тесты не скачивают и не запускают модель. HTTP-проверка Backend → AI подтвердила источник, отсутствие контекста и безопасную ошибку при недоступной модели.

После локального объединения 13.11.2026 также прошли тесты Backend (90,9% покрытия), а для Frontend — lint, production-сборка и 21 тест (97,01% покрытия строк). Повторный HTTP-прогон через объединённый Backend подтвердил SPT с источником и отказ для неизвестного термина. Это локальные результаты, не статус нового GitHub Actions run.

## Ограничения текущего прототипа

- RAG использует небольшой публичный корпус SPT/TDR: два материала, шесть чанков. Это минимальная база; определения отсутствующих терминов не выдумываются.
- Frontend передаёт все сообщения в Go. Backend обрабатывает явные демо-команды, подтверждение и отмену, хранит состояние и результаты повторов в Postgres. Это отдельный demo-flow; Python `/agent/step` пока не подключён к его исполнению. Целевая оркестрация описана в `specs/agent.spec.md` и `specs/ai-runtime.spec.md`.
- Поиск владельца и отправка сообщения используют синтетические mock endpoints. Пользователь подтверждает отправку, реальные сообщения коллегам не отправляются.
- UI не предоставляет настоящую авторизацию и не должен использоваться с реальными персональными данными.
- Наличие моделей проверяйте в выбранном сервере Ollama, не только по статусу контейнера. Обработаны ошибки модели и БД; автоматического перехода на выдуманный ответ нет.
- Mock API возвращает синтетические данные и не выдаёт реальные доступы, не отправляет сообщения в корпоративные системы и не обращается к реальным данным сотрудников.
- Локальные порты могут отличаться от значений по умолчанию, если они уже заняты.

Остановить контейнеры проекта, сохранив данные в volumes:

```bash
docker compose down
```

Не добавляйте `-v`, если не хотите удалить volumes PostgreSQL и Ollama.
