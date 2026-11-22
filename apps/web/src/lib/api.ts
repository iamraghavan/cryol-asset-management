export class RequestError extends Error {
  constructor(readonly status: number, message: string) { super(message) }
}

export async function request<T>(path: string, method = 'GET', payload?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch('/api/v1' + path, {
    method,
    signal,
    headers: payload === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: payload === undefined ? undefined : JSON.stringify(payload),
  })
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new RequestError(response.status, body?.error?.message ?? 'Request could not be completed.')
  }
  return response.json()
}
