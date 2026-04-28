export type ApiResponse<T> = { ok: true; data: T; error: null } | { ok: false; data: null; error: { code: string; message: string; details?: unknown } }

async function parsePayload<T>(res: Response): Promise<ApiResponse<T>> {
  const text = await res.text()
  if (!text) {
    return res.ok
      ? ({ ok: true, data: null as T, error: null })
      : ({ ok: false, data: null, error: { code: `HTTP_${res.status}`, message: res.statusText || '请求失败' } })
  }
  try {
    return JSON.parse(text) as ApiResponse<T>
  } catch {
    return { ok: false, data: null, error: { code: `HTTP_${res.status}`, message: text.slice(0, 200) || '服务器返回了无法解析的数据' } }
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init
  })
  const payload = await parsePayload<T>(res)
  if (!res.ok || !payload.ok) throw new Error(payload.error?.message || res.statusText || '请求失败')
  return payload.data
}

export async function upload(path: string, form: FormData): Promise<unknown> {
  const res = await fetch(`/api/v1${path}`, { method: 'POST', body: form, credentials: 'include' })
  const payload = await parsePayload<unknown>(res)
  if (!res.ok || !payload.ok) throw new Error(payload.error?.message || res.statusText || '上传失败')
  return payload.data
}
