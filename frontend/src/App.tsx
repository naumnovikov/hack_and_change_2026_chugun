import { useEffect, useRef, useState, type FormEvent } from 'react'
import { sendChatMessage } from './api'
import { useChatStore } from './stores/chatStore'

type ChatRequest = { message: string; requestId: string }

export default function App() {
  const messages = useChatStore((state) => state.messages)
  const addMessage = useChatStore((state) => state.addMessage)
  const isTyping = useChatStore((state) => state.isTyping)
  const setIsTyping = useChatStore((state) => state.setIsTyping)
  const [draft, setDraft] = useState('')
  const [actionError, setActionError] = useState<string | null>(null)
  const [retryRequest, setRetryRequest] = useState<ChatRequest | null>(null)
  const conversationRef = useRef<HTMLElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const inFlight = useRef(false)

  useEffect(() => {
    const conversation = conversationRef.current
    if (conversation) conversation.scrollTop = conversation.scrollHeight
  }, [messages, isTyping, actionError])

  useEffect(() => {
    if (!isTyping && !retryRequest) inputRef.current?.focus()
  }, [isTyping, retryRequest])

  async function requestChat(request: ChatRequest) {
    if (inFlight.current) return
    inFlight.current = true
    setRetryRequest(request)
    setActionError(null)
    setIsTyping(true)
    try {
      const response = await sendChatMessage(request.message, request.requestId)
      addMessage(response.reply, 'assistant', response.sources)
      setRetryRequest(null)
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Не удалось получить ответ. Повторите запрос, чтобы проверить результат.')
    } finally {
      inFlight.current = false
      setIsTyping(false)
    }
  }

  async function processUserMessage(rawMessage: string) {
    const message = rawMessage.trim()
    if (!message || inFlight.current || retryRequest) return
    addMessage(message)
    await requestChat({ message, requestId: crypto.randomUUID() })
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft.trim() || inFlight.current || retryRequest) return
    const message = draft
    setDraft('')
    await processUserMessage(message)
  }

  async function handleSuggestion(message: string) {
    await processUserMessage(message)
  }

  return (
    <main className="app-shell">
      <header className="chat-header">
        <div className="brand">
          <span className="avatar" aria-hidden="true">ai</span>
          <div>
            <h1>AI-buddy</h1>
            <p className="subtitle">Твой помощник в новой команде</p>
          </div>
        </div>
        <span className="demo-badge">Демо · синтетические данные</span>
      </header>

      <section
        ref={conversationRef}
        className="conversation"
        role="log"
        aria-label="История сообщений"
        aria-relevant="additions text"
      >
        {messages.length === 0 ? (
          <div className="empty-state">
            <div className="welcome-mark" aria-hidden="true">?</div>
            <p className="eyebrow">ПОМОЩНИК ДЛЯ ПЕРВЫХ НЕДЕЛЬ</p>
            <h2>С чего начнём?</h2>
            <p className="welcome-text">
              Задай вопрос или попроси помочь с доступом и поиском коллеги — всё происходит в одном чате.
            </p>
            <div className="suggestions" aria-label="Примеры запросов">
              <button className="suggestion" type="button" onClick={() => void handleSuggestion('Что такое SPT?')}>
                Что такое SPT?
              </button>
              <button className="suggestion" type="button" onClick={() => void handleSuggestion('Оформи демо-заявку на доступ к Jira')}>
                Оформи доступ к Jira
              </button>
              <button className="suggestion" type="button" onClick={() => void handleSuggestion('Найди владельца сервиса billing')}>
                Найди владельца billing
              </button>
            </div>
          </div>
        ) : (
          <ol className="message-list">
            {messages.map((message) => (
              <li key={message.id} className={`message message-${message.role}`}>
                <span className="message-author">{message.role === 'user' ? 'Вы' : 'AI-buddy'}</span>
                <p>{message.content}</p>
                {message.sources && message.sources.length > 0 && (
                  <ul className="source-list" aria-label="Источники ответа">
                    {message.sources.map((source) => (
                      <li key={`${source.title}-${source.url}`}>
                        <a href={source.url} target="_blank" rel="noreferrer">{source.title}</a>
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            ))}
          </ol>
        )}

        {actionError && (
          <div className="feedback feedback-error" role="alert">
            <span>{actionError}</span>
            {retryRequest && (
              <button type="button" disabled={isTyping} onClick={() => retryRequest && void requestChat(retryRequest)}>
                Повторить запрос
              </button>
            )}
          </div>
        )}
      </section>

      {isTyping && (
        <p className="typing-status" role="status">
          <span className="typing-dots" aria-hidden="true"><span /><span /><span /></span>
          AI-buddy готовит ответ…
        </p>
      )}

      <div className="composer-area">
        <form className="composer" aria-label="Отправка сообщения" onSubmit={(event) => void handleSubmit(event)}>
          <label className="sr-only" htmlFor="chat-message">Сообщение AI-buddy</label>
          <textarea
            ref={inputRef}
            id="chat-message"
            rows={2}
            maxLength={8000}
            placeholder="Напиши вопрос или действие…"
            aria-describedby="composer-hint"
            value={draft}
            disabled={isTyping || retryRequest !== null}
            onChange={(event) => setDraft(event.target.value)}
          />
          <button type="submit" className="send-button" disabled={!draft.trim() || isTyping || retryRequest !== null}>
            Отправить
          </button>
        </form>
        <p id="composer-hint" className="composer-hint">
          AI сейчас отвечает заглушкой. Действия с доступом и сообщениями — демонстрационные mock-записи.
        </p>
      </div>

      <footer className="chat-footer">
        <span>ЧУГУН<span className="footer-separator">/</span>Hack&amp;Change 2026</span>
        <span>Только открытые и синтетические данные</span>
      </footer>
    </main>
  )
}
