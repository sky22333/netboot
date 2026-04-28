<template>
  <div class="space-y-4">
    <div v-for="menu in menus" :key="menu.id" class="card p-5">
      <div class="flex items-center justify-between">
        <div><h2 class="font-semibold">{{ names[menu.menu_type] ?? menu.menu_type }}</h2><p class="text-sm text-neutral-500">{{ menu.prompt }}</p></div>
        <label class="text-sm"><input v-model="menu.enabled" type="checkbox" /> 启用</label>
      </div>
      <div class="mt-4 space-y-2">
        <div v-for="item in menu.items" :key="item.id" class="grid gap-2 md:grid-cols-4">
          <input v-model="item.title" class="input" placeholder="显示名称" />
          <input v-model="item.boot_file" class="input" placeholder="启动文件或动态脚本" />
          <input v-model="item.pxe_type" class="input" placeholder="PXE 类型码" />
          <input v-model="item.server_ip" class="input" placeholder="服务器 IP，可留空" />
        </div>
      </div>
      <p class="mt-3 text-xs text-neutral-500">PXE 固件菜单建议使用 ASCII 名称；iPXE 菜单支持中文，但路径中有空格或中文时需要确认 HTTP Boot 可访问。</p>
    </div>
    <button class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中...' : '保存菜单' }}</button>
    <p v-if="message" class="text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
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
async function load() { menus.value = await api('/menus') }
async function save() {
  saving.value = true
  error.value = false
  try {
    menus.value = await api('/menus', { method: 'PUT', body: JSON.stringify(menus.value) })
    message.value = '菜单已保存，后续 PXE 请求会使用新菜单。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}
onMounted(load)
</script>
