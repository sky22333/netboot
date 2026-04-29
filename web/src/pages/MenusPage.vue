<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-lg font-semibold">启动菜单</h1>
          <p class="mt-1 text-sm text-neutral-500">维护 UEFI 原生菜单和 iPXE 动态菜单。菜单显示名称建议使用英文，避免固件控制台乱码。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn" :disabled="loading" @click="load">{{ loading ? '刷新中...' : '刷新' }}</button>
          <button class="btn" :disabled="saving" @click="restoreDefaults">恢复默认</button>
          <button class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中...' : '保存菜单' }}</button>
        </div>
      </div>
      <div class="mt-4 grid gap-3 lg:grid-cols-2">
        <div class="rounded-md border border-neutral-200 bg-neutral-50 p-3">
          <div class="text-sm font-medium">UEFI PXE</div>
          <p class="mt-1 text-xs text-neutral-500">完整 DHCP 模式可返回原生菜单；ProxyDHCP 通常直接下发启动文件。</p>
        </div>
        <div class="rounded-md border border-neutral-200 bg-neutral-50 p-3">
          <div class="text-sm font-medium">iPXE Menu</div>
          <p class="mt-1 text-xs text-neutral-500">进入 iPXE 后通过 HTTP 生成动态菜单，适合执行 boot.ipxe、netboot.xyz 或外部脚本。</p>
        </div>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>

    <section v-for="menu in menus" :key="menu.menu_type" class="card overflow-hidden">
      <div class="flex flex-col gap-3 border-b border-neutral-200 p-5 md:flex-row md:items-start md:justify-between">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="font-semibold">{{ names[menu.menu_type] ?? menu.menu_type }}</h2>
            <span class="rounded-full border border-neutral-200 px-2 py-0.5 text-xs text-neutral-500">{{ modeText(menu.menu_type) }}</span>
          </div>
          <p class="mt-1 text-sm text-neutral-500">{{ menuHint(menu.menu_type) }}</p>
        </div>
        <label class="flex items-center gap-2 text-sm"><input v-model="menu.enabled" type="checkbox" /> 启用</label>
      </div>

      <div class="grid gap-3 p-5 md:grid-cols-[minmax(0,1fr)_10rem_12rem]">
        <div>
          <label class="label">菜单标题</label>
          <input v-model.trim="menu.prompt" class="input mt-1 w-full" />
        </div>
        <div>
          <label class="label">等待秒数</label>
          <input v-model.number="menu.timeout_seconds" class="input mt-1 w-full" type="number" min="0" max="255" />
        </div>
        <label class="mt-7 flex items-center gap-2 text-sm"><input v-model="menu.randomize_timeout" type="checkbox" /> 随机等待时间</label>
      </div>

      <div class="px-5 pb-5">
        <div class="mb-3 flex items-center justify-between gap-3">
          <p class="text-xs text-neutral-500">{{ menuNote(menu.menu_type) }}</p>
          <button class="btn shrink-0" @click="addItem(menu)">添加菜单项</button>
        </div>
        <div class="hidden gap-2 px-1 pb-2 text-xs font-medium text-neutral-500 lg:grid lg:grid-cols-[4rem_1.1fr_1.7fr_.7fr_.9fr_10rem]">
          <span>排序</span>
          <span>显示名称</span>
          <span>启动文件或脚本</span>
          <span>类型码</span>
          <span>服务器 IP</span>
          <span>操作</span>
        </div>
        <div class="space-y-2">
          <div v-for="(item, index) in menu.items" :key="itemKey(item, index)" class="grid gap-2 rounded-md border border-neutral-200 p-2 lg:grid-cols-[4rem_1.1fr_1.7fr_.7fr_.9fr_10rem]">
            <input v-model.number="item.sort_order" class="input" type="number" min="1" />
            <input v-model.trim="item.title" class="input" placeholder="Run boot.ipxe" />
            <input v-model.trim="item.boot_file" class="input" placeholder="%dynamicboot%=boot.ipxe" />
            <input v-model.trim="item.pxe_type" class="input" placeholder="8005" />
            <input v-model.trim="item.server_ip" class="input" placeholder="%tftpserver%" />
            <div class="flex flex-wrap items-center justify-end gap-2">
              <label class="flex items-center gap-1 text-sm"><input v-model="item.enabled" type="checkbox" /> 启用</label>
              <button class="btn h-9 px-2" :disabled="index === 0" @click="moveItem(menu, index, -1)">上移</button>
              <button class="btn btn-danger h-9 px-2" @click="removeItem(menu, index)">删除</button>
            </div>
          </div>
        </div>
        <div v-if="menu.items.length === 0" class="rounded-md border border-neutral-200 p-4 text-sm text-neutral-500">此菜单没有菜单项。</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../lib/api'

type MenuItem = { id?: number; menu_id?: number; sort_order: number; title: string; boot_file: string; pxe_type: string; server_ip: string; enabled: boolean }
type Menu = { id?: number; menu_type: string; enabled: boolean; prompt: string; timeout_seconds: number; randomize_timeout: boolean; items: MenuItem[] }

const menus = ref<Menu[]>([])
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const error = ref(false)
const names: Record<string, string> = { uefi: 'UEFI Boot Menu', ipxe: 'iPXE Menu' }

async function load() {
  loading.value = true
  error.value = false
  try {
    const rows = await api<Menu[]>('/menus')
    menus.value = normalizeMenus(rows)
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '读取菜单失败'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (saving.value) return
  saving.value = true
  error.value = false
  try {
    const payload = normalizeMenus(menus.value).map((menu) => ({ ...menu, items: menu.items.map(sanitizeItem) }))
    const rows = await api<Menu[]>('/menus', { method: 'PUT', body: JSON.stringify(payload) })
    menus.value = normalizeMenus(rows)
    message.value = '菜单已保存，后续 PXE 请求会使用新菜单。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function addItem(menu: Menu) {
  const next = menu.items.length + 1
  menu.items.push({
    sort_order: next,
    title: menu.menu_type === 'ipxe' ? 'Run boot.ipxe' : 'iPXE UEFI',
    boot_file: menu.menu_type === 'ipxe' ? '%dynamicboot%=boot.ipxe' : 'ipxe.efi',
    pxe_type: menu.menu_type === 'ipxe' ? `800${next}` : `800${next + 1}`,
    server_ip: '%tftpserver%',
    enabled: true
  })
}

function removeItem(menu: Menu, index: number) {
  menu.items.splice(index, 1)
  normalizeItemOrder(menu)
}

function moveItem(menu: Menu, index: number, direction: -1 | 1) {
  const target = index + direction
  if (target < 0 || target >= menu.items.length) return
  const [item] = menu.items.splice(index, 1)
  menu.items.splice(target, 0, item)
  normalizeItemOrder(menu)
}

function restoreDefaults() {
  if (!window.confirm('确认恢复默认菜单？当前未保存修改会被覆盖。')) return
  menus.value = defaultMenus()
  message.value = '已恢复默认菜单，点击保存后生效。'
  error.value = false
}

function normalizeMenus(rows: Menu[] | unknown) {
  const list = Array.isArray(rows) ? rows : []
  const normalized = list.map((menu) => ({ ...menu, items: Array.isArray(menu.items) ? menu.items.map(sanitizeItem) : [] }))
  for (const menu of normalized) normalizeItemOrder(menu)
  return normalized
}

function normalizeItemOrder(menu: Menu) {
  menu.items = [...menu.items].sort((a, b) => a.sort_order - b.sort_order).map((item, index) => ({ ...item, sort_order: index + 1 }))
}

function sanitizeItem(item: MenuItem) {
  return {
    ...item,
    sort_order: Number(item.sort_order) || 1,
    title: item.title?.trim() || 'Boot Item',
    boot_file: item.boot_file?.trim() || '',
    pxe_type: item.pxe_type?.trim() || '0000',
    server_ip: item.server_ip?.trim() || '%tftpserver%',
    enabled: Boolean(item.enabled)
  }
}

function defaultMenus(): Menu[] {
  return [
    { menu_type: 'uefi', enabled: true, prompt: 'UEFI Boot Menu', timeout_seconds: 6, randomize_timeout: false, items: [
      { sort_order: 1, title: 'iPXE UEFI', boot_file: 'ipxe.efi', pxe_type: '8002', server_ip: '%tftpserver%', enabled: true },
      { sort_order: 2, title: 'Boot Local Disk', boot_file: '', pxe_type: '0000', server_ip: '0.0.0.0', enabled: true }
    ] },
    { menu_type: 'ipxe', enabled: true, prompt: 'iPXE Boot Menu', timeout_seconds: 6, randomize_timeout: false, items: [
      { sort_order: 1, title: 'Run boot.ipxe', boot_file: '%dynamicboot%=boot.ipxe', pxe_type: '0001', server_ip: '%tftpserver%', enabled: true },
      { sort_order: 2, title: 'netboot.xyz', boot_file: 'https://boot.netboot.xyz', pxe_type: '8005', server_ip: '%tftpserver%', enabled: true },
      { sort_order: 3, title: 'Boot Local Disk', boot_file: '', pxe_type: '0000', server_ip: '0.0.0.0', enabled: true }
    ] }
  ]
}

function modeText(type: string) {
  return type === 'uefi' ? 'Native PXE' : 'HTTP Dynamic'
}

function menuHint(type: string) {
  return type === 'uefi'
    ? 'UEFI 客户端在完整 DHCP 模式下可使用原生 PXE 菜单。'
    : 'iPXE 菜单由 dynamic.ipxe 生成，适合链式加载本地或公网启动脚本。'
}

function menuNote(type: string) {
  return type === 'ipxe'
    ? 'iPXE 支持 %dynamicboot%=boot.ipxe；相对路径从 HTTP Boot 目录读取，URL 会直接 chain。'
    : 'UEFI 类型码需要保持唯一；服务器 IP 可用 %tftpserver% 表示当前通告 IP。'
}

function itemKey(item: MenuItem, index: number) {
  return item.id ? `id-${item.id}` : `new-${index}-${item.sort_order}`
}

onMounted(load)
</script>
