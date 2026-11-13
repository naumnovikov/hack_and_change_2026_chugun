import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchPresentation, sendChatMessage } from './api'
import { presentationReply } from './test/presentationFixture'
beforeEach(() => vi.stubGlobal('fetch', vi.fn()))
describe('Chat API contract', () => {
  it('accepts the optional PDF attachment from the agreed contract', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify(presentationReply)))
    await expect(sendChatMessage('Собери презентацию', 'request-pdf')).resolves.toEqual(presentationReply)
  })
  it.each([
    { download_url: 'https://evil.example/file.pdf' },
    { download_url: '//evil.example/file.pdf' },
    { download_url: 'javascript:alert(1)' },
    { download_url: '/api/presentations/../private' },
    { content_type: 'text/html' },
    { filename: '' },
    { expires_at: 'invalid' },
    { id: '' },
    { extra: 'unexpected' },
  ])('rejects malformed or unsafe attachments: %j', async (patch) => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({
      ...presentationReply, attachment: { ...presentationReply.attachment, ...patch },
    })))
    await expect(sendChatMessage('PDF', 'request-pdf')).rejects.toThrow('неожиданном формате')
  })
  it('accepts a plain reply without adding an attachment', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ reply: 'Обычный ответ' })))
    await expect(sendChatMessage('Привет', 'request-plain')).resolves.toEqual({ reply: 'Обычный ответ' })
  })
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

describe('PDF download', () => {
  it('downloads a PDF with the conversation cookie', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response('%PDF-test', { headers: { 'Content-Type': 'application/pdf' } }))
    const blob = await fetchPresentation(presentationReply.attachment as Parameters<typeof fetchPresentation>[0])
    expect(blob.size).toBeGreaterThan(0)
    expect(fetch).toHaveBeenCalledWith(presentationReply.attachment.download_url, { credentials: 'same-origin' })
  })
  it.each([
    [404, 'application/json', 'Презентация недоступна'],
    [500, 'application/json', 'Попробуйте позже'],
    [200, 'text/html', 'неожиданном формате'],
  ])('handles download status %s / %s', async (status, mime, error) => {
    vi.mocked(fetch).mockResolvedValue(new Response('file', { status, headers: { 'Content-Type': mime } }))
    await expect(fetchPresentation(presentationReply.attachment as Parameters<typeof fetchPresentation>[0])).rejects.toThrow(error)
  })
  it('reports network failure without starting a new generation', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('offline'))
    await expect(fetchPresentation(presentationReply.attachment as Parameters<typeof fetchPresentation>[0])).rejects.toThrow('Проверьте соединение')
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('handles an interrupted body', async () => {
    const result = new Response('', { headers: { 'Content-Type': 'application/pdf' } })
    result.blob = () => Promise.reject(new Error('interrupted'))
    vi.mocked(fetch).mockResolvedValue(result)
    await expect(fetchPresentation(presentationReply.attachment as Parameters<typeof fetchPresentation>[0])).rejects.toThrow('прервалась')
  })
  it('refuses an unsafe URL before sending a request', async () => {
    await expect(fetchPresentation({ ...presentationReply.attachment, content_type: 'application/pdf', download_url: 'https://evil.example/file' })).rejects.toThrow('Некорректная ссылка')
    expect(fetch).not.toHaveBeenCalled()
  })
})
