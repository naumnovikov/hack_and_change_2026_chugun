# Frontend AI-buddy

Основа Vite + React + TypeScript для команды «Чугун».
Реализованы адаптивный экран чата, Zustand-стор сообщений и индикатор печати-заготовка.
Отправка локальная: история хранится в памяти страницы и исчезает после обновления.
Backend/AI-интеграции, API-клиента и SSE пока нет.

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

Frontend job в GitHub Actions запускает установку зависимостей, линтер, сборку и тесты.
