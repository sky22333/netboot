<template>
  <div class="space-y-6">
    <section class="grid gap-4 md:grid-cols-4">
      <div v-for="(value, key) in status?.services" :key="key" class="card p-4">
        <div class="text-sm text-neutral-500">{{ labels[key] ?? key }}</div>
        <div class="mt-2 flex items-center gap-2 text-lg font-semibold">
          <span class="h-2.5 w-2.5 rounded-full" :class="value === 'running' ? 'bg-green-500' : 'bg-neutral-300'" />
          {{ value === 'running' ? '运行中' : '已停止' }}
        </div>
      </div>
    </section>
    <section class="card p-5">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 class="text-lg font-semibold">服务控制</h2>
          <p class="mt-1 text-sm text-neutral-500">启动或停止当前已启用的 PXE 服务。</p>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-primary" :disabled="busy" @click="start">{{ busy ? '处理中...' : '启动服务' }}</button>
          <button class="btn" :disabled="busy" @click="stop">停止服务</button>
        </div>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>
    <section class="card p-5">
      <h2 class="text-lg font-semibold">实时事件</h2>
      <div class="mt-4 space-y-2">
        <div v-for="event in events" :key="event.time + event.message" class="rounded-md border border-neutral-200 p-3 text-sm">
          <span class="font-medium">{{ event.source }}</span>
          <span class="mx-2 text-neutral-400">/</span>
          <span>{{ event.message }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../lib/api'

const labels: Record<string, string> = { dhcp: '完整 DHCP', proxy_dhcp_67: 'ProxyDHCP 发现', proxy_dhcp: 'ProxyDHCP 4011', tftp: 'TFTP', httpboot: 'HTTP Boot', torrent: 'Tracker' }
const status = ref<any>()
const events = ref<any[]>([])
const busy = ref(false)
const message = ref('')
const error = ref(false)
let es: EventSource | null = null
let retryTimer: number | undefined

async function load() {
  try {
    status.value = await api('/status')
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '状态刷新失败'
  }
}
async function start() {
  if (!window.confirm('确认启动已启用的 PXE 服务？完整 DHCP、TFTP、HTTP 可能需要管理员权限并影响当前局域网。')) return
  busy.value = true
  error.value = false
  try {
    status.value = await api('/services/start', { method: 'POST' })
    message.value = '服务启动请求已完成。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '启动失败'
  } finally {
    busy.value = false
  }
}
async function stop() {
  if (!window.confirm('确认停止所有 PXE 服务？正在启动或传输的客户端可能会中断。')) return
  busy.value = true
  error.value = false
  try {
    status.value = await api('/services/stop', { method: 'POST' })
    message.value = '服务已停止。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '停止失败'
  } finally {
    busy.value = false
  }
}
function connectEvents() {
  es?.close()
  es = new EventSource('/api/v1/events/stream', { withCredentials: true })
  es.onmessage = (msg) => {
    try {
      events.value.unshift(JSON.parse(msg.data))
      events.value = events.value.slice(0, 20)
    } catch {
      // 忽略心跳或异常事件，保持页面稳定。
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
  window.addEventListener('pxe-refresh', load)
  connectEvents()
})

onUnmounted(() => {
  window.removeEventListener('pxe-refresh', load)
  es?.close()
  if (retryTimer) window.clearTimeout(retryTimer)
})
</script>
