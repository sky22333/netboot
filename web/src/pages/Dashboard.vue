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
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">实时事件</h2>
          <p class="mt-1 text-sm text-neutral-500">与日志页面使用同一条实时事件流，最新事件显示在底部。</p>
        </div>
        <span class="text-xs text-neutral-500">{{ connected ? '实时连接正常' : '连接重试中' }}</span>
      </div>
      <div class="mt-4 space-y-2">
        <div v-for="event in recent" :key="event.id" class="rounded-md border border-neutral-200 p-3 text-sm">
          <div class="flex items-center justify-between gap-3">
            <span class="font-medium">{{ event.source }}</span>
            <span class="text-xs text-neutral-500">{{ shortTime(event.time) }}</span>
          </div>
          <div class="mt-1 break-words text-neutral-700">{{ event.message }}</div>
        </div>
        <div v-if="recent.length === 0" class="rounded-md border border-neutral-200 p-4 text-sm text-neutral-500">暂无事件。</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../lib/api'
import { useEventLog } from '../lib/eventLog'

const labels: Record<string, string> = { dhcp: '完整 DHCP', proxy_dhcp_67: 'ProxyDHCP 发现', proxy_dhcp: 'ProxyDHCP 4011', tftp: 'TFTP', httpboot: 'HTTP Boot', torrent: 'Tracker' }
const status = ref<any>()
const { recent, connected, load: loadEvents, connect: connectEvents } = useEventLog()
const busy = ref(false)
const message = ref('')
const error = ref(false)

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
