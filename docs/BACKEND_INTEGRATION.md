# Backend: контракт чата и хранение моков

Статус: предложение для ревью Frontend/AI, не подтверждённое согласование команды.

## Транспорт чата на текущем этапе

Frontend -> `POST /api/chat` Go -> `POST /chat` Python.
Запрос: `{"message":"Что такое SPT?"}`. Только message, без user_id/role;
пробелы по краям удаляются, длина после удаления 1–8000 Unicode-символов.
Ответ: `{"reply":"...","sources":[{"title":"Demo Playbook","url":"https://example.org/demo"}]}`.
`sources` необязателен; при отсутствии источников ссылки не придумываются.
Текущий Python возвращает только reply, что совместимо с Go.
Формат источников — предложение для следующей итерации AI; существующий Python пока их не производит.

Ошибки Go: `{"code":"ai_timeout","error":"AI-сервис не ответил вовремя"}`.
- 400 invalid_request — некорректный JSON, лишние поля, пустое/длинное сообщение.
- 503 ai_unavailable — адрес AI не настроен либо сервис недоступен.
- 504 ai_timeout — таймаут AI (AI_REQUEST_TIMEOUT, по умолчанию 180 секунд).
- 502 ai_error — AI вернул не 200; детали upstream не раскрываются.
- 502 ai_invalid_response — невалидный JSON, пустой reply, неверный источник или ответ >1 MiB.

Адрес читается из AI_SERVICE_URL. При сбое записи или чата автоматических повторов нет.
Это JSON smoke/integration transport. SSE, история диалога, trusted user context,
tool calls и серверное подтверждение отправки из agent.spec.md пока не реализованы.
Этот документ не объявляет полную готовность трёх CJM.

## Сверка существующих моков с OpenAPI

Совпадают пути, методы и поля успешных ответов:
- GET /health -> status.
- POST /api/mock/access-request: service_name -> ticket_id, status, message.
- POST /api/mock/message: recipient_id, text -> message_id, sent_at.
- GET /api/mock/employee?service_name=... и GET /api/mock/employee/{id}:
  employee_id, name, role, mattermost_id.

Моки — синтетические HTTP-адаптеры, не endpoints выполнения действий агентом.
В agent.spec.md уже закреплено: Python отвечает за RAG/LLM, Go — за права/tools/сохранение.
Это уточнение приоритетнее устаревшей схемы в PROJECT_CONTEXT.md.

## Postgres

Подготовлены таблицы mock_access_requests и mock_messages, параметризованные INSERT
с RETURNING, уникальные последовательные DEMO-ID и время отправки из Postgres.
CREATE TABLE IF NOT EXISTS подходит для уже существующего volume; удалять volume не нужно.
Хранилище подключено в main через lib/pq v1.10.9; DATABASE_URL обязателен. При старте проверяется соединение и создаются таблицы. При сбое БД сервис останавливается без перехода на несохраняемые ответы.

## Что согласовать на Sync

1. Принять ли JSON POST /api/chat как промежуточный контракт до SSE.
2. Принять ли sources: [{title,url}] для будущих RAG-ответов.
3. Проверить Docker на машине с Docker Desktop; драйвер lib/pq согласован.

## Воспроизводимая проверка

- `cd backend && go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`
- `go vet ./...`
- Реальный Postgres: указать TEST_DATABASE_URL отдельной тестовой БД и запустить
  `go test -run TestPostgresIntegration -v`. Тест создаёт только синтетические записи,
  сверяет их независимым SELECT и удаляет свои записи. Без env тест явно пропускается.
- Compose: скопировать .env.example в .env, выполнить
  `docker compose up --build postgres backend ai-service`.
- POST localhost:8080/api/chat с JSON `{"message":"Привет"}` должен вернуть reply от AI.
- POST /api/mock/access-request и /api/mock/message должны сохранять строки;
  проверить их после перезапуска backend.

Проверено 12.11.2026: unit-тесты и go vet проходят, покрытие 89,7%. В Docker прошёл TestPostgresIntegration на отдельной БД. Smoke-проверка: /health, /api/chat, создание заявки и сообщения; записи подтверждены SQL после перезапуска Backend. На Windows HTTP health/chat проверены на порту 18080 (8080 занят Apache). AI запущен с --no-deps без Ollama, так как текущая заглушка не вызывает LLM. Полный запуск всех сервисов и RAG не проверены.
Для мок-адаптеров нет trusted user context, IAM, подтверждения отправки и идемпотентности.
Они не должны публиковаться как готовые безопасные tools агента.

## Таймаут локальной модели

Backend читает AI_REQUEST_TIMEOUT из окружения: по умолчанию 180s, допустимо от
1s до 10m (например, 5m). Некорректное значение даёт предупреждение и fallback 180s;
нулевой timeout, отключающий ограничение HTTP-клиента, не допускается.
Compose передаёт переменную из .env. После изменения пересоздайте Backend:
`docker compose up -d --no-deps backend`.
Nginx Frontend ждёт upstream до 610s, чтобы не обрывать запрос раньше Go.
Это время ожидания, а не обещание скорости модели. По истечении Go возвращает
504 ai_timeout; UI продолжает показывать ожидание и допускает безопасный повтор.
