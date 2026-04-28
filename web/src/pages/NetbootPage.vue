<template>
  <div class="card p-5">
    <div class="flex items-center justify-between">
      <div><h1 class="text-lg font-semibold">netboot.xyz</h1><p class="text-sm text-neutral-500">从官方地址下载常用启动文件。</p></div>
      <button class="btn btn-primary" :disabled="busy" @click="download">{{ busy ? '下载中...' : '下载文件' }}</button>
    </div>
    <div v-if="info" class="mt-4 rounded-md bg-neutral-50 p-4 text-sm">
      <div>来源：{{ info.base_url }}</div>
      <div>保存目录：{{ info.download_dir }}</div>
      <div>文件：{{ info.files?.join(', ') }}</div>
    </div>
    <div v-if="localFiles.length" class="mt-4 grid gap-2 md:grid-cols-3">
      <div v-for="item in localFiles" :key="item.file" class="rounded-md border p-3 text-sm">
        <div class="font-medium">{{ item.file }}</div>
        <div class="mt-1" :class="item.exists ? 'text-green-700' : 'text-neutral-500'">{{ item.exists ? '已存在' : '未下载' }}</div>
        <div v-if="item.exists" class="mt-1 text-xs text-neutral-500">{{ item.size }} B</div>
        <div class="mt-1 break-all text-xs text-neutral-500">{{ item.path }}</div>
      </div>
    </div>
    <div class="mt-4 space-y-2">
      <div v-for="r in results" :key="r.file" class="rounded-md border p-3 text-sm">
        <div class="font-medium">{{ r.file }} - {{ r.ok ? (r.existing ? '已存在，已跳过' : '下载完成') : r.error }}</div>
        <div v-if="r.sha256" class="mt-1 break-all text-xs text-neutral-500">SHA256：{{ r.sha256 }}</div>
        <div class="mt-1 break-all text-xs text-neutral-500">{{ r.target_path }}</div>
      </div>
    </div>
    <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../lib/api'
const info = ref<any>()
const results = ref<any[]>([])
const busy = ref(false)
const message = ref('')
const error = ref(false)
const localFiles = computed(() => Array.isArray(info.value?.local) ? info.value.local : [])
async function load() { info.value = await api('/netbootxyz/files') }
async function download() {
  const files = info.value?.files?.join(', ') ?? ''
  if (!window.confirm(`确认从 ${info.value?.base_url} 下载以下文件？\n${files}`)) return
  busy.value = true
  error.value = false
  try {
    const rows = await api<any[]>('/netbootxyz/download', { method: 'POST' })
    results.value = Array.isArray(rows) ? rows : []
    message.value = '下载任务已完成。'
    await load()
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '下载失败'
  } finally {
    busy.value = false
  }
}
onMounted(load)
</script>
