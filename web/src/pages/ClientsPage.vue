<template>
  <div class="space-y-4">
    <div class="card p-5">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-lg font-semibold">客户端</h1>
          <p class="text-sm text-neutral-500">管理静态绑定、待分配客户端和唤醒操作。</p>
        </div>
        <button class="btn btn-primary" :disabled="busy" @click="add">添加客户端</button>
      </div>
      <div class="mt-4 grid gap-2 md:grid-cols-4">
        <input v-model="batchPrefix" class="input" placeholder="名称前缀，如 PC-" />
        <input v-model="batchIP" class="input" placeholder="起始 IP，如 192.168.1.101" />
        <input v-model.number="batchCount" class="input" type="number" min="1" max="1000" />
        <button class="btn" :disabled="busy" @click="batch">批量添加待分配</button>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
      <div class="mt-4 overflow-x-auto">
        <table class="w-full text-sm">
          <thead><tr class="border-b text-left text-neutral-500"><th class="py-2">名称</th><th>IP</th><th>MAC</th><th>固件</th><th>状态</th><th>健康</th><th></th></tr></thead>
          <tbody>
            <tr v-for="c in clients" :key="c.id" class="border-b">
              <td class="py-2">{{ c.name }}</td><td>{{ c.ip }}</td><td>{{ c.mac || '待认领' }}</td><td>{{ c.firmware }}</td><td>{{ statusText[c.status] ?? c.status }}</td><td>{{ c.disk_health || '-' }} / {{ c.net_speed || '-' }}</td>
              <td class="space-x-2 whitespace-nowrap text-right"><button class="btn" :disabled="busy || !c.mac" @click="wol(c)">唤醒</button><button class="btn" :disabled="busy || !c.mac" @click="clearMac(c)">清 MAC</button><button class="btn btn-danger" :disabled="busy" @click="remove(c)">删除</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../lib/api'
const clients = ref<any[]>([])
const batchPrefix = ref('PC-')
const batchIP = ref('192.168.1.101')
const batchCount = ref(10)
const busy = ref(false)
const message = ref('')
const error = ref(false)
const statusText: Record<string, string> = { unknown: '未知', unassigned: '待认领', online: '在线', offline: '离线', pxe: 'PXE', ipxe: 'iPXE' }
async function load() { const rows = await api<any[]>('/clients'); clients.value = Array.isArray(rows) ? rows : [] }
async function run(task: () => Promise<void>) {
  busy.value = true
  error.value = false
  try { await task() } catch (e) { error.value = true; message.value = e instanceof Error ? e.message : '操作失败' } finally { busy.value = false }
}
async function add() { await run(async () => { await api('/clients', { method: 'POST', body: JSON.stringify({ name: `客户端${clients.value.length + 1}`, ip: '', mac: '', firmware: 'unknown', status: 'unknown' }) }); message.value = '客户端已添加'; await load() }) }
async function batch() {
  if (!window.confirm(`确认批量创建 ${batchCount.value} 台待认领客户端？`)) return
  await run(async () => { await api('/clients/batch', { method: 'POST', body: JSON.stringify({ prefix: batchPrefix.value, ip_start: batchIP.value, count: batchCount.value }) }); message.value = '批量客户端已创建'; await load() })
}
async function clearMac(client: any) {
  if (!window.confirm(`确认清除 ${client.name} 的 MAC 绑定？该客户端需要重新认领。`)) return
  await run(async () => { await api(`/clients/${client.id}/clear-mac`, { method: 'POST' }); message.value = 'MAC 绑定已清除'; await load() })
}
async function wol(client: any) {
  if (!window.confirm(`确认向 ${client.name} 发送 WOL 唤醒包？`)) return
  await run(async () => { await api(`/clients/${client.id}/wol`, { method: 'POST' }); message.value = '唤醒包已发送' })
}
async function remove(client: any) {
  if (!window.confirm(`确认删除客户端 ${client.name}？此操作不可恢复。`)) return
  await run(async () => { await api(`/clients/${client.id}`, { method: 'DELETE' }); message.value = '客户端已删除'; await load() })
}
onMounted(load)
</script>
