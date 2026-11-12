# AI-buddy — прототип для адаптации стажёров

Проект команды **Чугун** для Hack&Change 2026. Репозиторий содержит Go mock API, базовый FastAPI-сервис и Docker Compose-инфраструктуру.

> **Текущее состояние:** Frontend содержит три демонстрационных сценария и в Docker обслуживается на порту `3000`; чат вызывает Go API, а заявки и сообщения используют mock endpoints. Backend сохраняет mock-действия в Postgres и проксирует чат. AI endpoint пока возвращает фиксированный ответ, поэтому RAG/LLM и полный цикл agent tools ещё не реализованы. Последние CI jobs для Backend, Frontend и AI прошли; полное Compose end-to-end прохождение в этой инструкции не заявляется как проверенное.

## Документация

- [Требования к кейсу](./docs/REQUIREMENTS.md)
- [Hackathon FAQ](./docs/HACKATHON_FAQ.md)
- [Контекст и ограничения проекта](./docs/PROJECT_CONTEXT.md)
- [Правила команды](./docs/TEAM_RULES.md)
- [CJM](./docs/CJM.md)
- [Метрики](./docs/METRICS.md)
- [OpenAPI Backend](./specs/openapi.yaml)
- [Спецификация агента](./specs/agent.spec.md)

## Требования

- Docker Desktop с Docker Compose.
- Свободные локальные порты или их переопределение в `.env`.

## Frontend

Реализован единый адаптивный чат для трёх MVP-сценариев: вопрос по материалам, демо-заявка на доступ, поиск синтетического владельца и подтверждаемая отправка mock-сообщения. Подсказки и ответы действий отображаются в общей истории сообщений — отдельных вкладок сценариев нет. Интерфейс запускается через Compose и проксирует `/api` через Nginx к Go Backend. История чата хранится в памяти страницы.

Локальная разработка, команды проверок и границы текущих интеграций описаны в [`frontend/README.md`](./frontend/README.md).

## Запуск через Docker Compose

Из корня репозитория:

```bash
cp .env.example .env
docker compose config -q
docker compose up -d --build
docker compose ps
```

Compose описывает пять сервисов:

| Сервис | Порт на хосте по умолчанию | Примечание |
|---|---:|---|
| Backend | `8080` | Go mock API |
| Frontend | `3000` | Nginx со статической React-сборкой; `/api` проксируется к Backend |
| AI service | `8000` | FastAPI endpoint; пока возвращает фиксированный ответ |
| Ollama | `11434` | Запущенный контейнер не означает, что модель загружена |
| PostgreSQL | `5432` | Образ pgvector; расширение `vector` создаётся из `init.sql` |

В локальном окружении порты `5432` и `8080` были заняты. Для такого случая измените соответствующие значения в `.env`, например:

```dotenv
POSTGRES_PORT=5433
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
  -d '{"message":"Hello"}'
```

При `PORT=18080` замените `localhost:8080` на `localhost:18080`. Проверенный результат для Backend endpoints — HTTP 200. Ответ `POST /api/chat` сейчас проксируется в AI-сервис, который возвращает фиксированный `{"reply":"Hello from AI"}`. Это проверка транспорта, не работающая LLM или RAG.

## Проверки

```bash
(cd backend && go test -cover ./...)
docker compose run --build --rm --no-deps ai-service python -m pytest
(cd frontend && npm ci && npm run lint && npm run build && npm run test:coverage)
```

Frontend CI запускает lint, build, тесты с порогом покрытия строк выше 80% и загружает `frontend-coverage`. Backend и AI workflows загружают собственные отчёты покрытия.

## Ограничения текущего прототипа

- AI-сервис возвращает фиксированный ответ. RAG, вызовы модели и function calling ещё не подключены; чат не отвечает на основе базы знаний.
- Обработка команд для mock-заявки и поиска владельца сейчас запускается Frontend по распознанным словам и вызывает существующие mock endpoints напрямую. Это демонстрационный чатовый flow, не полноценное выполнение tools AI-агентом; целевая серверная оркестрация описана в `specs/agent.spec.md`.
- Поиск владельца и отправка сообщения используют синтетические mock endpoints. Пользователь подтверждает отправку, реальные сообщения коллегам не отправляются.
- UI не предоставляет настоящую авторизацию и не должен использоваться с реальными персональными данными.
- Ollama контейнер стартует, но наличие скачанной модели отдельно не проверено. Проверить список моделей можно командой `docker compose exec ollama ollama list`.
- Mock API возвращает синтетические данные и не выдаёт реальные доступы, не отправляет сообщения в корпоративные системы и не обращается к реальным данным сотрудников.
- Локальные порты могут отличаться от значений по умолчанию, если они уже заняты.

Остановить контейнеры проекта, сохранив данные в volumes:

```bash
docker compose down
```

Не добавляйте `-v`, если не хотите удалить volumes PostgreSQL и Ollama.
