<template>
  <div class="space-y-4">
    <div class="card p-5">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h1 class="text-lg font-semibold">启动文件管理</h1>
          <p class="mt-1 text-sm text-neutral-500">管理 HTTP Boot、TFTP 和 netboot.xyz 目录中的启动脚本、固件和镜像文件。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button v-for="item in roots" :key="item.key" class="btn gap-2" :class="root === item.key ? 'btn-primary' : ''" @click="switchRoot(item.key)">
            <component :is="item.icon" class="h-4 w-4" />
            {{ item.label }}
          </button>
          <button class="btn gap-2" :disabled="busy" @click="load">
            <RefreshCw class="h-4 w-4" />
            刷新
          </button>
        </div>
      </div>

      <div class="mt-4 rounded-md border border-neutral-200 bg-neutral-50 p-3">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="min-w-0">
            <div class="text-sm font-medium">{{ activeRoot.label }}</div>
            <p class="mt-1 break-all text-xs text-neutral-500">{{ activeRoot.description }}</p>
            <p class="mt-1 break-all text-xs text-neutral-500">本地目录：{{ basePath || activeRoot.localPath }}</p>
          </div>
          <button class="btn shrink-0 gap-2" :disabled="!accessExample" @click="copyText(accessExample)">
            <Copy class="h-4 w-4" />
            复制访问示例
          </button>
        </div>
      </div>
    </div>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
      <div class="card overflow-hidden">
        <div class="flex flex-col gap-3 border-b border-neutral-200 p-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="min-w-0">
            <div class="text-xs text-neutral-500">当前位置</div>
            <div class="mt-1 flex flex-wrap items-center gap-1 text-sm">
              <button class="rounded px-2 py-1 font-medium hover:bg-neutral-100" @click="goPath('.')">{{ activeRoot.label }}</button>
              <template v-for="crumb in crumbs" :key="crumb.path">
                <ChevronRight class="h-3.5 w-3.5 text-neutral-400" />
                <button class="rounded px-2 py-1 hover:bg-neutral-100" @click="goPath(crumb.path)">{{ crumb.name }}</button>
              </template>
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn gap-2" :disabled="busy" @click="openCreateDir">
              <FolderPlus class="h-4 w-4" />
              新建目录
            </button>
            <button class="btn gap-2" :disabled="busy" @click="openCreateFile">
              <FilePlus2 class="h-4 w-4" />
              新建文件
            </button>
            <label class="btn cursor-pointer gap-2" :class="busy ? 'opacity-50' : ''">
              <Upload class="h-4 w-4" />
              上传文件
              <input class="hidden" type="file" :disabled="busy" @change="onFile" />
            </label>
          </div>
        </div>

        <div class="hidden overflow-x-auto md:block">
          <table class="w-full table-fixed text-sm">
            <thead class="border-b border-neutral-200 bg-neutral-50 text-left text-xs font-medium text-neutral-500">
              <tr>
                <th class="w-[42%] px-4 py-3">名称</th>
                <th class="w-[14%] px-4 py-3">类型</th>
                <th class="w-[14%] px-4 py-3">大小</th>
                <th class="w-[18%] px-4 py-3">修改时间</th>
                <th class="w-[12%] px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-neutral-100">
              <tr v-if="currentPath !== '.'" class="hover:bg-neutral-50">
                <td class="px-4 py-3" colspan="5">
                  <button class="flex items-center gap-2 text-sm font-medium" @click="goUp">
                    <CornerUpLeft class="h-4 w-4" />
                    返回上一级
                  </button>
                </td>
              </tr>
              <tr v-for="file in sortedFiles" :key="file.name" class="hover:bg-neutral-50" :class="selected?.name === file.name ? 'bg-neutral-50' : ''">
                <td class="min-w-0 px-4 py-3">
                  <button class="flex max-w-full items-center gap-2" @click="selectFile(file)">
                    <Folder v-if="file.dir" class="h-4 w-4 shrink-0 text-amber-600" />
                    <FileText v-else class="h-4 w-4 shrink-0 text-neutral-500" />
                    <span class="truncate font-medium">{{ file.name }}</span>
                  </button>
                </td>
                <td class="px-4 py-3 text-neutral-500">{{ file.dir ? '目录' : filePurpose(file.name) }}</td>
                <td class="px-4 py-3 text-neutral-500">{{ file.dir ? '-' : formatSize(file.size) }}</td>
                <td class="px-4 py-3 text-neutral-500">{{ formatTime(file.mod_time) }}</td>
                <td class="px-4 py-3">
                  <div class="flex justify-end gap-1">
                    <button class="btn h-8 px-2" :disabled="file.dir || !file.editable" title="编辑" @click.stop="editFile(file)">
                      <Pencil class="h-4 w-4" />
                    </button>
                    <button class="btn h-8 px-2" title="更多" @click.stop="selectFile(file)">
                      <Info class="h-4 w-4" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="divide-y divide-neutral-100 md:hidden">
          <button v-if="currentPath !== '.'" class="flex w-full items-center gap-2 p-4 text-left text-sm font-medium" @click="goUp">
            <CornerUpLeft class="h-4 w-4" />
            返回上一级
          </button>
          <button v-for="file in sortedFiles" :key="file.name" class="flex w-full items-start justify-between gap-3 p-4 text-left text-sm hover:bg-neutral-50" @click="selectFile(file)">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <Folder v-if="file.dir" class="h-4 w-4 shrink-0 text-amber-600" />
                <FileText v-else class="h-4 w-4 shrink-0 text-neutral-500" />
                <span class="truncate font-medium">{{ file.name }}</span>
              </div>
              <div class="mt-1 text-xs text-neutral-500">{{ file.dir ? '目录' : `${filePurpose(file.name)} · ${formatSize(file.size)}` }}</div>
            </div>
            <ChevronRight class="mt-0.5 h-4 w-4 shrink-0 text-neutral-400" />
          </button>
        </div>

        <div v-if="sortedFiles.length === 0" class="p-10 text-center text-sm text-neutral-500">
          当前目录为空，可以上传文件或新建目录。
        </div>
      </div>

      <aside class="space-y-4">
        <div class="card p-4">
          <div class="flex items-center justify-between gap-3">
            <h2 class="font-medium">文件详情</h2>
            <span class="rounded bg-neutral-100 px-2 py-1 text-xs text-neutral-500">{{ selected ? '已选择' : '未选择' }}</span>
          </div>
          <div v-if="selected" class="mt-4 space-y-3 text-sm">
            <div>
              <div class="text-xs text-neutral-500">相对路径</div>
              <div class="mt-1 break-all font-medium">{{ selectedFullPath }}</div>
            </div>
            <div>
              <div class="text-xs text-neutral-500">访问路径</div>
              <div class="mt-1 break-all rounded-md bg-neutral-50 p-2 text-xs">{{ selectedAccessPath }}</div>
            </div>
            <div class="grid grid-cols-2 gap-2">
              <button class="btn gap-2" :disabled="!selectedAccessPath" @click="copyText(selectedAccessPath)">
                <Copy class="h-4 w-4" />
                复制路径
              </button>
              <button class="btn gap-2" :disabled="selected.dir || !selected.editable" @click="editFile(selected)">
                <Pencil class="h-4 w-4" />
                编辑
              </button>
              <button class="btn gap-2" :disabled="selected.dir" @click="startRename">
                <MoveRight class="h-4 w-4" />
                重命名
              </button>
              <button class="btn btn-danger gap-2" @click="remove(selectedFullPath)">
                <Trash2 class="h-4 w-4" />
                删除
              </button>
            </div>
            <button class="btn w-full gap-2" :disabled="root !== 'http' || selected.dir" @click="makeTorrent">
              <Share2 class="h-4 w-4" />
              为 HTTP 文件制作种子
            </button>
          </div>
          <p v-else class="mt-4 text-sm text-neutral-500">选择文件后，可查看访问路径、在线编辑文本脚本、重命名、删除或制作种子。</p>
        </div>

        <p v-if="message" class="rounded-md border p-3 text-sm" :class="error ? 'border-red-200 bg-red-50 text-red-700' : 'border-neutral-200 bg-white text-neutral-600'">{{ message }}</p>
      </aside>
    </div>

    <div v-if="editorOpen && editing" class="fixed inset-0 z-50 flex bg-black/35 p-3 sm:p-6">
      <div class="mx-auto flex h-full w-full max-w-6xl flex-col overflow-hidden rounded-lg border border-neutral-200 bg-white shadow-2xl">
        <div class="flex flex-col gap-3 border-b border-neutral-200 bg-white p-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <Pencil class="h-4 w-4 text-neutral-500" />
              <h2 class="font-medium">在线编辑</h2>
              <span v-if="dirty" class="rounded bg-amber-100 px-2 py-1 text-xs text-amber-800">未保存</span>
            </div>
            <p class="mt-1 break-all text-xs text-neutral-500">{{ editingPath }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn btn-primary gap-2" :disabled="busy || !dirty" @click="saveContent">
              <Save class="h-4 w-4" />
              保存
            </button>
            <button class="btn gap-2" :disabled="busy" @click="closeEditorWindow">
              <X class="h-4 w-4" />
              关闭
            </button>
          </div>
        </div>
        <div class="flex items-center justify-between gap-3 border-b border-neutral-100 bg-neutral-50 px-4 py-2 text-xs text-neutral-500">
          <span>{{ editorStats }}</span>
          <span>UTF-8 文本 · 最大 1 MiB · Ctrl/⌘ + S 保存</span>
        </div>
        <textarea
          ref="editorRef"
          v-model="editorContent"
          class="min-h-0 flex-1 resize-none border-0 bg-white p-4 font-mono text-sm leading-6 text-neutral-900 outline-none selection:bg-neutral-900 selection:text-white focus:ring-0"
          spellcheck="false"
          @keydown.ctrl.s.prevent="saveContent"
          @keydown.meta.s.prevent="saveContent"
        />
      </div>
    </div>

    <div v-if="dialog" class="fixed inset-0 z-50 grid place-items-center bg-black/30 p-4">
      <div class="w-full max-w-md rounded-lg bg-white p-5 shadow-xl">
        <h2 class="font-medium">{{ dialogTitle }}</h2>
        <p class="mt-1 text-sm text-neutral-500">{{ dialogHint }}</p>
        <input v-model="dialogValue" class="input mt-4 w-full" :placeholder="dialogPlaceholder" @keyup.enter="confirmDialog" />
        <div class="mt-4 flex justify-end gap-2">
          <button class="btn" @click="dialog = ''">取消</button>
          <button class="btn btn-primary" :disabled="busy || !dialogValue.trim()" @click="confirmDialog">确定</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { ChevronRight, Copy, CornerUpLeft, FilePlus2, FileText, Folder, FolderPlus, Globe2, HardDrive, Info, MoveRight, Pencil, RefreshCw, Save, Share2, Trash2, Upload, X } from 'lucide-vue-next'
import { api, upload } from '../lib/api'
import type { ServiceConfig } from '../lib/types'

type RootKey = 'http' | 'tftp' | 'netboot'

type FileEntry = {
  name: string
  dir: boolean
  size: number
  mod_time: string
  editable?: boolean
}

type FileListResponse = {
  root: RootKey
  path: string
  base_path: string
  files: FileEntry[]
}

const roots = [
  { key: 'http' as RootKey, label: 'HTTP Boot', localPath: 'data/boot/http', icon: Globe2, description: '放 boot.ipxe、linux、initrd.gz、自动安装配置和大镜像，通过 HTTP Boot 服务访问。' },
  { key: 'tftp' as RootKey, label: 'TFTP 启动', localPath: 'data/boot/tftp', icon: HardDrive, description: '放 undionly.kpxe、ipxe-x86_64.efi、ipxe-arm64.efi、local-vars.ipxe 等第一阶段或 TFTP 引导文件。' },
  { key: 'netboot' as RootKey, label: 'netboot.xyz', localPath: 'data/boot/netboot', icon: FileText, description: '存放 netboot.xyz 官方启动文件，BIOS/UEFI 会按规则优先使用。' }
]

const locationStorageKey = 'pxe.files.location'
const initialLocation = loadSavedLocation()

const root = ref<RootKey>(initialLocation.root)
const currentPath = ref(initialLocation.path)
const basePath = ref('')
const files = ref<FileEntry[]>([])
const selected = ref<FileEntry | null>(null)
const message = ref('')
const error = ref(false)
const busy = ref(false)
const dialog = ref<'mkdir' | 'file' | 'rename' | ''>('')
const dialogValue = ref('')
const config = ref<ServiceConfig | null>(null)
const editing = ref(false)
const editorOpen = ref(false)
const editingPath = ref('')
const editorContent = ref('')
const originalContent = ref('')
const editorRef = ref<HTMLTextAreaElement | null>(null)

const activeRoot = computed(() => roots.find(item => item.key === root.value) ?? roots[0])
const sortedFiles = computed(() => [...files.value].sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name, 'zh-Hans-CN')))
const dirty = computed(() => editorContent.value !== originalContent.value)
const editorStats = computed(() => {
  const bytes = new Blob([editorContent.value]).size
  const lines = editorContent.value === '' ? 1 : editorContent.value.split('\n').length
  return `${lines} 行 · ${formatSize(bytes)}`
})
const crumbs = computed(() => {
  if (currentPath.value === '.') return []
  const parts = currentPath.value.split('/').filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
})
const selectedFullPath = computed(() => selected.value ? fullPath(selected.value.name) : '')
const selectedAccessPath = computed(() => selected.value ? accessPath(selectedFullPath.value) : '')
const accessExample = computed(() => {
  if (root.value === 'http') return `${httpBase()}/boot.ipxe`
  if (root.value === 'tftp') return 'undionly.kpxe'
  return `${httpBase()}/netboot/netboot.xyz.kpxe`
})
const dialogTitle = computed(() => {
  if (dialog.value === 'mkdir') return '新建目录'
  if (dialog.value === 'file') return '新建文件'
  return '重命名或移动'
})
const dialogHint = computed(() => {
  if (dialog.value === 'mkdir') return '在当前目录下创建一个子目录。'
  if (dialog.value === 'file') return '创建一个空白文本文件，创建后会自动打开编辑器。'
  return '输入新的文件名，或输入相对路径移动到其他目录。'
})
const dialogPlaceholder = computed(() => {
  if (dialog.value === 'mkdir') return '目录名'
  if (dialog.value === 'file') return '例如 boot.ipxe'
  return '新名称或目标路径'
})

async function load() {
  await run(async () => {
    await fetchFiles(selectedFullPath.value)
    message.value = files.value.length === 0 ? '当前目录为空' : `已加载 ${files.value.length} 个条目`
  })
}

async function fetchFiles(selectPath = '') {
  const res = await api<FileListResponse>(`/files?root=${root.value}&path=${encodeURIComponent(currentPath.value)}`)
  files.value = Array.isArray(res.files) ? res.files : []
  basePath.value = res.base_path || ''
  selected.value = selectPath ? files.value.find(file => fullPath(file.name) === selectPath) ?? null : null
  persistLocation()
}

async function loadConfig() {
  try {
    config.value = await api<ServiceConfig>('/config')
  } catch {
    config.value = null
  }
}

async function run(task: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = false
  try {
    await task()
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '操作失败'
  } finally {
    busy.value = false
  }
}

function switchRoot(value: RootKey) {
  if (root.value === value) return
  root.value = value
  currentPath.value = '.'
  persistLocation()
  closeEditor()
  load()
}

function fullPath(name: string) {
  return currentPath.value === '.' ? name : `${currentPath.value}/${name}`
}

function goPath(path: string) {
  currentPath.value = normalizePath(path)
  persistLocation()
  closeEditor()
  load()
}

function goUp() {
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  goPath(parts.join('/') || '.')
}

function selectFile(file: FileEntry) {
  if (file.dir) {
    goPath(fullPath(file.name))
    return
  }
  selected.value = file
}

function openCreateDir() {
  dialog.value = 'mkdir'
  dialogValue.value = ''
}

function openCreateFile() {
  dialog.value = 'file'
  dialogValue.value = ''
}

function startRename() {
  if (!selected.value) return
  dialog.value = 'rename'
  dialogValue.value = selectedFullPath.value
}

async function confirmDialog() {
  const value = dialogValue.value.trim()
  if (!value) return
  if (dialog.value === 'mkdir') {
    await mkdir(value)
  } else if (dialog.value === 'file') {
    await createFile(value)
  } else if (dialog.value === 'rename') {
    await rename(value)
  }
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
    input.value = ''
    await refreshCurrentDirectory('', '文件已上传')
  })
}

async function mkdir(name: string) {
  await run(async () => {
    await api('/files/mkdir', { method: 'POST', body: JSON.stringify({ root: root.value, path: fullPath(name) }) })
    dialog.value = ''
    await refreshCurrentDirectory('', '目录已创建')
  })
}

async function createFile(name: string) {
  const path = fullPath(name)
  await run(async () => {
    await api('/files/content', { method: 'PUT', body: JSON.stringify({ root: root.value, path, content: '' }) })
    dialog.value = ''
    await refreshCurrentDirectory(path, '文件已创建')
    const created = files.value.find(item => !item.dir && fullPath(item.name) === path)
    if (created?.editable) {
      selected.value = created
      await loadEditorContent(path)
    }
  })
}

async function rename(to: string) {
  if (!selected.value) return
  await run(async () => {
    await api('/files/rename', { method: 'POST', body: JSON.stringify({ root: root.value, from: selectedFullPath.value, to }) })
    dialog.value = ''
    await refreshCurrentDirectory(to, '已重命名')
  })
}

async function remove(path: string) {
  if (!window.confirm(`确认删除 ${path}？此操作不可恢复。`)) return
  await run(async () => {
    await api(`/files?root=${root.value}&path=${encodeURIComponent(path)}`, { method: 'DELETE' })
    closeEditor()
    await refreshCurrentDirectory('', '已删除')
  })
}

async function editFile(file: FileEntry) {
  if (file.dir || !file.editable) return
  selected.value = file
  const path = fullPath(file.name)
  await run(async () => {
    await loadEditorContent(path)
  })
}

async function loadEditorContent(path: string) {
  const res = await api<{ content: string }>(`/files/content?root=${root.value}&path=${encodeURIComponent(path)}`)
  editing.value = true
  editorOpen.value = true
  editingPath.value = path
  editorContent.value = res.content
  originalContent.value = res.content
  message.value = `正在编辑 ${path}`
  await focusEditor()
}

async function saveContent() {
  if (!editing.value || !dirty.value) return
  await run(async () => {
    await api('/files/content', { method: 'PUT', body: JSON.stringify({ root: root.value, path: editingPath.value, content: editorContent.value }) })
    originalContent.value = editorContent.value
    await refreshCurrentDirectory(editingPath.value, '文件已保存')
  })
}

function closeEditor() {
  editing.value = false
  editorOpen.value = false
  editingPath.value = ''
  editorContent.value = ''
  originalContent.value = ''
}

function closeEditorWindow() {
  if (dirty.value && !window.confirm('当前文件有未保存修改，确认关闭编辑器窗口？')) return
  editorOpen.value = false
}

async function focusEditor() {
  await nextTick()
  editorRef.value?.focus()
}

async function makeTorrent() {
  if (!selected.value || root.value !== 'http') return
  await run(async () => {
    const res = await api<{ torrent_path: string }>('/files/torrent', { method: 'POST', body: JSON.stringify({ root: root.value, path: selectedFullPath.value }) })
    await refreshCurrentDirectory('', `种子已创建：${res.torrent_path}`)
  })
}

async function refreshCurrentDirectory(selectPath = '', nextMessage = '') {
  await fetchFiles(selectPath)
  message.value = nextMessage || (files.value.length === 0 ? '当前目录为空' : `已刷新 ${files.value.length} 个条目`)
}

async function copyText(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    message.value = '已复制到剪贴板'
    error.value = false
  } catch {
    message.value = text
    error.value = false
  }
}

function accessPath(path: string) {
  const clean = path.replace(/^\.?\//, '')
  if (root.value === 'http') return `${httpBase()}/${clean}`
  if (root.value === 'netboot') return `${httpBase()}/netboot/${clean}`
  return clean
}

function httpBase() {
  const ip = config.value?.server.advertise_ip || '通告IP'
  const addr = config.value?.httpboot.addr || ':80'
  const port = httpPort(addr)
  return port && port !== '80' ? `http://${ip}:${port}` : `http://${ip}`
}

function httpPort(addr: string) {
  if (!addr) return '80'
  if (addr.startsWith(':')) return addr.slice(1) || '80'
  const match = addr.match(/:(\d+)$/)
  return match?.[1] || '80'
}

function filePurpose(name: string) {
  const ext = extName(name)
  if (ext === 'efi') return 'UEFI 固件'
  if (['kpxe', 'pxe', 'bios'].includes(ext)) return 'PXE 固件'
  if (ext === 'ipxe') return 'iPXE 脚本'
  if (ext === 'wim') return 'WIM 镜像'
  if (ext === 'iso') return 'ISO 镜像'
  if (['vhd', 'vhdx', 'img'].includes(ext)) return '磁盘镜像'
  if (['gz', 'xz', 'zip'].includes(ext)) return '压缩文件'
  return '文件'
}

function extName(name: string) {
  const index = name.lastIndexOf('.')
  return index >= 0 ? name.slice(index + 1).toLowerCase() : ''
}

function loadSavedLocation() {
  try {
    const raw = window.localStorage.getItem(locationStorageKey)
    const saved = raw ? JSON.parse(raw) as { root?: string; path?: string } : {}
    const savedRoot = roots.some(item => item.key === saved.root) ? saved.root as RootKey : 'http'
    return { root: savedRoot, path: normalizePath(saved.path || '.') }
  } catch {
    return { root: 'http' as RootKey, path: '.' }
  }
}

function persistLocation() {
  try {
    window.localStorage.setItem(locationStorageKey, JSON.stringify({ root: root.value, path: currentPath.value }))
  } catch {
    // localStorage may be unavailable in hardened browser profiles; the file manager still works in memory.
  }
}

function normalizePath(path: string) {
  const clean = path.replaceAll('\\', '/').split('/').filter(Boolean).join('/')
  return clean || '.'
}

function formatSize(size: number) {
  if (!Number.isFinite(size)) return '-'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MiB`
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GiB`
}

function formatTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', { hour12: false })
}

onMounted(async () => {
  await loadConfig()
  await load()
})
</script>
