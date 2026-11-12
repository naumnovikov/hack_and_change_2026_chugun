import { z } from 'zod'

const sourceSchema = z.object({
  title: z.string().trim().min(1),
  url: z.string().url().refine((value) => ['http:', 'https:'].includes(new URL(value).protocol)),
}).strict()
const chatResponseSchema = z.object({
  reply: z.string().trim().min(1),
  sources: z.array(sourceSchema).optional(),
}).strict()
const errorResponseSchema = z.object({ code: z.string(), error: z.string() })
export type ChatSource = z.infer<typeof sourceSchema>
export type ChatReply = z.infer<typeof chatResponseSchema>

// The caller owns the key: never generate a new one for a retry.
export async function sendChatMessage(message: string, requestId: string): Promise<ChatReply> {
  let response: Response
  try {
    response = await fetch('/api/chat', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-Request-ID': requestId },
      body: JSON.stringify({ message }),
    })
  } catch {
    throw new Error('Не удалось связаться с Backend. Повторите запрос, чтобы проверить его результат.')
  }
  if (!response.ok) {
    const payload: unknown = await response.json().catch(() => null)
    const parsed = errorResponseSchema.safeParse(payload)
    throw new Error(parsed.success ? parsed.data.error : `Backend вернул ошибку (${response.status}). Повторите запрос позже.`)
  }
  const payload: unknown = await response.json().catch(() => null)
  const parsed = chatResponseSchema.safeParse(payload)
  if (!parsed.success) throw new Error('Backend вернул ответ в неожиданном формате. Повторите запрос, чтобы проверить результат.')
  return parsed.data
}
