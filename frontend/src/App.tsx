import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useChatStore } from './stores/chatStore'

export default function App() {
  const messages = useChatStore((state) => state.messages)
  const addMessage = useChatStore((state) => state.addMessage)
  const isTyping = useChatStore((state) => state.isTyping)
  const [draft, setDraft] = useState('')
  const conversationRef = useRef<HTMLElement>(null)

  useEffect(() => {
    const conversation = conversationRef.current
    if (conversation) conversation.scrollTop = conversation.scrollHeight
  }, [messages, isTyping])

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft.trim()) return
    addMessage(draft)
    setDraft('')
  }

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

      <section ref={conversationRef} className="conversation" role="log" aria-label="История сообщений" aria-relevant="additions text">
        {messages.length === 0 ? <div className="empty-state">
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
        </div> : <ol className="message-list">
          {messages.map((message) => (
            <li key={message.id} className={`message message-${message.role}`}>
              <span className="message-author">{message.role === 'user' ? 'Вы' : 'AI-buddy'}</span>
              <p>{message.content}</p>
            </li>
          ))}
        </ol>}
      </section>

      {isTyping && <p className="typing-status" role="status">AI-buddy печатает…</p>}

      <div className="composer-area">
        <form className="composer" aria-label="Отправка сообщения" onSubmit={handleSubmit}>
          <label className="sr-only" htmlFor="message">Сообщение AI-buddy</label>
          <textarea
            id="message"
            name="message"
            rows={2}
            placeholder="О чём хочешь спросить?"
            aria-describedby="composer-hint"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
          <button type="submit" className="send-button" disabled={!draft.trim()}>
            <span>Отправить</span>
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M5 12h14m-6-6 6 6-6 6" />
            </svg>
          </button>
        </form>
        <p id="composer-hint" className="composer-hint">
          Локальный чат · сообщения не отправляются AI-buddy и исчезнут после обновления страницы
        </p>
      </div>

      <footer className="chat-footer">
        <span>ЧУГУН<span className="footer-separator">/</span>Hack&amp;Change 2026</span>
        <span>Первый шаг к своей команде</span>
      </footer>
    </main>
  )
}
