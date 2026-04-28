<template>
  <div class="card p-5">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold">实时日志</h1>
        <p class="text-sm text-neutral-500">按时间从上到下显示，最新日志固定在底部。滚动查看历史时会暂停自动跟随。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button class="btn" @click="toggleFollow">{{ follow ? '暂停跟随' : '回到最新' }}</button>
        <button class="btn" :disabled="loading" @click="refresh">{{ loading ? '同步中...' : '同步历史' }}</button>
      </div>
    </div>
    <p v-if="error" class="mt-3 text-sm text-red-600">{{ error }}</p>
    <div class="mt-4 overflow-hidden rounded-md border bg-white">
      <div class="flex flex-col gap-1 border-b px-3 py-2 text-xs text-neutral-500 sm:flex-row sm:items-center sm:justify-between">
        <span>{{ connected ? '实时连接正常' : '实时连接重试中' }}，最多保留最近 1000 条。</span>
        <span>{{ follow ? '自动跟随最新日志' : '已暂停，点击回到最新' }}</span>
      </div>
      <div ref="eventBox" class="max-h-[72vh] overflow-auto" @scroll="onScroll">
        <div v-for="line in events" :key="line.id" class="grid gap-1 border-b px-3 py-2 text-xs sm:grid-cols-[6rem_5.5rem_1fr] sm:gap-3">
          <span class="text-neutral-500">{{ shortTime(line.time) }}</span>
          <span :class="levelClass(line.level)">{{ line.source }}</span>
          <span class="min-w-0 break-words text-neutral-800">{{ line.message }}</span>
        </div>
        <div v-if="events.length === 0" class="p-6 text-sm text-neutral-500">暂无日志。启动 PXE 服务后，客户端 DHCP、TFTP 和 HTTP 请求会显示在这里。</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useEventLog } from '../lib/eventLog'

const { events, connected, loading, error, load, connect } = useEventLog()
const follow = ref(true)
const eventBox = ref<HTMLElement>()

async function refresh() {
  await load(800)
  await scrollToBottom()
}
function toggleFollow() {
  follow.value = !follow.value
  if (follow.value) scrollToBottom()
}
function onScroll() {
  const box = eventBox.value
  if (!box) return
  const distance = box.scrollHeight - box.scrollTop - box.clientHeight
  follow.value = distance < 40
}
async function scrollToBottom() {
  await nextTick()
  if (eventBox.value) eventBox.value.scrollTop = eventBox.value.scrollHeight
}
function levelClass(level: string) {
  if (level === 'error') return 'text-red-600 font-medium'
  if (level === 'warning') return 'text-amber-600 font-medium'
  return 'text-neutral-700 font-medium'
}
function shortTime(value: string) {
  if (!value) return ''
  const match = value.match(/T(\d{2}:\d{2}:\d{2})/)
  return match?.[1] ?? value.slice(0, 19)
}

watch(() => events.value.length, () => {
  if (follow.value) scrollToBottom()
})

onMounted(async () => {
  await load(800)
  connect()
  await scrollToBottom()
  window.addEventListener('pxe-refresh', refresh)
})
onUnmounted(() => {
  window.removeEventListener('pxe-refresh', refresh)
})
</script>
