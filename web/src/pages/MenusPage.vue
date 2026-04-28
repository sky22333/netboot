<template>
  <div class="space-y-4">
    <div class="card p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-lg font-semibold">启动菜单</h1>
          <p class="mt-1 text-sm text-neutral-500">管理 PXE 启动菜单。BIOS 和 ProxyDHCP 默认下发启动文件；UEFI 在完整 DHCP 模式下可使用原生菜单；进入 iPXE 后使用动态 HTTP 菜单。</p>
        </div>
        <button class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中...' : '保存菜单' }}</button>
      </div>
      <div class="mt-4 grid gap-3 lg:grid-cols-3">
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-sm font-medium">BIOS PXE</div>
          <p class="mt-1 text-xs text-neutral-500">老式 BIOS 默认直接下发启动文件，避免原生菜单兼容问题。</p>
        </div>
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-sm font-medium">UEFI PXE</div>
          <p class="mt-1 text-xs text-neutral-500">完整 DHCP 可显示原生菜单；ProxyDHCP 默认下发启动文件。</p>
        </div>
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-sm font-medium">iPXE 菜单</div>
          <p class="mt-1 text-xs text-neutral-500">支持 HTTP 时显示动态菜单；不支持时回退到可执行启动文件。</p>
        </div>
      </div>
    </div>

    <div v-for="menu in menus" :key="menu.id" class="card overflow-hidden">
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

      <div class="grid gap-3 p-5 md:grid-cols-3">
        <div>
          <label class="label">提示文本</label>
          <input v-model="menu.prompt" class="input mt-1 w-full" />
        </div>
        <div>
          <label class="label">等待秒数</label>
          <input v-model.number="menu.timeout_seconds" class="input mt-1 w-full" type="number" min="0" max="255" />
        </div>
        <label class="mt-6 flex items-center gap-2 text-sm"><input v-model="menu.randomize_timeout" type="checkbox" /> 随机等待时间</label>
      </div>

      <div class="px-5 pb-5">
        <div class="mb-2 grid gap-2 px-1 text-xs font-medium text-neutral-500 md:grid-cols-[1.2fr_1.6fr_.7fr_.9fr_auto]">
          <span>显示名称</span>
          <span>启动文件或脚本</span>
          <span>类型码</span>
          <span>服务器 IP</span>
          <span>启用</span>
        </div>
        <div class="space-y-2">
          <div v-for="item in menuItems(menu)" :key="item.id" class="grid gap-2 rounded-md border border-neutral-200 p-2 md:grid-cols-[1.2fr_1.6fr_.7fr_.9fr_auto]">
            <input v-model="item.title" class="input" placeholder="例如 netboot.xyz" />
            <input v-model="item.boot_file" class="input" placeholder="例如 netboot/netboot.xyz.kpxe 或 %dynamicboot%=ipxefm" />
            <input v-model="item.pxe_type" class="input" placeholder="8000" />
            <input v-model="item.server_ip" class="input" placeholder="%tftpserver%" />
            <label class="flex items-center gap-2 px-1 text-sm"><input v-model="item.enabled" type="checkbox" /> 启用</label>
          </div>
        </div>
        <p class="mt-3 text-xs text-neutral-500">{{ menuNote(menu.menu_type) }}</p>
      </div>
    </div>

    <p v-if="message" class="rounded-md border p-3 text-sm" :class="error ? 'border-red-200 bg-red-50 text-red-700' : 'border-neutral-200 bg-white text-neutral-600'">{{ message }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../lib/api'

const menus = ref<any[]>([])
const saving = ref(false)
const message = ref('')
const error = ref(false)
const names: Record<string, string> = { bios: 'BIOS 菜单', uefi: 'UEFI 菜单', ipxe: 'iPXE 菜单' }

async function load() {
  const rows = await api<any[]>('/menus')
  menus.value = Array.isArray(rows) ? rows.map(normalizeMenu) : []
}
async function save() {
  saving.value = true
  error.value = false
  try {
    const rows = await api<any[]>('/menus', { method: 'PUT', body: JSON.stringify(menus.value.map(normalizeMenu)) })
    menus.value = Array.isArray(rows) ? rows.map(normalizeMenu) : []
    message.value = '菜单已保存，后续 PXE 请求会使用新菜单。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}
function modeText(type: string) {
  if (type === 'bios') return '老设备兼容'
  if (type === 'uefi') return '原生菜单可选'
  return '动态菜单'
}
function menuHint(type: string) {
  if (type === 'bios') return 'BIOS 客户端默认使用启动文件；这里用于保留传统菜单项和兼容配置。'
  if (type === 'uefi') return 'UEFI 客户端在完整 DHCP 模式下可显示原生菜单，也可直接使用启动文件。'
  return 'iPXE 菜单通过 HTTP 动态生成，可列出本地启动文件和 netboot.xyz。'
}
function menuNote(type: string) {
  if (type === 'ipxe') return 'iPXE 菜单支持动态宏，例如 %dynamicboot%=ipxefm；路径建议使用 HTTP 可访问的相对路径或完整 URL。'
  return '传统 PXE 菜单名称建议使用 ASCII；PXE 类型码需要保持唯一，服务器 IP 可用 %tftpserver% 表示当前通告 IP。'
}
function normalizeMenu(menu: any) {
  return { ...menu, items: Array.isArray(menu?.items) ? menu.items : [] }
}
function menuItems(menu: any) {
  return Array.isArray(menu?.items) ? menu.items : []
}
onMounted(load)
</script>
