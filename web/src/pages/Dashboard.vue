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
          <button class="btn btn-primary" :disabled="!canStart" @click="start">{{ busy ? '处理中...' : anyRunning ? '已启动' : '启动服务' }}</button>
          <button class="btn" :disabled="!canStop" @click="stop">{{ busy ? '处理中...' : anyRunning ? '停止服务' : '已停止' }}</button>
        </div>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>
    <section class="card p-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="font-semibold">实时事件</h2>
          <p class="mt-0.5 text-xs text-neutral-500">最近事件摘要，完整内容见日志页面。</p>
        </div>
        <span class="shrink-0 text-xs text-neutral-500">{{ connected ? '实时连接正常' : '连接重试中' }}</span>
      </div>
      <div class="mt-3 divide-y divide-neutral-100 rounded-md border border-neutral-200">
        <div v-for="event in compactEvents" :key="event.id" class="grid min-h-8 grid-cols-[4.8rem_5.2rem_1fr] items-center gap-2 px-2.5 py-1.5 text-xs">
          <span class="text-neutral-500">{{ shortTime(event.time) }}</span>
          <span class="truncate font-medium text-neutral-700">{{ event.source }}</span>
          <span class="truncate text-neutral-700" :title="event.message">{{ event.message }}</span>
        </div>
        <div v-if="compactEvents.length === 0" class="px-2.5 py-2 text-xs text-neutral-500">暂无事件。</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../lib/api'
import { useEventLog } from '../lib/eventLog'

const labels: Record<string, string> = { dhcp: '完整 DHCP', proxy_dhcp_67: 'ProxyDHCP 发现', proxy_dhcp: 'ProxyDHCP 4011', tftp: 'TFTP', httpboot: 'HTTP Boot', torrent: 'Tracker', smb: 'SMB 共享' }
const status = ref<any>()
const { recent, connected, load: loadEvents, connect: connectEvents } = useEventLog()
const compactEvents = computed(() => recent.value.slice(-6))
const busy = ref(false)
const message = ref('')
const error = ref(false)
const services = computed<Record<string, string>>(() => status.value?.services ?? {})
const anyRunning = computed(() => Object.values(services.value).some((value) => value === 'running'))
const canStart = computed(() => !busy.value && !anyRunning.value)
const canStop = computed(() => !busy.value && anyRunning.value)

async function load() {
  try {
    status.value = await api('/status')
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '状态刷新失败'
  }
}
async function start() {
  if (!canStart.value) return
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
  if (!canStop.value) return
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
function shortTime(value: string) {
  const match = value.match(/T(\d{2}:\d{2}:\d{2})/)
  return match?.[1] ?? value.slice(0, 19)
}

onMounted(() => {
  refreshAll()
  window.addEventListener('pxe-refresh', refreshAll)
  connectEvents()
})

onUnmounted(() => {
  window.removeEventListener('pxe-refresh', refreshAll)
})

function refreshAll() {
  load()
  loadEvents(200)
}
</script>
