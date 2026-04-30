<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div class="min-w-0">
          <h1 class="text-lg font-semibold">netboot.xyz</h1>
          <p class="mt-1 text-sm text-neutral-500">下载 PXE 启动文件，并生成本地 iPXE 镜像菜单钩子。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn gap-2" :disabled="busy" @click="load">
            <RefreshCw class="h-4 w-4" />
            刷新
          </button>
          <button class="btn btn-primary gap-2" :disabled="busy || !info" @click="download">
            <Download class="h-4 w-4" />
            {{ busy ? '下载中...' : '下载文件' }}
          </button>
        </div>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>

    <section v-if="info" class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div class="card overflow-hidden">
        <div class="border-b border-neutral-200 p-4">
          <h2 class="font-medium">启动文件</h2>
          <p class="mt-1 break-all text-xs text-neutral-500">来源：{{ info.base_url }}</p>
        </div>
        <div class="grid gap-3 p-4 md:grid-cols-3">
          <div v-for="item in localFiles" :key="item.file" class="rounded-md border border-neutral-200 p-3 text-sm">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="truncate font-medium" :title="item.file">{{ item.file }}</div>
                <div class="mt-1 text-xs" :class="item.exists ? 'text-green-700' : 'text-neutral-500'">{{ item.exists ? '已下载' : '未下载' }}</div>
              </div>
              <CheckCircle2 v-if="item.exists" class="h-4 w-4 shrink-0 text-green-600" />
              <CircleDashed v-else class="h-4 w-4 shrink-0 text-neutral-400" />
            </div>
            <div v-if="item.exists" class="mt-2 text-xs text-neutral-500">{{ formatSize(item.size) }} · {{ formatTime(item.mod_time) }}</div>
            <div class="mt-2 break-all text-xs text-neutral-500">{{ item.path }}</div>
          </div>
        </div>
      </div>

      <aside class="space-y-4">
        <section class="card p-4">
          <h2 class="font-medium">保存位置</h2>
          <div class="mt-3 space-y-3 text-sm">
            <div>
              <div class="text-xs text-neutral-500">下载目录</div>
              <div class="mt-1 break-all font-medium">{{ info.download_dir }}</div>
            </div>
            <div v-if="info.local_vars">
              <div class="text-xs text-neutral-500">local-vars.ipxe</div>
              <div class="mt-1 break-all font-medium">{{ info.local_vars.path }}</div>
              <div class="mt-1 text-xs" :class="info.local_vars.exists ? 'text-green-700' : 'text-neutral-500'">{{ info.local_vars.exists ? '已存在' : '未生成' }}</div>
            </div>
          </div>
        </section>

        <section v-if="resultItems.length || localVarsResult" class="card p-4">
          <h2 class="font-medium">任务结果</h2>
          <div class="mt-3 space-y-2">
            <div v-if="localVarsResult" class="rounded-md border border-neutral-200 p-3 text-xs">
              <div class="font-medium">local-vars.ipxe · {{ localVarsResult.error ? localVarsResult.error : (localVarsResult.created ? '已生成' : '已存在') }}</div>
              <div class="mt-1 break-all text-neutral-500">{{ localVarsResult.path }}</div>
            </div>
            <div v-for="r in resultItems" :key="r.file" class="rounded-md border border-neutral-200 p-3 text-xs">
              <div class="font-medium" :class="r.ok ? 'text-green-700' : 'text-red-700'">{{ r.file }} · {{ r.ok ? (r.existing ? '已存在' : '下载完成') : r.error }}</div>
              <div v-if="r.sha256" class="mt-1 break-all text-neutral-500">SHA256：{{ r.sha256 }}</div>
              <div class="mt-1 break-all text-neutral-500">{{ r.target_path }}</div>
            </div>
          </div>
        </section>
      </aside>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CheckCircle2, CircleDashed, Download, RefreshCw } from 'lucide-vue-next'
import { api } from '../lib/api'

type LocalFile = { file: string; path: string; exists: boolean; size?: number; mod_time?: string }
type NetbootInfo = { base_url: string; download_dir: string; files: string[]; local: LocalFile[]; local_vars?: LocalFile }
type DownloadResult = { file: string; ok: boolean; existing?: boolean; error?: string; sha256?: string; target_path: string }
type LocalVarsResult = { path: string; created: boolean; error?: string }

const info = ref<NetbootInfo | null>(null)
const results = ref<DownloadResult[]>([])
const localVarsResult = ref<LocalVarsResult | null>(null)
const busy = ref(false)
const message = ref('')
const error = ref(false)
const localFiles = computed(() => Array.isArray(info.value?.local) ? info.value.local : [])
const resultItems = computed(() => Array.isArray(results.value) ? results.value : [])

async function load() {
  busy.value = true
  error.value = false
  try {
    info.value = await api<NetbootInfo>('/netbootxyz/files')
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '读取失败'
  } finally {
    busy.value = false
  }
}

async function download() {
  if (!info.value) return
  const files = info.value.files?.join(', ') ?? ''
  if (!window.confirm(`确认从 ${info.value.base_url} 下载以下文件？\n${files}`)) return
  busy.value = true
  error.value = false
  try {
    const res = await api<{ downloads: DownloadResult[]; local_vars: LocalVarsResult }>('/netbootxyz/download', { method: 'POST' })
    results.value = Array.isArray(res?.downloads) ? res.downloads : []
    localVarsResult.value = res?.local_vars ?? null
    message.value = '下载任务已完成。'
    await load()
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '下载失败'
  } finally {
    busy.value = false
  }
}

function formatSize(size?: number) {
  if (!Number.isFinite(size)) return '-'
  const value = Number(size)
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

function formatTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', { hour12: false })
}

onMounted(load)
</script>
