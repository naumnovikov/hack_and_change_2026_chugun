import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { presentationReply } from './test/presentationFixture'
import { useChatStore } from './stores/chatStore'

const response = (reply: string) => new Response(JSON.stringify({ reply }))
beforeEach(() => {
  useChatStore.setState(useChatStore.getInitialState(), true)
  vi.stubGlobal('fetch', vi.fn())
  vi.stubGlobal('URL', class extends URL {
    static createObjectURL = vi.fn(() => 'blob:preview-pdf')
    static revokeObjectURL = vi.fn()
  })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
function submit(text: string) {
  fireEvent.change(screen.getByRole('textbox'), { target: { value: text } })
  fireEvent.submit(screen.getByRole('form'))
}
describe('Server-owned chat', () => {
  it('downloads a PDF through the same-origin API and releases the blob URL', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(presentationReply)))
      .mockResolvedValueOnce(new Response('%PDF-test', { headers: { 'Content-Type': 'application/pdf' } }))
    render(<App />); submit('Собери презентацию')
    fireEvent.click(await screen.findByRole('button', { name: 'Скачать PDF' }))
    await screen.findByRole('button', { name: 'Скачать PDF' })
    expect(fetch).toHaveBeenLastCalledWith(presentationReply.attachment.download_url, { credentials: 'same-origin' })
    expect(URL.createObjectURL).toHaveBeenCalledTimes(1)
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:preview-pdf')
  })
  it('explains a 404 and requests a fresh presentation with a new request key', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(presentationReply)))
      .mockResolvedValueOnce(new Response('', { status: 404 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ reply: 'Новая презентация готова', attachment: presentationReply.attachment })))
    render(<App />); submit('Собери презентацию')
    fireEvent.click(await screen.findByRole('button', { name: 'Скачать PDF' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Презентация недоступна')
    expect(URL.createObjectURL).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(2)
    fireEvent.click(screen.getByRole('button', { name: 'Запросить презентацию заново' }))
    expect(await screen.findByText('Новая презентация готова')).toBeVisible()
    const chats = vi.mocked(fetch).mock.calls.filter(([path]) => path === '/api/chat')
    expect(chats).toHaveLength(2)
    const keys = chats.map(([, options]) => (options!.headers as Record<string, string>)['X-Request-ID'])
    expect(keys[0]).not.toBe(keys[1])
  })
  it('renders a PDF download from the server response and keeps the same-origin URL', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify(presentationReply)))
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'Собери презентацию' }))
    await screen.findByRole('button', { name: 'Скачать PDF' })
    expect(screen.getByText(presentationReply.reply)).toBeVisible()
    expect(screen.getByText(presentationReply.attachment.filename)).toBeVisible()
    expect(screen.getByText('Демо-черновик · PDF')).toBeVisible()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(vi.mocked(fetch).mock.calls[0]?.[1]).toMatchObject({ credentials: 'same-origin' })
  })
  it('leaves an ordinary response without a PDF download card', async () => {
    vi.mocked(fetch).mockResolvedValue(response('Обычный ответ без файла'))
    render(<App />); submit('Привет')
    expect(await screen.findByText('Обычный ответ без файла')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Скачать PDF' })).not.toBeInTheDocument()
    expect(screen.queryByText('Демо-черновик · PDF')).not.toBeInTheDocument()
  })
  it('shows examples and sends a suggestion through the chat endpoint', async () => {
    vi.mocked(fetch).mockResolvedValue(response('Создать демо-заявку?'))
    render(<App />)
    expect(screen.getByRole('button', { name: 'Что такое SPT?' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Собери презентацию' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Оформи доступ к Jira' }))
    expect(await screen.findByText('Создать демо-заявку?')).toBeVisible()
    expect(fetch).toHaveBeenCalledWith('/api/chat', expect.objectContaining({
      body: JSON.stringify({ message: 'Оформи демо-заявку на доступ к Jira' }),
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-Request-ID': expect.any(String) },
    }))
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('describes the live RAG and clearly labels demo actions', () => {
    render(<App />)
    expect(screen.getByText(/Ответы основаны на открытых материалах/)).toBeVisible()
    expect(screen.queryByText(/AI сейчас отвечает заглушкой/)).not.toBeInTheDocument()
    expect(screen.getByText(/реальные доступы не выдаются/)).toBeVisible()
  })
  it('renders sources returned by Backend', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ reply: 'Ответ по источнику', sources: [{ title: 'Playbook', url: 'https://example.org/spt' }] })))
    render(<App />); submit('Что такое SPT?')
    expect(await screen.findByText('Ответ по источнику')).toBeVisible()
    expect(screen.getByRole('link', { name: 'Playbook' })).toHaveAttribute('href', 'https://example.org/spt')
  })
  it('sends cancellation and confirmation as ordinary messages with fresh keys', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(response('Проверь черновик'))
      .mockResolvedValueOnce(response('Отменено'))
      .mockResolvedValueOnce(response('Нет действия'))
    render(<App />)
    submit('Найди владельца billing'); await screen.findByText('Проверь черновик')
    submit('отмена'); await screen.findByText('Отменено')
    submit('подтверждаю'); await screen.findByText('Нет действия')
    const calls = vi.mocked(fetch).mock.calls
    expect(calls.every(([path]) => path === '/api/chat')).toBe(true)
    expect(new Set(calls.map(([, options]) => (options!.headers as Record<string, string>)['X-Request-ID'])).size).toBe(3)
    expect(calls.map(([, options]) => JSON.parse(options?.body as string).message)).toEqual(['Найди владельца billing', 'отмена', 'подтверждаю'])
  })
  it('retries an uncertain confirmation with exactly the same key and no duplicate user message', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('lost response')).mockResolvedValueOnce(response('DEMO-MSG-42 сохранено'))
    render(<App />); submit('подтверждаю')
    expect(await screen.findByRole('alert')).toHaveTextContent('Не удалось связаться')
    expect(screen.getByRole('textbox')).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить запрос' }))
    await screen.findByText('DEMO-MSG-42 сохранено')
    const calls = vi.mocked(fetch).mock.calls
    expect(calls[0]).toEqual(calls[1])
    expect(within(screen.getByRole('log')).getAllByText('подтверждаю')).toHaveLength(1)
    expect(screen.getByRole('textbox')).toBeEnabled()
    expect(screen.getByRole('textbox')).toHaveFocus()
  })
  it('does not issue overlapping requests or lose order', async () => {
    let resolve!: (value: Response) => void
    vi.mocked(fetch).mockReturnValue(new Promise<Response>((done) => { resolve = done }))
    render(<App />)
    submit('Первый вопрос')
    expect(screen.getByRole('status')).toHaveTextContent('готовит ответ')
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    submit('Второй вопрос')
    expect(fetch).toHaveBeenCalledTimes(1)
    await act(async () => resolve(response('Ответ')))
    expect(await screen.findByText('Ответ')).toBeVisible()
  })
  it('does not invent action success on a server error', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ code: 'storage_unavailable', error: 'Хранилище недоступно' }), { status: 503 }))
    render(<App />); submit('Оформи доступ к Jira')
    expect(await screen.findByRole('alert')).toHaveTextContent('Хранилище недоступно')
    expect(screen.queryByText(/DEMO-ACCESS/)).not.toBeInTheDocument()
  })
  it('ignores an empty submission', () => {
    render(<App />); submit('   ')
    expect(fetch).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
  })
})
