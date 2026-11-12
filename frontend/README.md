# Frontend AI-buddy

Основа Vite + React + TypeScript для команды «Чугун».
Zustand и Zod установлены для следующих задач. Чат, стор сообщений,
API-клиент и SSE пока не реализованы.

## Локальная разработка

Нужны Node.js 20.19+ в ветке 20 либо 22.12+ и npm.
Команды выполняются из `frontend/`:

```bash
npm ci
npm run dev
```

Открыть http://localhost:5173. Это локальный режим из плана первого дня.
Существующий Docker Compose пока запускает Frontend-заглушку;
контейнерный запуск приложения ещё не настроен.

## Проверки

```bash
npm run typecheck
npm run lint
npm test
npm run build
```

`npm run test:watch` включает наблюдение за тестами.
Vitest и React Testing Library проверяют стартовый экран в jsdom.
`npm run build` проверяет типы и создаёт production-сборку в `dist/`.
`npm run preview` позволяет проверить её локально.

TypeScript настроен строго: `strict`, `noUncheckedIndexedAccess`,
`exactOptionalPropertyTypes`, проверки неиспользуемого кода.
Версии зависимостей закреплены в `package.json` и `package-lock.json`.
Новые зависимости добавляются по правилам команды.

Следующая задача по бэклогу: сверстать экран чата, затем подключить Zustand-стор.
