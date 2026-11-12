import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import App from './App'
import { useChatStore } from './stores/chatStore'

beforeEach(() => useChatStore.setState(useChatStore.getInitialState(), true))

describe('Экран чата', () => {
  it('показывает пустую историю и отключает отправку до ввода текста', () => {
    render(<App />)

    expect(screen.getByRole('heading', { name: 'AI-buddy', level: 1 })).toBeVisible()
    expect(screen.getByRole('log', { name: 'История сообщений' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Осваиваться проще вместе' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: 'Сообщение AI-buddy' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
  })

  it('добавляет сообщение в ленту, скрывает приветствие и очищает черновик', () => {
    render(<App />)
    const input = screen.getByRole('textbox', { name: 'Сообщение AI-buddy' })
    fireEvent.change(input, { target: { value: '  Как получить доступ к Jira?  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Отправить' }))

    const history = screen.getByRole('log')
    expect(within(history).getByText('Как получить доступ к Jira?')).toBeVisible()
    expect(within(history).getByText('Вы')).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Осваиваться проще вместе' })).not.toBeInTheDocument()
    expect(input).toHaveValue('')
    expect(input).toHaveAccessibleDescription(/исчезнут после обновления страницы/)
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    fireEvent.submit(screen.getByRole('form'))
    expect(useChatStore.getState().messages).toHaveLength(1)
  })

  it('не добавляет сообщение из пробелов даже при отправке формы', () => {
    render(<App />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: '  \n  ' } })
    fireEvent.submit(screen.getByRole('form'))

    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    expect(useChatStore.getState().messages).toHaveLength(0)
  })

  it('сохраняет порядок нескольких сообщений и выводит HTML как обычный текст', () => {
    render(<App />)
    for (const text of ['Первый вопрос', '<img src=x onerror=alert(1)>']) {
      fireEvent.change(screen.getByRole('textbox'), { target: { value: text } })
      fireEvent.click(screen.getByRole('button', { name: 'Отправить' }))
    }

    const items = within(screen.getByRole('log')).getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent('Первый вопрос')
    expect(items[1]).toHaveTextContent('<img src=x onerror=alert(1)>')
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('реагирует на сообщения ассистента и флаг печати из стора', () => {
    render(<App />)
    act(() => {
      useChatStore.getState().addMessage('Демонстрационный ответ', 'assistant')
      useChatStore.getState().setIsTyping(true)
    })

    expect(within(screen.getByRole('log')).getByText('AI-buddy')).toBeVisible()
    expect(screen.getByText('Демонстрационный ответ')).toBeVisible()
    expect(screen.getByRole('status')).toHaveTextContent('AI-buddy печатает…')
    act(() => useChatStore.getState().setIsTyping(false))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})
