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
  if (res.status === 401) window.dispatchEvent(new Event('pxe-auth-expired'))
  const payload = await parsePayload<T>(res)
  if (!res.ok || !payload.ok) throw new Error(payload.error?.message || res.statusText || '请求失败')
  return payload.data
}

export function upload(path: string, file: File, signal: AbortSignal, onProgress: (percent: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const abort = () => xhr.abort()
    xhr.open('POST', `/api/v1${path}`)
    xhr.withCredentials = true
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.upload.onprogress = event => {
      if (event.lengthComputable) onProgress(Math.round(event.loaded / event.total * 100))
    }
    xhr.onload = () => {
      if (xhr.status === 401) window.dispatchEvent(new Event('pxe-auth-expired'))
      try {
        const payload = JSON.parse(xhr.responseText) as ApiResponse<unknown>
        if (xhr.status >= 200 && xhr.status < 300 && payload.ok) resolve()
        else reject(new Error(payload.error?.message || `上传失败（HTTP ${xhr.status}）`))
      } catch { reject(new Error(`上传响应无效（HTTP ${xhr.status}）`)) }
    }
    xhr.onerror = () => reject(new Error('网络中断，上传未确认完成，请刷新目录检查后重试'))
    xhr.onabort = () => reject(new Error('已取消上传'))
    xhr.onloadend = () => signal.removeEventListener('abort', abort)
    if (signal.aborted) { reject(new Error('已取消上传')); return }
    signal.addEventListener('abort', abort, { once: true })
    xhr.send(file)
  })
}
