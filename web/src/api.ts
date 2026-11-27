import { ref } from 'vue'

export const authed = ref(false)

// 当前管理端上下文的应用（多应用切换）。放 localStorage：刷新、换标签页都保持一致。
let currentApp = localStorage.getItem('devour_app') ?? sessionStorage.getItem('devour_app') ?? ''
if (currentApp && !localStorage.getItem('devour_app')) localStorage.setItem('devour_app', currentApp)

export function getCurrentApp(): string {
  return currentApp
}

export function setCurrentApp(id: string) {
  currentApp = id
  localStorage.setItem('devour_app', id)
}
export const agentAuthed = ref(false)

// 当前管理员绑定的应用：0 = 全局，>0 = 只能看这个应用
export const adminAppID = ref(0)
export function setAdminApp(id: number | undefined) {
  adminAppID.value = id ?? 0
}

// 主动退出后，本次页面生命周期内不再尝试恢复会话，
// 否则退出请求失败时路由守卫会立刻把人送回后台（看起来像「退出没反应」）。
let signedOut = false
export function markSignedOut() {
  signedOut = true
  authed.value = false
  agentAuthed.value = false
}
export function markSignedIn() {
  signedOut = false
}

// 敏感操作需要 5 分钟内重新验证；请求层遇到 403 就挂起等用户验证完再重试一次。
export const reauthState = {
  show: ref(false),
  message: ref(''),
  pending: null as null | ((ok: boolean) => void),
}

export function resolveReauth(ok: boolean) {
  const p = reauthState.pending
  reauthState.pending = null
  reauthState.show.value = false
  if (p) p(ok)
}

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

async function request<T>(path: string, init: RequestInit = {}, scope: Scope = 'admin', retried = false): Promise<T> {
  // 管理端请求统一带上当前应用
  if (scope === 'admin' && currentApp && path.startsWith('/admin/api/')) {
    const u = new URL(path, location.origin)
    if (!u.searchParams.has('app_id') && !u.searchParams.has('app')) {
      u.searchParams.set('app_id', currentApp)
      path = u.pathname + '?' + u.searchParams.toString()
    }
  }
  const method = (init.method ?? 'GET').toUpperCase()
  const headers: Record<string, string> = {}
  if (init.body) headers['Content-Type'] = 'application/json'
  if (method !== 'GET' && method !== 'HEAD') headers['X-CSRF-Token'] = getCSRF(scope)

  // 15 秒超时：网络卡住时给明确反馈，而不是一直转圈
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), 15000)
  let res: Response
  try {
    res = await fetch(path, {
      ...init,
      headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
      credentials: 'same-origin',
      signal: ctrl.signal,
    })
  } catch (e) {
    throw new ApiError('NETWORK', (e as Error).name === 'AbortError' ? '请求超时，请重试' : '网络错误，请检查连接')
  } finally {
    clearTimeout(timer)
  }
  const body = (await res.json().catch(() => ({
    ok: false,
    code: 'BAD_INPUT',
    message: '响应不是合法 JSON',
  }))) as Body

  if (!body.ok) {
    // 需要 reauth：弹窗验证后重试一次
    if (scope === 'admin' && !retried && body.message?.includes('重新验证')) {
      const ok = await new Promise<boolean>((resolve) => {
        reauthState.message.value = '这个操作需要重新验证身份'
        reauthState.pending = resolve
        reauthState.show.value = true
      })
      if (ok) return request<T>(path, init, scope, true)
    }
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
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body === undefined ? undefined : JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: body === undefined ? undefined : JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}

export const agentApi = {
  get: <T>(path: string) => request<T>(path, {}, 'agent'),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }, 'agent'),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body === undefined ? undefined : JSON.stringify(body) }, 'agent'),
}

export async function tryRestore(): Promise<boolean> {
  if (signedOut) return false
  try {
    const me = await request<{ csrf_token?: string; username?: string; role?: string; app_id?: number }>('/admin/api/me')
    if (me?.csrf_token) setCSRF(me.csrf_token)
    setAdminApp(me?.app_id)
    // 新标签页/刷新后 sessionStorage 是空的，用 /me 补回来
    if (me?.username) sessionStorage.setItem('devour_user', `${me.username} · ${me.role ?? ''}`)
    authed.value = true
    return true
  } catch {
    authed.value = false
    return false
  }
}

export async function tryRestoreAgent(): Promise<boolean> {
  if (signedOut) return false
  try {
    const me = await request<{ name?: string; level?: number }>('/agent/api/me', {}, 'agent')
    if (me?.name) sessionStorage.setItem('devour_agent_name', `${me.name} · L${me.level ?? 1}`)
    agentAuthed.value = true
    return true
  } catch {
    agentAuthed.value = false
    return false
  }
}
