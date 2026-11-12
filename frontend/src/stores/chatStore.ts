import { create } from 'zustand'

type MessageRole = 'user' | 'assistant'

export interface ChatMessage {
  id: string
  role: MessageRole
  content: string
}

interface ChatState {
  messages: ChatMessage[]
  isTyping: boolean
  addMessage: (content: string, role?: MessageRole) => void
  setIsTyping: (isTyping: boolean) => void
}

export const useChatStore = create<ChatState>()((set) => ({
  messages: [],
  isTyping: false,
  addMessage: (content, role = 'user') => {
    const text = content.trim()
    if (!text) return

    const message: ChatMessage = { id: crypto.randomUUID(), role, content: text }
    set((state) => ({ messages: [...state.messages, message] }))
  },
  setIsTyping: (isTyping) => set({ isTyping }),
}))
