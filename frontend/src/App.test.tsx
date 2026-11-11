import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import App from './App'

describe('Основа Frontend', () => {
  it('показывает название проекта и текущий этап вместо шаблона Vite', () => {
    render(<App />)

    expect(screen.getByRole('heading', { name: 'AI-buddy', level: 1 })).toBeVisible()
    expect(screen.getByText(/Интерфейс чата — следующий этап/)).toBeVisible()
  })
})
