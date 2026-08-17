export const errorMessage = (reason: unknown) => reason instanceof Error ? reason.message : String(reason)

async function responseError(response: Response) {
  const body = await response.json().catch(() => null) as { error?: unknown } | null
  return new Error(typeof body?.error === 'string' ? body.error : `HTTP ${response.status}`)
}

export async function requestJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const response = await fetch(input, init)
  if (!response.ok) throw await responseError(response)
  return response.json() as Promise<T>
}

export async function requestText(input: RequestInfo | URL, allowNotFound = false): Promise<string> {
  const response = await fetch(input)
  if (allowNotFound && response.status === 404) return ''
  if (!response.ok) throw await responseError(response)
  return response.text()
}
