export default function App() {
  return (
    <main className="chat" aria-label="Чат с AI-buddy">
      <header className="chat-header">
        <div className="brand">
          <span className="avatar" aria-hidden="true">ai</span>
          <div>
            <h1>AI-buddy</h1>
            <p className="subtitle">Твой помощник в новой команде</p>
          </div>
        </div>
        <span className="demo-badge">Демо</span>
      </header>

      <section className="conversation" role="log" aria-label="История сообщений">
        <div className="empty-state">
          <div className="welcome-mark" aria-hidden="true">
            <svg viewBox="0 0 48 48" fill="none">
              <path d="M11 10h26a5 5 0 0 1 5 5v16a5 5 0 0 1-5 5H23l-11 7v-7h-1a5 5 0 0 1-5-5V15a5 5 0 0 1 5-5Z" />
              <path d="M16 23h.01M24 23h.01M32 23h.01" />
            </svg>
          </div>
          <p className="eyebrow">НОВАЯ КОМАНДА. ПОНЯТНЫЙ СТАРТ.</p>
          <h2>Осваиваться проще вместе</h2>
          <p className="welcome-text">
            Здесь можно спросить о рабочих инструментах,<br className="desktop-break" />
            доступах и людях, которые помогут с первой задачей.
          </p>
          <ul className="topics" aria-label="Темы для разговора">
            <li><span className="topic-dot blue" />Доступы к сервисам</li>
            <li><span className="topic-dot purple" />Внутренние термины</li>
            <li><span className="topic-dot green" />Поиск коллег</li>
          </ul>
        </div>
      </section>

      <div className="composer-area">
        <div className="composer">
          <label className="sr-only" htmlFor="message">Сообщение AI-buddy</label>
          <textarea
            id="message"
            name="message"
            rows={2}
            placeholder="О чём хочешь спросить?"
            aria-describedby="composer-hint"
          />
          <button type="button" className="send-button" disabled>
            <span>Отправить</span>
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M5 12h14m-6-6 6 6-6 6" />
            </svg>
          </button>
        </div>
        <p id="composer-hint" className="composer-hint">
          Демо интерфейса · отправка сообщений пока недоступна
        </p>
      </div>

      <footer className="chat-footer">
        <span>ЧУГУН<span className="footer-separator">/</span>Hack&amp;Change 2026</span>
        <span>Первый шаг к своей команде</span>
      </footer>
    </main>
  )
}
