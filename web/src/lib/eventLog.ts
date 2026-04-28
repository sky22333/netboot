import { computed, ref } from 'vue'
import { api } from './api'

export type LogEvent = {
  id: string
  time: string
  level: string
  source: string
  message: string
}

const maxEvents = 1000
const events = ref<LogEvent[]>([])
const connected = ref(false)
const loading = ref(false)
const error = ref('')
let source: EventSource | null = null
let retryTimer: number | undefined

export function useEventLog() {
  const latest = computed(() => events.value)
  const recent = computed(() => events.value.slice(-20))

  async function load(limit = 500) {
    loading.value = true
    error.value = ''
    try {
      const rows = await api<any[]>(`/logs?limit=${limit}`)
      merge(Array.isArray(rows) ? rows.map(normalizeEvent) : [])
    } catch (e) {
      error.value = e instanceof Error ? e.message : '日志读取失败'
    } finally {
      loading.value = false
    }
  }

  function connect() {
    if (source) return
    source = new EventSource('/api/v1/events/stream', { withCredentials: true })
    source.onopen = () => {
      connected.value = true
      error.value = ''
    }
    source.onmessage = (message) => {
      try {
        merge([normalizeEvent(JSON.parse(message.data))])
      } catch {
        // ignore malformed messages; SSE heartbeats are comments and do not arrive here.
      }
    }
    source.onerror = () => {
      connected.value = false
      source?.close()
      source = null
      if (retryTimer) window.clearTimeout(retryTimer)
      retryTimer = window.setTimeout(connect, 3000)
    }
  }

  function disconnect() {
    source?.close()
    source = null
    connected.value = false
    if (retryTimer) window.clearTimeout(retryTimer)
  }

  return { events: latest, recent, connected, loading, error, load, connect, disconnect }
}

function normalizeEvent(raw: any): LogEvent {
  const id = Number(raw?.id ?? 0)
  const origin = raw?.ts ? 'db' : 'live'
  return {
    id: Number.isFinite(id) && id > 0 ? `${origin}-${id}` : `${origin}-${fallbackID(raw)}`,
    time: raw?.time ?? raw?.ts ?? '',
    level: String(raw?.level ?? 'info').toLowerCase(),
    source: raw?.source ?? 'system',
    message: raw?.message ?? ''
  }
}

function merge(next: LogEvent[]) {
  if (next.length === 0) return
  const byID = new Map<string, LogEvent>()
  for (const item of events.value) byID.set(item.id, item)
  for (const item of next) byID.set(item.id, item)
  events.value = [...byID.values()].sort(compareEvent).slice(-maxEvents)
}

function compareEvent(a: LogEvent, b: LogEvent) {
  const at = Date.parse(a.time)
  const bt = Date.parse(b.time)
  if (Number.isFinite(at) && Number.isFinite(bt) && at !== bt) return at - bt
  return a.id.localeCompare(b.id)
}

function fallbackID(raw: any) {
  const text = `${raw?.time ?? raw?.ts ?? ''}|${raw?.source ?? ''}|${raw?.message ?? ''}`
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return Math.abs(hash)
}
