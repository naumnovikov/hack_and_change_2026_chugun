# Покрытие тестами

Дата измерения: **16.11.2026, Europe/Moscow**.
Проверенный код: `f12aed80c668c68a8d2d232adbff03574fe1e199`.
Данные получены локально из снимка `git archive HEAD`, в Docker:
Python 3.11, Go 1.22, Node 24 и отдельная тестовая PostgreSQL/pgvector.
Все запущенные проверки прошли. Это снимок результатов, не статус последнего CI run.

## Результаты

| Сервис | Измерение | Результат тестов | Условие CI |
|---|---|---|---|
| AI | 628 / 633 исполняемых строк — **99,21%** | 218 passed, 4 skipped | ≥81% |
| AI, дополнительно с `synonyms.py` | 651 / 656 — **99,24%** | 218 passed, 4 skipped | Тот же порог |
| Backend | 714 / 837 statements — **85,3%** | 33 верхнеуровневых теста прошли, 1 skipped; включая subtests — 105 pass, 1 skip | >80% |
| Frontend | 100 / 101 lines — **99%**; 69 / 75 branches — **92%** | 44 passed, 3 файла | ≥81% lines |

Frontend также имеет 96,72% statements и 96,77% functions.
Backend `go vet`, Frontend `npm ci`, lint и production build прошли.
Три Bash startup-теста прошли; разбор PowerShell startup-скрипта дал 0 ошибок.
Процент покрытия скриптов не измерялся.

Разные инструменты считают разные единицы кода. Единого среднего процента
для проекта не приводим. Покрытие не измеряет точность модели и не доказывает
полноту проверки требований.

## Что входит в измерение

- **AI:** HTTP-контракты, провайдеры, embeddings, RAG/цитаты, контекст диалога,
  выбор инструментов, валидация пяти слайдов, ошибки/таймауты,
  корпус знаний и интеграция pgvector. Штатный список `--cov` включает десять модулей;
  `synonyms.py` измерен отдельно дополнительным аргументом, его покрытие — 100%.
  Branch coverage Python не включён.
- **Backend:** HTTP и сессии, сохранение демо-действий в PostgreSQL,
  история, выбор завершённых задач, AI-контракт, создание/скачивание PDF,
  идемпотентность, откат и конкурентное создание. Точка `main()` не покрыта,
  обработчики проверяются отдельно.
- **Frontend:** компоненты чата, API и store в jsdom с подменой HTTP.
  `App.tsx`: 98,48% lines и 88,88% branches; `api.ts` и `chatStore.ts`: 100%.
  CSS, `src/main.tsx`, тестовые файлы и реальная адаптивная раскладка не входят в метрику.

## Пропуски и ограничения

- Четыре AI-теста с реальной моделью отключены без `AI_LIVE_TESTS=1`.
  Тест с настоящей pgvector выполнен. Перед включением live-набора необходимо
  обновить старое ожидание шести векторов в `test_live_embeddings_and_grounded_generation`:
  текущий корпус содержит 27 чанков.
- Go `TestPresentationAIContract` пропущен без `TEST_PRESENTATION_AI_URL`.
  Это отдельный тест Go → настоящий FastAPI с детерминированным провайдером.
- Качество retrieval оценивается [отдельными выборками](../ai-service/evaluations/README.md).
  Прежние результаты локальной embeddinggemma нельзя считать оценкой Gemini embeddings.
- Визуальная/мобильная приёмка, нагрузка, доступность внешнего API и чистый запуск
  на другом компьютере не подтверждаются этими процентами.
- Автоматические напоминания пока не реализованы и не покрыты как готовый сценарий.

## Повторение и артефакты

Установите зависимости сервисов. Для интеграционных тестов задайте
`TEST_DATABASE_URL` на отдельную тестовую PostgreSQL; AI требует pgvector.
Не используйте рабочую БД. Запускайте команды из соответствующей папки.

```bash
# ai-service/ — параметры coverage и порог уже заданы в pyproject.toml
python -m pytest -ra
# Дополнительное измерение synonyms.py вместе со штатными модулями
python -m pytest -ra --cov=synonyms --cov-report=term-missing --cov-report=xml:coverage-all.xml --cov-report=html:htmlcov

# backend/
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
go vet ./...

# frontend/
npm ci
npm run lint
npm run build
npm run test:coverage

# Корень проекта, Linux с Bash
python -m unittest discover -s scripts/tests -v
```

В GitHub откройте **Actions → нужный запуск → Artifacts**:

| Артефакт CI | Содержимое | Конфигурация |
|---|---|---|
| `ai-coverage` | `coverage.xml` | [ai-tests.yml](../.github/workflows/ai-tests.yml) |
| `backend-coverage` | `coverage.out` | [ci.yml](../.github/workflows/ci.yml) |
| `frontend-coverage` | Cobertura XML, LCOV и HTML | [ci.yml](../.github/workflows/ci.yml) |

Локальный HTML AI находится в `ai-service/htmlcov/index.html`, Go —
`backend/coverage.html`, Frontend — `frontend/coverage/lcov-report/index.html`.
Сгенерированные файлы покрытия игнорируются Git; исходные тесты и конфигурация
хранятся в репозитории, а результаты CI публикуются артефактами.
При изменении кода таблицу в README и этот снимок следует обновить по новому прогону.
