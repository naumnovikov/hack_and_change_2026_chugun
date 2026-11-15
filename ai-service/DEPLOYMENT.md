# Локальное развёртывание и провайдер с API-ключом

Обновлено 15.11.2026. Команды запускаются из корня репозитория.
Python обслуживает RAG и JSON-черновики, Go проверяет действия и создаёт PDF.
Браузер обращается только к Go через same-origin `/api`.

## Два разных секрета

| Переменная | Назначение |
|---|---|
| `LLM_API_KEY` | Ключ внешнего провайдера модели; передаётся Python → провайдер как Bearer credential |
| `AI_INTERNAL_TOKEN` | Случайный сервисный токен Go → Python, минимум 32 символа; нужен agent flow и презентациям |

Это разные значения. Для локальной Ollama `LLM_API_KEY` обычно пустой,
но `AI_INTERNAL_TOKEN` всё равно нужен. Ни один секрет не передаётся Frontend.
Не публикуйте `.env`, скриншоты ключей или вывод полного `docker compose config`.
Для проверки синтаксиса используйте `docker compose config -q`.

## Вариант 1: локальные модели в Docker

1. Установите и запустите Docker Desktop. CPU поддерживается; GPU не обязателен.
2. Если `.env` отсутствует, скопируйте `.env.example` в `.env`; существующий
   файл не перезаписывайте. Сам AI-сервис `.env` не читает: переменные передаёт Compose.
3. В `.env` задайте собственный пароль PostgreSQL и случайный AI_INTERNAL_TOKEN.
   Например, сгенерируйте токен PowerShell и вставьте результат в файл локально:

```powershell
[Convert]::ToBase64String([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
```

4. Для контейнерной Ollama оставьте настройки:

```dotenv
LLM_PROVIDER=ollama
LLM_BASE_URL=http://ollama:11434/v1
LLM_MODEL=qwen3.5:4b
LLM_API_KEY=
EMBEDDING_MODEL=embeddinggemma
EMBEDDING_DIMENSIONS=768
```

5. Поднимите зависимости и проверьте модели. Скачивайте только отсутствующие
   именно в выбранной Ollama модели:

```powershell
docker compose config -q
docker compose up -d postgres ollama
docker compose exec ollama ollama list
docker compose exec ollama ollama pull qwen3.5:4b
docker compose exec ollama ollama pull embeddinggemma
docker compose run --build --rm --no-deps ai-service python -m ingest
docker compose up -d --build
docker compose ps
```

`ingest` создаёт embeddings актуального корпуса и атомарно заменяет коллекцию,
не накапливая дубликаты. После добавления материалов повторите ingest.
Volume `pgdata` хранит БД, `ollamadata` — модели; `down -v` для обновления не нужен.

Откройте `http://localhost:3000`. Backend по умолчанию опубликован на 8080;
при изменённом PORT используйте фактический порт из `docker compose ps`.
AI опубликован на 8000. `/health` подтверждает только запуск процесса.

Нативная Ollama в Windows имеет отдельное хранилище. Если используете её,
AI-контейнеру нужен `http://host.docker.internal:11434/v1`, а не localhost.
Не запускайте второй сервер на уже занятом 11434.

## Локальный NVIDIA GPU — необязательный override

Основной Compose остаётся пригодным для CPU. Для доступной из Docker NVIDIA
создайте рядом локальный файл `compose.gpu.local.yaml`:

```yaml
services:
  ollama:
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

```powershell
docker compose -f docker-compose.yml -f compose.gpu.local.yaml up -d --no-deps ollama
docker compose exec ollama ollama ps
```

Проверяйте `ollama ps` во время запроса: возможна смешанная загрузка CPU/GPU.
GPU и прогретая модель не гарантируют фиксированное время ответа.
Локальный override с настройкой конкретного компьютера не включайте в общий PR.

## Вариант 2: внешний OpenAI-compatible API

Это режим адаптера совместимого API, а не требование использовать конкретного
провайдера. Проверьте в документации своего провайдера адрес, имена моделей,
размерность embeddings и поддержку structured JSON/tools. В текущей конфигурации
генерация и embeddings используют один base URL и один ключ: только chat API
без embeddings недостаточно для RAG.

Задайте в локальном `.env`, подставив реальные значения вместо примеров:

```dotenv
LLM_PROVIDER=openai
LLM_BASE_URL=https://YOUR_PROVIDER/v1
LLM_API_KEY=YOUR_SECRET_KEY
LLM_MODEL=YOUR_CHAT_MODEL
EMBEDDING_MODEL=YOUR_EMBEDDING_MODEL
EMBEDDING_DIMENSIONS=768
```

`768` здесь только пример: укажите реальную размерность embedding-модели
в поддерживаемом проектом диапазоне 1–2000. Секретный ключ вводите локально,
не коммитьте и не включайте в issue/отчёт. Установите лимиты расходов в аккаунте
провайдера: реальные запросы и индексация могут тарифицироваться.

```powershell
docker compose up -d postgres
docker compose run --build --rm --no-deps ai-service python -m ingest
docker compose up -d --build --no-deps backend ai-service frontend
```

Ollama для этого режима не требуется. `AI_INTERNAL_TOKEN` остаётся обязательным
для защищённых endpoint. При смене провайдера, embedding-модели или размерности
нужен ingest: векторные пространства разных моделей несовместимы. При смене
только chat-модели без изменения корпуса/embedding-модели переиндексация не нужна.
Не увеличивайте RAG_MAX_DISTANCE вслепую: оцените поиск на проверочных вопросах.

Адаптер принимает OpenAI-compatible embeddings/chat, JSON schema и tools.
Для Google-compatible hostname предусмотрен режим json_object вместо json_schema;
это не отменяет последующую Pydantic-проверку ответа. Совместимость и успешная
генерация у конкретного внешнего провайдера требуют отдельного smoke test.
В этой задаче платные запросы с настоящим API-ключом не выполнялись.

## Контекст диалога и презентации

Go передаёт только предыдущий сохранённый обмен той же сессии с валидными
RAG-источниками. Python принимает до четырёх реплик и использует до двух пар,
если Go их передал. История — подсказка для понимания темы, не источник фактов
и не разрешение выполнить действие. Уточнения «а какие типы?», «объясни проще»,
«приведи пример» используют тему истории. Новый явный латинский термин
не наследует старую тему при поиске. Без темы «объясни проще» вызывает уточнение.

Просьба «собери презу по моим таскам» в Python объясняет поддерживаемый сценарий
и предлагает точную команду `Создай презентацию`. Сама подсказка PDF не создаёт.
Запуск PDF и выбор задач принадлежат Go; расширение его команд согласуется
с Backend отдельно. Произвольные достижения и тема презентации не выдумываются.
Контекст после PDF сейчас не гарантируется: Go передаёт предыдущий обмен
только при наличии валидных RAG-источников.

## Проверка после запуска

В одной браузерной сессии:

1. `Что такое SPT?` — ответ с источником.
2. `А какие типы?` — ответ по предыдущей теме либо честный отказ при отсутствии
   подходящих фрагментов; не поиск владельца и не создание заявки.
3. `А что такое TDR?` — новая тема, не продолжение SPT.
4. В новой сессии `объясни проще` — просьба назвать тему.
5. `Можешь собрать презу?` — инструкция с точной командой, без ложного успеха.
6. `Создай презентацию` — JSON-черновик AI, PDF Backend, кнопка Frontend.
7. Неизвестный термин — без выдуманного определения.

Тесты без платных API-вызовов:

```powershell
docker compose run --build --rm --no-deps ai-service python -m pytest
```

Ошибки: 403 — сервисный токен; 422 — вход; 502 — неверный ответ/источники;
503 — настройка, сеть или отказ провайдера; 504 — timeout. Публичный Go API
может переименовывать коды. Повтор запроса на создание PDF должен сохранять
исходный X-Request-ID; новый ID означает новую операцию.
