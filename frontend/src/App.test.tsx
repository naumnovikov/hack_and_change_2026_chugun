import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import App from './App'

describe('Экран чата', () => {
  it('показывает имя бота, пустую историю и доступное поле ввода', () => {
    render(<App />)

    expect(screen.getByRole('heading', { name: 'AI-buddy', level: 1 })).toBeVisible()
    expect(screen.getByRole('log', { name: 'История сообщений' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Осваиваться проще вместе' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: 'Сообщение AI-buddy' })).toBeEnabled()
  })

  it('позволяет набрать черновик и обозначает недоступность отправки', () => {
    render(<App />)
    const input = screen.getByRole('textbox', { name: 'Сообщение AI-buddy' })

    fireEvent.change(input, { target: { value: 'Как получить доступ к Jira?' } })

    expect(input).toHaveValue('Как получить доступ к Jira?')
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    expect(input).toHaveAccessibleDescription(/отправка сообщений пока недоступна/)
  })
})
