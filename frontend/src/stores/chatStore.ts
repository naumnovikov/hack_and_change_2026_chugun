import { create } from 'zustand'
import type { ChatAttachment, ChatSource } from '../api'

type MessageRole = 'user' | 'assistant'

export interface ChatMessage {
  id: string
  role: MessageRole
  content: string
  sources?: ChatSource[]
  attachment?: ChatAttachment
}

interface ChatState {
  messages: ChatMessage[]
  isTyping: boolean
  addMessage: (content: string, role?: MessageRole, sources?: ChatSource[], attachment?: ChatAttachment) => void
  setIsTyping: (isTyping: boolean) => void
}

export const useChatStore = create<ChatState>()((set) => ({
  messages: [],
  isTyping: false,
  addMessage: (content, role = 'user', sources, attachment) => {
    const text = content.trim()
    if (!text) return

    const message: ChatMessage = {
      id: crypto.randomUUID(),
      role,
      content: text,
      ...(sources ? { sources } : {}),
      ...(attachment ? { attachment } : {}),
    }
    set((state) => ({ messages: [...state.messages, message] }))
  },
  setIsTyping: (isTyping) => set({ isTyping }),
}))
