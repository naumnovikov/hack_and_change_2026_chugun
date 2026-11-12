import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { useChatStore } from './stores/chatStore'

const response = (reply: string) => new Response(JSON.stringify({ reply }))
beforeEach(() => {
  useChatStore.setState(useChatStore.getInitialState(), true)
  vi.stubGlobal('fetch', vi.fn())
})
function submit(text: string) {
  fireEvent.change(screen.getByRole('textbox'), { target: { value: text } })
  fireEvent.submit(screen.getByRole('form'))
}
describe('Server-owned chat', () => {
  it('shows examples and sends a suggestion through the chat endpoint', async () => {
    vi.mocked(fetch).mockResolvedValue(response('Создать демо-заявку?'))
    render(<App />)
    expect(screen.getByRole('button', { name: 'Что такое SPT?' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Найди владельца billing' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Оформи доступ к Jira' }))
    expect(await screen.findByText('Создать демо-заявку?')).toBeVisible()
    expect(fetch).toHaveBeenCalledWith('/api/chat', expect.objectContaining({
      body: JSON.stringify({ message: 'Оформи демо-заявку на доступ к Jira' }),
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-Request-ID': expect.any(String) },
    }))
    expect(fetch).toHaveBeenCalledTimes(1)
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
