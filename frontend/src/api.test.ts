import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sendChatMessage } from './api'
beforeEach(() => vi.stubGlobal('fetch', vi.fn()))
describe('Chat API contract', () => {
  it('sends the caller key and keeps the conversation cookie', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ reply: 'Ответ', sources: [{ title: 'Источник', url: 'https://example.org' }] })))
    await expect(sendChatMessage('Вопрос', 'stable-request-key')).resolves.toMatchObject({ reply: 'Ответ' })
    expect(fetch).toHaveBeenCalledWith('/api/chat', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'stable-request-key' }, body: '{"message":"Вопрос"}' })
  })
  it.each([
    ['empty reply', { reply: '' }],
    ['bad source', { reply: 'a', sources: [{ title: 'a', url: 'javascript:alert(1)' }] }],
    ['unexpected tool', { reply: 'a', tool: 'send_message' }],
  ])('rejects %s', async (_, payload) => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify(payload)))
    await expect(sendChatMessage('hi', 'request-001')).rejects.toThrow('неожиданном формате')
  })
  it('rejects invalid JSON', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response('!'))
    await expect(sendChatMessage('hi', 'request-001')).rejects.toThrow('неожиданном формате')
  })
  it('shows a structured Backend error', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ code: 'ai_timeout', error: 'AI не ответил' }), { status: 504 }))
    await expect(sendChatMessage('hi', 'request-001')).rejects.toThrow('AI не ответил')
  })
  it('handles an HTML gateway error', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response('<html>bad gateway</html>', { status: 502 }))
    await expect(sendChatMessage('hi', 'request-001')).rejects.toThrow('502')
  })
  it('handles network failures', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('offline'))
    await expect(sendChatMessage('hi', 'request-001')).rejects.toThrow('Не удалось связаться')
  })
})
