import { ref } from 'vue'

export const authed = ref(false)
export const agentAuthed = ref(false)

export class ApiError extends Error {
  constructor(public code: string, message: string) {
    super(message)
  }
}

type Scope = 'admin' | 'agent'

function csrfKey(scope: Scope) {
  return scope === 'agent' ? 'devour_agent_csrf' : 'devour_csrf'
}

export function getCSRF(scope: Scope = 'admin'): string {
  return sessionStorage.getItem(csrfKey(scope)) ?? ''
}

export function setCSRF(v: string, scope: Scope = 'admin'): void {
  sessionStorage.setItem(csrfKey(scope), v)
}

type Body = { ok: boolean; code?: string; message?: string; data?: unknown }

async function request<T>(path: string, init: RequestInit = {}, scope: Scope = 'admin'): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase()
  const headers: Record<string, string> = {}
  if (init.body) headers['Content-Type'] = 'application/json'
  if (method !== 'GET' && method !== 'HEAD') headers['X-CSRF-Token'] = getCSRF(scope)

  const res = await fetch(path, {
    ...init,
    headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
    credentials: 'same-origin',
  })
  const body = (await res.json().catch(() => ({
    ok: false,
    code: 'BAD_INPUT',
    message: '响应不是合法 JSON',
  }))) as Body

  if (!body.ok) {
    if (body.code === 'UNAUTHORIZED') {
      if (scope === 'agent') agentAuthed.value = false
      else authed.value = false
    }
    throw new ApiError(body.code ?? 'INTERNAL', body.message ?? '请求失败')
  }
  return body.data as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: body === undefined ? undefined : JSON.stringify(body) }),
}

export const agentApi = {
  get: <T>(path: string) => request<T>(path, {}, 'agent'),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }, 'agent'),
}

export async function tryRestore(): Promise<boolean> {
  try {
    await request('/admin/api/me')
    authed.value = true
    return true
  } catch {
    authed.value = false
    return false
  }
}

export async function tryRestoreAgent(): Promise<boolean> {
  try {
    await request('/agent/api/me', {}, 'agent')
    agentAuthed.value = true
    return true
  } catch {
    agentAuthed.value = false
    return false
  }
}
