<template>
  <div class="space-y-4">
    <div class="card p-5">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-lg font-semibold">启动文件管理</h1>
          <p class="mt-1 text-sm text-neutral-500">管理 PXE 启动文件。HTTP Boot 对应 data/boot/http，TFTP 启动对应 data/boot/tftp。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn" :class="root === 'http' ? 'btn-primary' : ''" @click="switchRoot('http')">HTTP Boot</button>
          <button class="btn" :class="root === 'tftp' ? 'btn-primary' : ''" @click="switchRoot('tftp')">TFTP 启动</button>
          <button class="btn" :disabled="busy" @click="load">刷新</button>
        </div>
      </div>

      <div class="mt-4 grid gap-3 lg:grid-cols-3">
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-sm font-medium">HTTP Boot 目录</div>
          <p class="mt-1 text-xs text-neutral-500">对应 data/boot/http。放 boot.ipxe、linux、initrd.gz、ISO/WIM 等，通过 http://通告IP/文件名 访问。</p>
        </div>
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-sm font-medium">TFTP 启动目录</div>
          <p class="mt-1 text-xs text-neutral-500">对应 data/boot/tftp。放 ipxe.bios、ipxe.efi 等第一阶段启动文件，通过 TFTP 加载。</p>
        </div>
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-sm font-medium">netboot.xyz 文件</div>
          <p class="mt-1 text-xs text-neutral-500">对应 data/boot/netboot。推荐在 netboot.xyz 页面下载，BIOS/UEFI 会按规则优先使用。</p>
        </div>
      </div>
    </div>

    <div class="grid gap-4 xl:grid-cols-[1fr_360px]">
      <div class="card overflow-hidden">
        <div class="flex flex-col gap-3 border-b border-neutral-200 p-4 md:flex-row md:items-center md:justify-between">
          <div>
            <div class="text-sm text-neutral-500">当前位置 · {{ rootDescription }}</div>
            <div class="mt-1 flex flex-wrap items-center gap-1 text-sm">
              <button class="rounded px-2 py-1 font-medium hover:bg-neutral-100" @click="goPath('.')">{{ rootLabel }}</button>
              <template v-for="crumb in crumbs" :key="crumb.path">
                <span class="text-neutral-400">/</span>
                <button class="rounded px-2 py-1 hover:bg-neutral-100" @click="goPath(crumb.path)">{{ crumb.name }}</button>
              </template>
            </div>
          </div>
          <label class="btn cursor-pointer">
            上传文件
            <input class="hidden" type="file" :disabled="busy" @change="onFile" />
          </label>
        </div>

        <div class="divide-y divide-neutral-100">
          <button v-if="currentPath !== '.'" class="flex w-full items-center justify-between p-3 text-left text-sm hover:bg-neutral-50" @click="goUp">
            <span class="font-medium">返回上一级</span>
            <span class="text-neutral-400">..</span>
          </button>
          <button v-for="f in sortedFiles" :key="f.name" class="flex w-full items-center justify-between gap-3 p-3 text-left text-sm hover:bg-neutral-50" :class="selectedPath === f.name ? 'bg-neutral-100' : ''" @click="selectFile(f)">
            <div class="min-w-0">
              <div class="truncate font-medium">{{ f.dir ? '目录' : filePurpose(f.name) }} · {{ f.name }}</div>
              <div class="mt-1 text-xs text-neutral-500">{{ f.dir ? '点击进入目录' : fileHint(f.name) }}</div>
            </div>
            <div class="shrink-0 text-xs text-neutral-500">{{ f.dir ? '目录' : formatSize(f.size) }}</div>
          </button>
          <div v-if="sortedFiles.length === 0" class="p-8 text-center text-sm text-neutral-500">
            当前目录为空。可以先上传文件，或在右侧创建子目录。
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card p-4">
          <h2 class="font-medium">新建目录</h2>
          <p class="mt-1 text-xs text-neutral-500">在当前位置创建子目录，用于按系统、镜像或项目整理文件。</p>
          <div class="mt-3 flex gap-2">
            <input v-model="newDir" class="input min-w-0 flex-1" placeholder="目录名" />
            <button class="btn" :disabled="busy || !newDir" @click="mkdir">创建</button>
          </div>
        </div>

        <div class="card p-4">
          <h2 class="font-medium">已选择</h2>
          <p class="mt-1 break-all text-sm text-neutral-600">{{ selectedPath || '点击左侧文件后，可重命名、移动、删除或制作种子。' }}</p>
          <div class="mt-3 space-y-2">
            <input v-model="renameTo" class="input w-full" placeholder="新名称或目标路径" />
            <div class="grid grid-cols-2 gap-2">
              <button class="btn" :disabled="busy || !selectedPath || !renameTo" @click="rename">重命名/移动</button>
              <button class="btn btn-danger" :disabled="busy || !selectedPath" @click="remove(selectedPath)">删除</button>
            </div>
            <button class="btn w-full" :disabled="busy || root !== 'http' || !selectedPath" @click="makeTorrent">为 HTTP 文件制作种子</button>
          </div>
        </div>

        <p v-if="message" class="rounded-md border p-3 text-sm" :class="error ? 'border-red-200 bg-red-50 text-red-700' : 'border-neutral-200 bg-white text-neutral-600'">{{ message }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, upload } from '../lib/api'

const root = ref('http')
const currentPath = ref('.')
const files = ref<any[]>([])
const newDir = ref('')
const selectedPath = ref('')
const renameTo = ref('')
const message = ref('')
const error = ref(false)
const busy = ref(false)

const rootLabel = computed(() => root.value === 'http' ? 'HTTP Boot' : 'TFTP 启动')
const rootDescription = computed(() => root.value === 'http' ? 'data/boot/http' : 'data/boot/tftp')
const sortedFiles = computed(() => [...files.value].sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name)))
const crumbs = computed(() => {
  if (currentPath.value === '.') return []
  const parts = currentPath.value.split('/').filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
})

async function load() {
  await run(async () => {
    const res: any = await api(`/files?root=${root.value}&path=${encodeURIComponent(currentPath.value)}`)
    files.value = Array.isArray(res.files) ? res.files : []
    selectedPath.value = ''
    renameTo.value = ''
    message.value = files.value.length === 0 ? '当前目录为空' : `已加载 ${files.value.length} 个条目`
  })
}
async function run(task: () => Promise<void>) {
  busy.value = true
  error.value = false
  try { await task() } catch (e) { error.value = true; message.value = e instanceof Error ? e.message : '操作失败' } finally { busy.value = false }
}
function switchRoot(value: string) {
  root.value = value
  currentPath.value = '.'
  load()
}
function fullPath(name: string) {
  return currentPath.value === '.' ? name : `${currentPath.value}/${name}`
}
function goPath(path: string) {
  currentPath.value = path || '.'
  load()
}
function goUp() {
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  goPath(parts.join('/') || '.')
}
function selectFile(file: any) {
  if (file.dir) {
    goPath(fullPath(file.name))
    return
  }
  selectedPath.value = fullPath(file.name)
  renameTo.value = selectedPath.value
}
async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files?.[0]) return
  await run(async () => {
    const form = new FormData()
    form.append('root', root.value)
    form.append('path', currentPath.value)
    form.append('file', input.files![0])
    await upload('/files/upload', form)
    message.value = '文件已上传'
    input.value = ''
    await load()
  })
}
async function mkdir() {
  await run(async () => {
    await api('/files/mkdir', { method: 'POST', body: JSON.stringify({ root: root.value, path: fullPath(newDir.value) }) })
    newDir.value = ''
    message.value = '目录已创建'
    await load()
  })
}
async function rename() {
  if (!window.confirm(`确认将 ${selectedPath.value} 重命名或移动到 ${renameTo.value}？`)) return
  await run(async () => { await api('/files/rename', { method: 'POST', body: JSON.stringify({ root: root.value, from: selectedPath.value, to: renameTo.value }) }); message.value = '已重命名'; await load() })
}
async function remove(path: string) {
  if (!window.confirm(`确认删除 ${path}？此操作不可恢复。`)) return
  await run(async () => { await api(`/files?root=${root.value}&path=${encodeURIComponent(path)}`, { method: 'DELETE' }); message.value = '文件已删除'; await load() })
}
async function makeTorrent() { await run(async () => { const r: any = await api('/files/torrent', { method: 'POST', body: JSON.stringify({ root: root.value, path: selectedPath.value }) }); message.value = `种子已创建：${r.torrent_path}`; await load() }) }
function filePurpose(name: string) {
  const ext = name.split('.').pop()?.toLowerCase()
  if (ext === 'efi') return 'UEFI 固件'
  if (ext === 'kpxe' || ext === 'pxe' || ext === 'bios') return 'PXE 固件'
  if (ext === 'ipxe') return 'iPXE 脚本'
  if (ext === 'wim') return 'WIM 镜像'
  if (ext === 'iso') return 'ISO 镜像'
  if (ext === 'vhd' || ext === 'vhdx') return '虚拟磁盘'
  return '文件'
}
function fileHint(name: string) {
  const ext = name.split('.').pop()?.toLowerCase()
  if (ext === 'ipxe') return '可作为 iPXE 菜单或链式启动脚本'
  if (['efi', 'kpxe', 'pxe', 'bios'].includes(ext ?? '')) return '通常用于 DHCP 返回的第一阶段启动文件'
  if (['iso', 'wim', 'vhd', 'vhdx'].includes(ext ?? '')) return '建议放在 HTTP Boot 目录供 iPXE 高速读取'
  return '可在启动菜单或脚本中引用'
}
function formatSize(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MiB`
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GiB`
}
onMounted(load)
</script>
