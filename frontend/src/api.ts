import { z } from 'zod'

const sourceSchema = z.object({
  title: z.string().trim().min(1),
  url: z.string().url().refine((value) => ['http:', 'https:'].includes(new URL(value).protocol)),
}).strict()

const chatResponseSchema = z.object({
  reply: z.string().trim().min(1),
  sources: z.array(sourceSchema).optional(),
}).strict()

const accessRequestResponseSchema = z.object({
  ticket_id: z.string().min(1),
  status: z.string().min(1),
  message: z.string(),
}).strict()

const employeeResponseSchema = z.object({
  employee_id: z.string().min(1),
  name: z.string().min(1),
  role: z.string().min(1),
  mattermost_id: z.string().min(1),
}).strict()

const sendMessageResponseSchema = z.object({
  message_id: z.string().min(1),
  sent_at: z.string().datetime({ offset: true }),
}).strict()

const errorResponseSchema = z.object({
  code: z.string(),
  error: z.string(),
})

export type ChatSource = z.infer<typeof sourceSchema>
export type ChatReply = z.infer<typeof chatResponseSchema>
export type Employee = z.infer<typeof employeeResponseSchema>
export type AccessRequestResult = z.infer<typeof accessRequestResponseSchema>
export type SendMessageResult = z.infer<typeof sendMessageResponseSchema>

async function requestJson<T>(
  path: string,
  schema: z.ZodType<T>,
  body?: Record<string, string>,
): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, {
      method: body ? 'POST' : 'GET',
      ...(body
        ? {
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
          }
        : {}),
    })
  } catch {
    throw new Error('Не удалось связаться с Backend. Проверьте подключение и повторите попытку.')
  }

  if (!response.ok) {
    const payload: unknown = await response.json().catch(() => null)
    const parsedError = errorResponseSchema.safeParse(payload)
    throw new Error(
      parsedError.success
        ? parsedError.data.error
        : `Backend вернул ошибку (${response.status}). Повторите попытку позже.`,
    )
  }

  const payload: unknown = await response.json().catch(() => null)
  const parsed = schema.safeParse(payload)
  if (!parsed.success) {
    throw new Error('Backend вернул ответ в неожиданном формате. Обратитесь к команде.')
  }
  return parsed.data
}

export function sendChatMessage(message: string): Promise<ChatReply> {
  return requestJson('/api/chat', chatResponseSchema, { message })
}

export function createAccessRequest(serviceName: string): Promise<AccessRequestResult> {
  return requestJson('/api/mock/access-request', accessRequestResponseSchema, {
    service_name: serviceName,
  })
}

export function findEmployee(serviceName: string): Promise<Employee> {
  const query = new URLSearchParams({ service_name: serviceName })
  return requestJson(`/api/mock/employee?${query}`, employeeResponseSchema)
}

export function sendMockMessage(recipientId: string, text: string): Promise<SendMessageResult> {
  return requestJson('/api/mock/message', sendMessageResponseSchema, {
    recipient_id: recipientId,
    text,
  })
}
