import { useEffect, useRef, useState, type FormEvent } from 'react'
import {
  createAccessRequest,
  findEmployee,
  sendChatMessage,
  sendMockMessage,
  type Employee,
  type SendMessageResult,
} from './api'
import { useChatStore } from './stores/chatStore'

type PendingOwner = {
  employee: Employee
  serviceName: string
  message: string
  result?: SendMessageResult
}

type FollowUp = 'access-service' | 'owner-service' | null

function accessServiceFrom(message: string): string | null {
  const match = message.match(/(?:доступ|заявк\w*).{0,40}?\s(?:к|в)\s+(.+?)[?.!,]*$/i)
  return match?.[1]?.trim() || null
}

function ownerServiceFrom(message: string): string | null {
  const match = message.match(/(?:владел\w*|ответственн\w*|ревью|owner).{0,40}?(?:сервис(?:а)?\s+)?([a-zа-я0-9._-]+)\s*[?.!,]*$/i)
  return match?.[1]?.trim() || null
}

export default function App() {
  const messages = useChatStore((state) => state.messages)
  const addMessage = useChatStore((state) => state.addMessage)
  const isTyping = useChatStore((state) => state.isTyping)
  const setIsTyping = useChatStore((state) => state.setIsTyping)
  const [draft, setDraft] = useState('')
  const [followUp, setFollowUp] = useState<FollowUp>(null)
  const [pendingOwner, setPendingOwner] = useState<PendingOwner | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [retryMessage, setRetryMessage] = useState<string | null>(null)
  const conversationRef = useRef<HTMLElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const conversation = conversationRef.current
    if (conversation) conversation.scrollTop = conversation.scrollHeight
  }, [messages, isTyping, pendingOwner, actionError])

  async function handleAccess(serviceName: string) {
    setActionError(null)
    setIsTyping(true)
    try {
      const result = await createAccessRequest(serviceName)
      addMessage(
        `Создана демо-заявка ${result.ticket_id} на ${serviceName}. Статус: ${result.status}. ${result.message} Доступ не выдан: это запись в демонстрационной базе.`,
        'assistant',
      )
    } catch (error) {
      setActionError(
        `${error instanceof Error ? error.message : 'Не удалось создать демо-заявку.'} Результат мог быть не подтверждён; проверь запись перед повторной отправкой.`,
      )
    } finally {
      setIsTyping(false)
    }
  }

  async function handleOwnerSearch(serviceName: string) {
    setActionError(null)
    setPendingOwner(null)
    setIsTyping(true)
    try {
      const employee = await findEmployee(serviceName)
      addMessage(`Нашёл демо-владельца сервиса ${serviceName}: ${employee.name}, ${employee.role} (${employee.mattermost_id}). Это синтетические данные.`, 'assistant')
      setPendingOwner({
        employee,
        serviceName,
        message: `Привет! Нужна помощь с ревью по сервису ${serviceName}. Сможешь посмотреть?`,
      })
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Не удалось найти демо-владельца.')
    } finally {
      setIsTyping(false)
    }
  }

  async function handleMockMessage() {
    if (!pendingOwner || pendingOwner.result || !pendingOwner.message.trim()) return
    setActionError(null)
    setIsTyping(true)
    try {
      const result = await sendMockMessage(pendingOwner.employee.employee_id, pendingOwner.message.trim())
      setPendingOwner((current) => current ? { ...current, result } : current)
      addMessage(
        `Демо-сообщение ${result.message_id} сохранено для ${pendingOwner.employee.name}. Реальное сообщение коллеге не отправлялось.`,
        'assistant',
      )
    } catch (error) {
      setActionError(
        `${error instanceof Error ? error.message : 'Не удалось сохранить демо-сообщение.'} Если результат запроса неясен, проверьте его перед повторной отправкой.`,
      )
    } finally {
      setIsTyping(false)
    }
  }

  async function requestChat(message: string, addUserMessage: boolean) {
    if (addUserMessage) addMessage(message)
    setRetryMessage(message)
    setActionError(null)
    setIsTyping(true)
    try {
      const response = await sendChatMessage(message)
      addMessage(response.reply, 'assistant', response.sources)
      setRetryMessage(null)
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Не удалось получить ответ от Backend.')
    } finally {
      setIsTyping(false)
    }
  }

  async function processUserMessage(rawMessage: string) {
    const message = rawMessage.trim()
    if (!message || isTyping) return
    addMessage(message)
    setActionError(null)
    setRetryMessage(null)

    if (followUp === 'access-service') {
      setFollowUp(null)
      await handleAccess(message)
      return
    }
    if (followUp === 'owner-service') {
      setFollowUp(null)
      await handleOwnerSearch(message)
      return
    }

    const asksForAccess = /(?:оформ|созда|запрос|закаж|нужен|нужна|нужно\s+(?:получить|выдать|оформить))/i.test(message)
      && /(?:доступ|заявк\w*)/i.test(message)
    if (asksForAccess) {
      const serviceName = accessServiceFrom(message)
      if (serviceName) {
        await handleAccess(serviceName)
      } else {
        setFollowUp('access-service')
        addMessage('К какому сервису нужен доступ? Назови, например, Jira или Confluence.', 'assistant')
      }
      return
    }

    const asksForOwner = /(?:найди|найти|кто|кому|подскажи|покажи|узнай)/i.test(message)
      && /(?:владел\w*|ответственн\w*|ревью|owner)/i.test(message)
    if (asksForOwner) {
      const serviceName = ownerServiceFrom(message)
      if (serviceName) {
        await handleOwnerSearch(serviceName)
      } else {
        setFollowUp('owner-service')
        addMessage('Какой сервис или команду нужно проверить? Например, billing.', 'assistant')
      }
      return
    }

    await requestChat(message, false)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const message = draft.trim()
    if (!message || isTyping) return
    setDraft('')
    await processUserMessage(message)
    inputRef.current?.focus()
  }

  async function handleSuggestion(message: string) {
    if (isTyping) return
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

        {pendingOwner && !pendingOwner.result && (
          <div className="message message-assistant action-card">
            <span className="message-author">Проверь перед отправкой</span>
            <p><strong>{pendingOwner.employee.name}</strong> · {pendingOwner.employee.role} · {pendingOwner.employee.mattermost_id}</p>
            <label htmlFor="review-message">Текст демо-сообщения</label>
            <textarea
              id="review-message"
              rows={3}
              maxLength={2000}
              value={pendingOwner.message}
              disabled={isTyping}
              onChange={(event) => setPendingOwner({ ...pendingOwner, message: event.target.value })}
            />
            <p className="composer-hint">Только после нажатия кнопки будет создана запись в mock-БД. Реальному коллеге сообщение не уйдёт.</p>
            <button
              type="button"
              className="primary-button"
              disabled={!pendingOwner.message.trim() || isTyping}
              onClick={() => void handleMockMessage()}
            >
              Подтвердить демо-отправку
            </button>
          </div>
        )}

        {actionError && (
          <div className="feedback feedback-error" role="alert">
            <span>{actionError}</span>
            {retryMessage && (
              <button type="button" disabled={isTyping} onClick={() => void requestChat(retryMessage, false)}>
                Повторить вопрос
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
            disabled={isTyping}
            onChange={(event) => setDraft(event.target.value)}
          />
          <button type="submit" className="send-button" disabled={!draft.trim() || isTyping}>
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
