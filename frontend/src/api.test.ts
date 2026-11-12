import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createAccessRequest,
  findEmployee,
  sendChatMessage,
  sendMockMessage,
} from './api'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => vi.stubGlobal('fetch', vi.fn()))

describe('Клиент Backend API', () => {
  it('валидирует ответ чата и источники', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({
      reply: 'Подтверждённый ответ',
      sources: [{ title: 'Playbook', url: 'https://example.org/playbook' }],
    }))

    await expect(sendChatMessage('Вопрос')).resolves.toMatchObject({
      reply: 'Подтверждённый ответ',
      sources: [{ title: 'Playbook' }],
    })
  })

  it('кодирует поисковый параметр и валидирует сотрудника', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({
      employee_id: 'demo-1',
      name: 'Демо',
      role: 'Tech Lead',
      mattermost_id: '@demo',
    }))

    await expect(findEmployee('billing & api')).resolves.toMatchObject({ employee_id: 'demo-1' })
    expect(fetch).toHaveBeenCalledWith('/api/mock/employee?service_name=billing+%26+api', expect.anything())
  })

  it('валидирует результаты mock действий', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse({ ticket_id: 'DEMO-1', status: 'created', message: 'Создано' }))
      .mockResolvedValueOnce(jsonResponse({ message_id: 'DEMO-2', sent_at: '2026-11-12T10:00:00Z' }))

    await expect(createAccessRequest('Jira')).resolves.toMatchObject({ ticket_id: 'DEMO-1' })
    await expect(sendMockMessage('demo-1', 'Проверь PR')).resolves.toMatchObject({ message_id: 'DEMO-2' })
  })

  it('не принимает некорректный успешный ответ', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ reply: '' }))
    await expect(sendChatMessage('Вопрос')).rejects.toThrow('неожиданном формате')
  })

  it('передаёт сообщение Backend-ошибки пользователю', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ code: 'ai_timeout', error: 'AI не ответил' }, 504))
    await expect(sendChatMessage('Вопрос')).rejects.toThrow('AI не ответил')
  })
})
