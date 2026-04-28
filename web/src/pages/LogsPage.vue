<template>
  <div class="card p-5">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold">实时日志</h1>
        <p class="text-sm text-neutral-500">显示 DHCP、ProxyDHCP、TFTP、HTTP Boot 和系统事件，适合 PXE 调试时观察客户端请求。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button class="btn" @click="paused = !paused">{{ paused ? '继续滚动' : '暂停滚动' }}</button>
        <button class="btn" :disabled="loading" @click="load()">{{ loading ? '同步中...' : '立即同步' }}</button>
      </div>
    </div>
    <p v-if="error" class="mt-3 text-sm text-red-600">{{ error }}</p>
    <div class="mt-4 rounded-md border bg-white">
      <div class="flex flex-col gap-1 border-b px-3 py-2 text-xs text-neutral-500 sm:flex-row sm:items-center sm:justify-between">
        <span>自动同步中，实时事件即时显示，历史日志每 3 秒补齐。</span>
        <span>{{ paused ? '已暂停自动滚动' : '显示最新日志' }}</span>
      </div>
      <div ref="eventBox" class="max-h-[72vh] overflow-auto">
        <div v-for="line in visibleLines" :key="line.id" class="grid gap-1 border-b px-3 py-2 text-xs sm:grid-cols-[10rem_5.5rem_1fr] sm:gap-3">
          <span class="text-neutral-500">{{ line.time }}</span>
          <span :class="levelClass(line.level)">{{ line.source }}</span>
          <span class="min-w-0 break-words text-neutral-800">{{ line.message }}</span>
        </div>
        <div v-if="visibleLines.length === 0" class="p-6 text-sm text-neutral-500">暂无日志。启动 PXE 服务后，客户端 DHCP 请求会显示在这里。</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../lib/api'

const logs = ref<string[]>([])
const liveEvents = ref<any[]>([])
const loading = ref(false)
const error = ref('')
const paused = ref(false)
const eventBox = ref<HTMLElement>()

let es: EventSource | null = null
let retryTimer: number | undefined
let refreshTimer: number | undefined

const visibleLines = computed(() => {
  const live = liveEvents.value.map((event, index) => ({
    id: `live-${event.time}-${event.source}-${event.message}-${index}`,
    time: shortTime(event.time),
    level: event.level ?? 'info',
    source: event.source ?? 'system',
    message: event.message ?? ''
  }))
  const history = logs.value.map((line, index) => parseLogLine(line, index))
  const seen = new Set<string>()
  return [...live, ...history].filter((line) => {
    const key = `${line.time}|${line.source}|${line.message}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).slice(0, 400)
})

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  error.value = ''
  try {
    const nextLogs = await api<string[]>('/logs')
    logs.value = Array.isArray(nextLogs) ? nextLogs : []
  } catch (e) {
    error.value = e instanceof Error ? e.message : '日志读取失败'
  } finally {
    if (showLoading) loading.value = false
  }
}

function levelClass(level: string) {
  if (level === 'error') return 'text-red-600 font-medium'
  if (level === 'warning') return 'text-amber-600 font-medium'
  return 'text-neutral-700 font-medium'
}

function shortTime(value: string) {
  if (!value) return ''
  const match = value.match(/T(\d{2}:\d{2}:\d{2})/)
  return match?.[1] ?? value.replace(/^time=/, '').slice(0, 19)
}

function parseLogLine(line: string, index: number) {
  const time = shortTime(line.match(/time=([^\s]+)/)?.[1] ?? '')
  const level = (line.match(/level=([A-Z]+)/)?.[1] ?? 'INFO').toLowerCase()
  const source = line.match(/source=([^\s]+)/)?.[1] ?? 'system'
  const message = line.match(/msg="([^"]+)"/)?.[1] ?? line.match(/msg=([^\s].*?)(?:\s+source=|$)/)?.[1] ?? line
  return { id: `log-${index}-${line}`, time, level, source, message }
}

function connectEvents() {
  es?.close()
  es = new EventSource('/api/v1/events/stream', { withCredentials: true })
  es.onmessage = async (msg) => {
    try {
      const event = JSON.parse(msg.data)
      liveEvents.value.unshift(event)
      liveEvents.value = liveEvents.value.slice(0, 200)
      if (!paused.value) {
        await nextTick()
        if (eventBox.value) eventBox.value.scrollTop = 0
      }
    } catch {
      // heartbeat
    }
  }
  es.onerror = () => {
    es?.close()
    es = null
    if (retryTimer) window.clearTimeout(retryTimer)
    retryTimer = window.setTimeout(connectEvents, 3000)
  }
}

onMounted(() => {
  load()
  connectEvents()
  refreshTimer = window.setInterval(() => load(false), 3000)
})

onUnmounted(() => {
  es?.close()
  if (retryTimer) window.clearTimeout(retryTimer)
  if (refreshTimer) window.clearInterval(refreshTimer)
})
</script>
