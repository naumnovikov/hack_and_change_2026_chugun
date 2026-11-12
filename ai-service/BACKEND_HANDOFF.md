# AI -> Backend: контракт и результаты интеграции

Проверено 13.11.2026 на Backend из GitHub main
`e018308b3052cd70b1d22ba71c33d422d58366c9` и текущем незакоммиченном AI.
Код Backend не изменялся; запуск выполнен в отдельной копии репозитория.

## Уже совместимо

Frontend -> POST /api/chat Go -> POST /chat Python.
Go требует `X-Request-ID`: 8–128 символов a-z/A-Z/0-9/_/-.
Сохранять этот ключ при повторе одного запроса; для нового сообщения — новый ключ.
Сессионную cookie demo_chat обслуживает Go. В Python заголовок/куку передавать
для публичного RAG не требуется. Тело запроса: только `message`.

Python отвечает `{reply, sources:[{title,url}]}`. Go может опускать пустой sources:
потребитель должен обрабатывать отсутствие поля как отсутствие источников.

Результаты настоящего HTTP на localhost:18081:

| Проверка | Результат |
|---|---|
| «Что такое SPT?» | 200, объяснение и pinned Playbook source, 8,92 с после восстановления |
| «Что такое ZBP?» | 200, подтверждённый источник не найден, без sources, 0,06 с |
| Недоступная модель | 502, code=ai_error; детали upstream не раскрыты, 0,03 с |

Недоступность модели проверена отдельной конфигурацией AI с неработающим
LLM_BASE_URL. После проверки рабочий адрес Ollama восстановлен.
Результаты: `C:/Хакатон/tmp/backend-ai-check-20261113-003913/http-success.json`
и `http-fault.json`. Это локальные артефакты проверки, не переносимые зависимости.

## Что подключить для настоящих AI tools

Текущий Go обрабатывает demo-команды собственным processDemo; это отдельный
рабочий механизм, не вызов Python /agent/step. Автоматически смешивать два
исполнителя действий нельзя: иначе возможны двойные операции и разные состояния.

1. Выбрать единый серверный путь orchestration в Go по agent.spec.md.
2. Настроить одинаковый сервисный AI_INTERNAL_TOKEN (32+ символа) в Go и AI.
   Отправлять его только из Go как X-AI-Internal-Token. Не отдавать браузеру.
3. Go формирует AgentRequest из своего доверенного контекста и состояния:
   request_id, conversation_id, message, user_context, tool_steps,
   allowed_services, allowed_recipients, explicit_access_request,
   confirmed_message и, при продолжении, tool_result. Полная схема —
   ai-service/openapi.yaml, семантика — specs/ai-runtime.spec.md.
4. При kind=tool_call проверить права, аргументы и подтверждение; сохранить
   call_id/аргументы и выполнить действие идемпотентно. Модель не исполняет tool.
5. После сохранения результата вернуть в Python tool_result с тем же call_id,
   name и status=success/error/unknown. Повтор не получает новый request_id/ключ.
6. Ответ kind=answer отобразить в выбранном публичном JSON/SSE-контракте Go.

Пример начала запроса (только демонстрационные значения):

```json
{
  "request_id": "demo-request-001",
  "conversation_id": "demo-conversation-001",
  "message": "Создай заявку на доступ к Jira",
  "user_context": {
    "user_id": "demo-intern-001", "role": "Intern",
    "team_id": "demo-team", "onboarding_week": 1
  },
  "tool_steps": 0,
  "allowed_services": ["Jira"],
  "explicit_access_request": true
}
```

Python вернёт предложение create_access_request, но не создаст заявку сам.
Пользовательские role/user_id и текст «я подтвердил» не являются доверенным
контекстом. Подтверждение и каталог формируются и проверяются Go.

## Оставшиеся решения Backend

- Общий timeout сейчас 30 с. Проверенный запрос уложился, но это не гарантирует
  холодный старт CPU. Сделать timeout конфигурируемым с учётом embedding+LLM.
- Сопоставить request_id/call_id с существующим X-Request-ID и состоянием Go.
- Утвердить статусы заявок, TTL подтверждения и публичный JSON/SSE-протокол.
- Реализовать trusted user context/IAM перед включением agent endpoint для UI.

Отдельный сервисный credential Python не заменяет пользовательскую авторизацию.
Ни один из этих пунктов не объявляется уже реализованным в текущем Backend.
