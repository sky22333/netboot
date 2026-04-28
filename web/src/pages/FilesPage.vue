<template>
  <div class="card p-5">
    <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
      <div>
        <h1 class="text-lg font-semibold">启动文件管理</h1>
        <p class="text-sm text-neutral-500">HTTP 目录用于 iPXE 菜单、WIM/ISO/VHD/netboot 等大文件；TFTP 目录用于传统 PXE 第一阶段加载 ipxe.efi、ipxe.bios。</p>
      </div>
      <div class="flex gap-2">
        <select v-model="root" class="input w-40" @change="load"><option value="http">HTTP Boot</option><option value="tftp">TFTP 启动</option></select>
        <button class="btn" :disabled="busy" @click="load">刷新</button>
      </div>
    </div>
    <div class="mt-4 rounded-md border bg-neutral-50 p-3 text-sm text-neutral-600">
      当前目录：{{ root === 'http' ? 'HTTP Boot 根目录' : 'TFTP 根目录' }}。上传启动资源后，iPXE 菜单会自动扫描 HTTP 目录中的可启动文件。
    </div>
    <div class="mt-4 flex flex-col gap-2 rounded-md border p-3 md:flex-row md:items-center">
      <input type="file" :disabled="busy" @change="onFile" />
      <span class="text-sm text-neutral-500">建议把大镜像放 HTTP 目录，把 PXE 固件放 TFTP 目录。</span>
    </div>
    <div class="mt-4 grid gap-2 md:grid-cols-4">
      <input v-model="newDir" class="input" placeholder="新目录路径" />
      <button class="btn" :disabled="busy" @click="mkdir">新建目录</button>
      <input v-model="selectedPath" class="input" placeholder="选中文件路径" />
      <button class="btn" :disabled="busy || !selectedPath" @click="makeTorrent">制作种子</button>
    </div>
    <div class="mt-2 grid gap-2 md:grid-cols-3">
      <input v-model="renameTo" class="input" placeholder="重命名/移动到" />
      <button class="btn" :disabled="busy || !selectedPath || !renameTo" @click="rename">重命名/移动</button>
      <p class="text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </div>
    <div class="mt-4 divide-y rounded-md border">
      <div v-for="f in files" :key="f.name" class="flex items-center justify-between p-3 text-sm">
        <button class="text-left font-medium hover:underline" @click="selectedPath = f.name">{{ f.dir ? '目录' : filePurpose(f.name) }} {{ f.name }}</button>
        <div class="flex items-center gap-3">
          <span class="text-neutral-500">{{ f.dir ? '目录' : `${f.size} B` }}</span>
          <button class="btn btn-danger" :disabled="busy" @click="remove(f.name)">删除</button>
        </div>
      </div>
      <div v-if="files.length === 0" class="p-6 text-sm text-neutral-500">当前目录为空。可以上传 `ipxe.efi`、`ipxe.bios` 到 TFTP 目录，或上传 WIM/ISO/EFI/VHD 到 HTTP 目录。</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, upload } from '../lib/api'
const root = ref('http')
const files = ref<any[]>([])
const newDir = ref('')
const selectedPath = ref('')
const renameTo = ref('')
const message = ref('')
const error = ref(false)
const busy = ref(false)
async function load() {
  await run(async () => {
    const res: any = await api(`/files?root=${root.value}`)
    files.value = Array.isArray(res.files) ? res.files : []
    message.value = files.value.length === 0 ? '当前目录为空' : `已加载 ${files.value.length} 个条目`
  })
}
async function run(task: () => Promise<void>) {
  busy.value = true
  error.value = false
  try { await task() } catch (e) { error.value = true; message.value = e instanceof Error ? e.message : '操作失败' } finally { busy.value = false }
}
async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files?.[0]) return
  await run(async () => {
    const form = new FormData()
    form.append('root', root.value)
    form.append('file', input.files![0])
    await upload('/files/upload', form)
    message.value = '文件已上传'
    await load()
  })
}
async function mkdir() { await run(async () => { await api('/files/mkdir', { method: 'POST', body: JSON.stringify({ root: root.value, path: newDir.value }) }); message.value = '目录已创建'; await load() }) }
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
  if (ext === 'efi') return 'UEFI'
  if (ext === 'kpxe' || ext === 'pxe' || ext === 'bios') return 'PXE'
  if (ext === 'wim') return 'WIM'
  if (ext === 'iso') return 'ISO'
  if (ext === 'vhd' || ext === 'vhdx') return '虚拟磁盘'
  return '文件'
}
onMounted(load)
</script>
