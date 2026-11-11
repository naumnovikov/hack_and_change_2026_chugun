import { beforeEach, describe, expect, it } from 'vitest'
import { useChatStore } from './chatStore'

beforeEach(() => useChatStore.setState(useChatStore.getInitialState(), true))

describe('Хранилище чата', () => {
  it('начинает с пустой истории и выключенного индикатора печати', () => {
    expect(useChatStore.getState().messages).toEqual([])
    expect(useChatStore.getState().isTyping).toBe(false)
  })

  it('добавляет сообщения по порядку с разными ID, не изменяя прежний массив', () => {
    const initial = useChatStore.getState().messages
    useChatStore.getState().addMessage('  Первый вопрос  ')
    const previous = useChatStore.getState().messages
    useChatStore.getState().addMessage('Ответ\nна вопрос', 'assistant')
    const messages = useChatStore.getState().messages

    expect(initial).toHaveLength(0)
    expect(previous).toHaveLength(1)
    expect(messages.map(({ content, role }) => ({ content, role }))).toEqual([
      { content: 'Первый вопрос', role: 'user' },
      { content: 'Ответ\nна вопрос', role: 'assistant' },
    ])
    expect(new Set(messages.map(({ id }) => id)).size).toBe(2)
    expect(messages.every(({ id }) => id.length > 0)).toBe(true)
  })

  it.each(['', '  ', '\n\t'])('игнорирует пустой текст %j', (text) => {
    useChatStore.getState().addMessage(text)
    expect(useChatStore.getState().messages).toHaveLength(0)
  })

  it('переключает индикатор печати, сохраняя сообщения', () => {
    useChatStore.getState().addMessage('Вопрос')
    const messages = useChatStore.getState().messages
    useChatStore.getState().setIsTyping(true)
    expect(useChatStore.getState().isTyping).toBe(true)
    useChatStore.getState().setIsTyping(false)
    expect(useChatStore.getState().isTyping).toBe(false)
    expect(useChatStore.getState().messages).toBe(messages)
  })
})
