# AI-buddy — прототип для адаптации стажёров

Проект команды **Чугун** для Hack&Change 2026. Репозиторий содержит Go mock API, базовый FastAPI-сервис и Docker Compose-инфраструктуру.

> **Текущее состояние:** инфраструктура и Backend mock API проверены в Docker. AI endpoint пока возвращает фиксированный ответ. RAG/LLM-вызов и пользовательский Frontend ещё не реализованы.

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
| Frontend | `3000` | Пока заглушка, веб-интерфейса нет |
| AI service | `8000` | FastAPI endpoint-заглушка |
| Ollama | `11434` | Запущенный контейнер не означает, что модель загружена |
| PostgreSQL | `5432` | Образ pgvector; расширение `vector` создаётся из `init.sql` |

В локальном окружении порты `5432` и `8080` были заняты. Для такого случая измените соответствующие значения в `.env`, например:

```dotenv
POSTGRES_PORT=5433
PORT=18080
VITE_API_URL=http://localhost:18080
```

В Compose `PORT` задаёт опубликованный порт хоста; Backend внутри контейнера слушает `8080`. `VITE_API_URL` здесь приведён для будущего Frontend. Не коммитьте `.env`; коммитьте только обновлённый `.env.example`, если меняется общий набор настроек.

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

При `PORT=18080` замените `localhost:8080` на `localhost:18080`. Проверенный результат для Backend endpoints — HTTP 200. Ответ `POST /chat` сейчас фиксированный (`{"reply":"Hello from AI"}`): это проверка транспорта, не работающая LLM или RAG.

## Проверки

```bash
(cd backend && go test -cover ./...)
docker compose run --build --rm --no-deps ai-service python -m pytest
```

Backend CI загружает отчёт покрытия как artifact; последний проверенный показатель Backend — 91,5%. AI CI запускает тесты и сборку Docker-образа. Frontend job пока нет, так как Frontend scaffold и тесты ещё не добавлены.

## Ограничения текущего прототипа

- Frontend-контейнер запускает `tail -f /dev/null`; пользовательского интерфейса и страницы на порту `3000` пока нет.
- AI-сервис возвращает фиксированный ответ. RAG, вызовы модели и function calling ещё не подключены.
- Ollama контейнер стартует, но наличие скачанной модели отдельно не проверено. Проверить список моделей можно командой `docker compose exec ollama ollama list`.
- Mock API возвращает синтетические данные и не выдаёт реальные доступы, не отправляет сообщения в корпоративные системы и не обращается к реальным данным сотрудников.
- Локальные порты могут отличаться от значений по умолчанию, если они уже заняты.

Остановить контейнеры проекта, сохранив данные в volumes:

```bash
docker compose down
```

Не добавляйте `-v`, если не хотите удалить volumes PostgreSQL и Ollama.
