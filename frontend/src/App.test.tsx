import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { useChatStore } from './stores/chatStore'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  useChatStore.setState(useChatStore.getInitialState(), true)
  vi.stubGlobal('fetch', vi.fn())
})

async function submitMessage(text: string) {
  fireEvent.change(screen.getByRole('textbox', { name: 'Сообщение AI-buddy' }), {
    target: { value: text },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Отправить' }))
}

describe('Единый чат AI-buddy', () => {
  it('показывает три примера действий внутри одного чата', () => {
    render(<App />)

    expect(screen.getByRole('log', { name: 'История сообщений' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Что такое SPT?' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Оформи доступ к Jira' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Найди владельца billing' })).toBeVisible()
    expect(screen.queryByRole('navigation', { name: 'Сценарии адаптации' })).not.toBeInTheDocument()
  })

  it('отправляет вопрос в чат, показывает ответ и источник', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({
      reply: 'SPT — процесс проверки изменений.',
      sources: [{ title: 'AvitoTech Playbook: SPT', url: 'https://example.org/spt' }],
    }))
    render(<App />)

    await submitMessage('Что такое SPT?')

    expect(await screen.findByText('SPT — процесс проверки изменений.')).toBeVisible()
    expect(screen.getByRole('link', { name: 'AvitoTech Playbook: SPT' })).toHaveAttribute(
      'href',
      'https://example.org/spt',
    )
    expect(fetch).toHaveBeenCalledWith('/api/chat', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ message: 'Что такое SPT?' }),
    }))
  })

  it('повторяет упавший chat-запрос, не дублируя сообщение пользователя', async () => {
    vi.mocked(fetch)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(jsonResponse({ reply: 'Ответ после повтора.' }))
    render(<App />)
    await submitMessage('Вопрос по документации')

    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось связаться с Backend')
    fireEvent.click(screen.getByRole('button', { name: 'Повторить вопрос' }))
    expect(await screen.findByText('Ответ после повтора.')).toBeVisible()
    expect(within(screen.getByRole('log')).getAllByText('Вопрос по документации')).toHaveLength(1)
  })

  it('создаёт демо-заявку по сообщению и показывает номер и статус в чате', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({
      ticket_id: 'DEMO-ACCESS-17',
      status: 'created',
      message: 'Демо-заявка создана',
    }))
    render(<App />)

    await submitMessage('Оформи демо-заявку на доступ к Jira')

    expect(await screen.findByText(/DEMO-ACCESS-17/)).toBeVisible()
    expect(screen.getByText(/Статус: created/)).toBeVisible()
    expect(screen.getByText(/Доступ не выдан/)).toBeVisible()
    expect(fetch).toHaveBeenCalledWith('/api/mock/access-request', expect.objectContaining({
      body: JSON.stringify({ service_name: 'Jira' }),
    }))
  })

  it('уточняет сервис в чате, если пользователь не назвал его', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({
      ticket_id: 'DEMO-ACCESS-18',
      status: 'created',
      message: 'Создано',
    }))
    render(<App />)
    await submitMessage('Мне нужен доступ')

    expect(await screen.findByText(/К какому сервису нужен доступ/)).toBeVisible()
    await submitMessage('Confluence')

    expect(await screen.findByText(/DEMO-ACCESS-18/)).toBeVisible()
    expect(fetch).toHaveBeenCalledWith('/api/mock/access-request', expect.objectContaining({
      body: JSON.stringify({ service_name: 'Confluence' }),
    }))
  })

  it('не превращает информационный вопрос о доступе в действие создания заявки', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ reply: 'Уточни, какой именно доступ нужен.' }))
    render(<App />)
    await submitMessage('Что нужно знать про доступ к Jira?')

    expect(await screen.findByText('Уточни, какой именно доступ нужен.')).toBeVisible()
    expect(fetch).toHaveBeenCalledWith('/api/chat', expect.anything())
  })

  it('ищет владельца в чате и отправляет сообщение только после подтверждения', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse({
        employee_id: 'emp-demo-01',
        name: 'Демо-владелец',
        role: 'Tech Lead (billing)',
        mattermost_id: '@demo-owner',
      }))
      .mockResolvedValueOnce(jsonResponse({
        message_id: 'DEMO-MSG-19',
        sent_at: '2026-11-12T10:00:00Z',
      }))
    render(<App />)
    await submitMessage('Найди владельца сервиса billing')

    expect(await screen.findByLabelText('Текст демо-сообщения')).toBeVisible()
    expect(screen.getByLabelText('Текст демо-сообщения')).toBeVisible()
    expect(fetch).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Подтвердить демо-отправку' }))

    expect(await screen.findByText(/DEMO-MSG-19/)).toBeVisible()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch).toHaveBeenLastCalledWith('/api/mock/message', expect.objectContaining({
      body: JSON.stringify({
        recipient_id: 'emp-demo-01',
        text: 'Привет! Нужна помощь с ревью по сервису billing. Сможешь посмотреть?',
      }),
    }))
  })

  it('не создаёт ложное сообщение об успехе при ошибке mock API', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({
      code: 'storage_unavailable',
      error: 'Хранилище недоступно',
    }, 503))
    render(<App />)
    await submitMessage('Нужен доступ к Jira')

    expect(await screen.findByRole('alert')).toHaveTextContent('Хранилище недоступно')
    expect(screen.queryByText(/Создана демо-заявка/)).not.toBeInTheDocument()
  })

  it('не отправляет пустое сообщение', () => {
    render(<App />)
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    expect(fetch).not.toHaveBeenCalled()
  })
})
