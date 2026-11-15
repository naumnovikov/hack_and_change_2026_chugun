# AI-buddy — прототип для адаптации стажёров

Проект команды **Чугун** для Hack&Change 2026: React-чат, Go API с демонстрационными действиями, Python AI/RAG и Docker Compose-инфраструктура.

> **Состояние кода на 15.11.2026:** Frontend отправляет чат в Go `/api/chat`. При заданном `AI_INTERNAL_TOKEN` Backend передаёт защищённому AI demo-контекст и предыдущий обмен с RAG-источниками. Доступны RAG, demo-заявки и создание PDF по завершённым задачам синтетической Jira. Go проверяет JSON AI и сохраняет PDF; скачать его может исходная сессия до истечения TTL. Реальные IAM и корпоративные сервисы не подключены. Legacy people-search tools ещё существуют. Наличие кода не заменяет свежую сквозную проверку окружения.
>
> **MVP команды:** запрос доступа, RAG и PDF-черновик презентации по запросу. Автоматические напоминания и реальная Jira не входят в реализованную PDF-цепочку. Внешний вид чата обновлён; остаток legacy people-search требует отдельной миграции.

## Документация

- [Требования к кейсу](./docs/REQUIREMENTS.md)
- [Hackathon FAQ](./docs/HACKATHON_FAQ.md)
- [Контекст и ограничения проекта](./docs/PROJECT_CONTEXT.md)
- [Правила команды](./docs/TEAM_RULES.md)
- [CJM](./docs/CJM.md)
- [Метрики](./docs/METRICS.md)
- [Покрытие тестами и воспроизведение проверок](./docs/TEST_COVERAGE.md)
- [OpenAPI Backend](./specs/openapi.yaml)
- [Спецификация агента](./specs/agent.spec.md)
- [Спецификация PDF-презентации по Jira-задачам](./specs/presentation.spec.md)
- [QA-чек-лист PDF-сценария](./docs/PRESENTATION_QA_CHECKLIST.md)
- [AI: запуск, настройки и тесты](./ai-service/README.md)
- [Локальное развёртывание, GPU и использование API-ключа](./ai-service/DEPLOYMENT.md)
- [Контракт AI для Backend и результаты интеграции](./ai-service/BACKEND_HANDOFF.md)
- [Внутренний AI API](./ai-service/openapi.yaml)

## Требования

- Docker Engine или Docker Desktop с Docker Compose plugin.
- Свободные локальные порты или их переопределение в `.env`.

### Установка Docker по платформам

Установите Docker один раз и проверьте, что доступны и Docker, и Compose:

```text
docker --version
docker compose version
```

- **macOS:** установите и запустите [Docker Desktop for Mac](https://docs.docker.com/desktop/setup/install/mac-install/). Выберите сборку для Apple silicon или Intel в соответствии с процессором Mac. Перед запуском Compose дождитесь, пока Docker Desktop покажет, что Engine запущен; Docker Compose входит в Docker Desktop.
- **Windows:** установите [Docker Desktop for Windows](https://docs.docker.com/desktop/setup/install/windows-install/). Для этого проекта используйте backend WSL 2; установите WSL 2, если установщик попросит. Запускайте команды Compose из PowerShell или терминала WSL в папке репозитория. Compose входит в Docker Desktop.
- **Ubuntu:** установите Docker Engine и Compose plugin по [официальной инструкции Docker для Ubuntu](https://docs.docker.com/engine/install/ubuntu/). В списке пакетов следуйте инструкции для Compose plugin (`docker-compose-plugin`). Docker Desktop на сервере не требуется. После установки проверьте `sudo docker --version` и `sudo docker compose version`; если разрешили запуск Docker без `sudo`, команды можно выполнять без него. Доступ к группе `docker` фактически даёт права root — добавляйте пользователя в неё только если это приемлемо для вашей машины.

Если команда сообщает `failed to connect to the docker API` или `Cannot connect to the Docker daemon`, значит Docker Engine не запущен или Docker CLI не может к нему подключиться. На macOS откройте Docker Desktop и дождитесь запуска Engine, затем повторите команду. Если Docker Engine/Daemon не запущен или команда `docker compose version` не работает, сначала устраните это по инструкции для своей ОС. Затем перейдите к [запуску проекта через Docker Compose](#запуск-через-docker-compose).

## Frontend

Реализован единый адаптивный чат с RAG-ответами, demo-заявками и скачиваемыми PDF-вложениями. Подсказки и ответы отображаются в общей истории; отдельных вкладок сценариев нет. Nginx проксирует `/api` к Go. Отображаемая история хранится в памяти страницы; Go отдельно сохраняет результаты и передаёт AI ограниченный контекст прошлой RAG-реплики. Это не полная долговременная память диалога.

Локальная разработка, команды проверок и границы текущих интеграций описаны в [`frontend/README.md`](./frontend/README.md).

## Запуск через Docker Compose

Скрипт запуска проверяет Docker, пытается запустить Docker Desktop (macOS/Windows) или службу Docker (Ubuntu), создаёт `.env` из `.env.example` только если файла ещё нет, запускает PostgreSQL и Ollama, при необходимости скачивает модели, индексирует базу знаний, затем собирает и запускает приложение. Он не устанавливает Docker и не перезаписывает существующий `.env`.

Из корня репозитория запустите подходящий скрипт:

**macOS и Ubuntu:**

```bash
./scripts/start.sh
```

**Windows PowerShell:**

```powershell
.\scripts\start.ps1
```

Если PowerShell блокирует запуск локального скрипта, разрешите его только для текущего окна PowerShell и повторите команду:

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\scripts\start.ps1
```

Нужны Docker Desktop (macOS/Windows) или Docker Engine и Compose plugin (Ubuntu). На Ubuntu, если служба остановлена, скрипт может запросить пароль `sudo` для её запуска; если служба уже запущена, но у пользователя нет доступа, скрипт сообщит об этом. Скрипты запускают Ollama в Compose: перед запуском остановите нативную Ollama, если она уже использует порт `11434`. При первом запуске будут скачаны Docker-образы и модели `qwen3.5:4b` и `embeddinggemma`; загрузка может занять время и несколько гигабайт диска. По завершении откройте `http://localhost:3000`.

Если `.env` создан автоматически, при необходимости включить AI-agent tools задайте в нём случайный `AI_INTERNAL_TOKEN` длиной не менее 32 символов. Сгенерируйте токен локально командой `openssl rand -hex 32`. Backend и AI должны получить одно и то же значение; оно не передаётся в браузер и не должно попадать в Git. Пустое значение оставляет детерминированный demo-chat.

Если скрипт сообщает, что Docker Engine недоступен, запустите Docker Desktop и дождитесь готовности Engine (на Ubuntu убедитесь, что у пользователя есть доступ к Docker). После этого запустите скрипт ещё раз. При проблемах с сервисами смотрите `docker compose ps -a` и `docker compose logs --tail=80`.

**Ручной запуск с уже работающей Windows Ollama:** вместо контейнера модели можно использовать нативный сервер. В PowerShell задайте адрес только для текущего сеанса и запустите остальные сервисы без зависимости `ollama`:

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

При включённом AI-agent Backend использует только демонстрационного пользователя
`demo-intern-001` и синтетические записи владельцев. Создание заявки и сообщение
не предоставляют реальные доступы и не отправляются сотрудникам.

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

### Покрытие тестами

Последний локальный прогон: **16.11.2026**, код `f12aed8`, изолированные Docker-окружения и тестовая PostgreSQL/pgvector.

| Сервис | Покрытие | Результат | Порог CI |
|---|---|---|---|
| AI / Python | **99,21%** исполняемых строк; **99,24%** с дополнительным учётом `synonyms.py` | 218 passed, 4 skipped | ≥81% |
| Backend / Go | **85,3%** statements | 33 верхнеуровневых теста прошли, 1 skipped | >80% |
| Frontend / React | **99%** lines; **92%** branches | 44 passed | ≥81% lines |

Дополнительно прошли `go vet`, Frontend lint/build и три теста Bash-скрипта запуска.
CI сохраняет артефакты `ai-coverage`, `backend-coverage`, `frontend-coverage`.
Это датированный снимок локальных измерений, а не автоматически обновляемый статус GitHub Actions.

Область измерения, пропущенные тесты и команды повторения — в [документации покрытия](./docs/TEST_COVERAGE.md).
Метрики разных инструментов не усредняются; покрытие кода не является оценкой точности ответов модели.

## Ограничения текущего прототипа

- RAG использует небольшой публичный корпус SPT/TDR: два материала, шесть чанков. Это минимальная база; определения отсутствующих терминов не выдумываются.
- При заданном `AI_INTERNAL_TOKEN` Go передаёт сообщения в Python `/agent/step`, исполняет только разрешённые предложения и хранит ответы/черновики в Postgres. Без токена остаётся прежний маршрут с детерминированными демо-командами и Python `/chat`. Реальная IAM и корпоративные сервисы не подключены.
- AI-agent использует фиксированный синтетический контекст и demo-каталог billing/search. Он не подтверждает личность, не выдаёт реальный доступ и не отправляет сообщения сотрудникам; демонстрационные заявки/сообщения не имеют реального внешнего эффекта.
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
