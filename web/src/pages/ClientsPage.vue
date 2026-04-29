<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-lg font-semibold">客户端</h1>
          <p class="mt-1 text-sm text-neutral-500">管理 PXE 客户端、静态绑定、待认领设备和唤醒操作。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn" :disabled="busy" @click="load">{{ busy ? '刷新中...' : '刷新' }}</button>
          <button class="btn btn-primary" :disabled="busy" @click="newClient">添加客户端</button>
        </div>
      </div>
      <div class="mt-4 grid gap-2 lg:grid-cols-[minmax(0,1fr)_10rem_8rem_auto]">
        <input v-model.trim="batchPrefix" class="input" placeholder="名称前缀，例如 PC-" />
        <input v-model.trim="batchIP" class="input" placeholder="起始 IP" />
        <input v-model.number="batchCount" class="input" type="number" min="1" max="1000" />
        <button class="btn" :disabled="busy || !canBatch" @click="batch">批量添加待认领</button>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <section class="card overflow-hidden">
        <div class="hidden overflow-x-auto md:block">
          <table class="w-full table-fixed text-sm">
            <thead class="border-b border-neutral-200 bg-neutral-50 text-left text-xs font-medium text-neutral-500">
              <tr>
                <th class="w-[18%] px-4 py-3">名称</th>
                <th class="w-[16%] px-4 py-3">IP</th>
                <th class="w-[20%] px-4 py-3">MAC</th>
                <th class="w-[12%] px-4 py-3">状态</th>
                <th class="w-[18%] px-4 py-3">健康</th>
                <th class="w-[16%] px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-neutral-100">
              <tr v-for="client in clients" :key="client.id" class="hover:bg-neutral-50" :class="selected?.id === client.id ? 'bg-neutral-50' : ''">
                <td class="min-w-0 px-4 py-3">
                  <button class="max-w-full truncate font-medium" @click="select(client)">{{ client.name }}</button>
                </td>
                <td class="px-4 py-3 text-neutral-600">{{ client.ip || '-' }}</td>
                <td class="px-4 py-3 text-neutral-600">{{ client.mac || '待认领' }}</td>
                <td class="px-4 py-3">
                  <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusClass(client.status)">{{ statusText[client.status] ?? client.status }}</span>
                </td>
                <td class="px-4 py-3 text-neutral-600">{{ client.disk_health || '-' }} / {{ client.net_speed || '-' }}</td>
                <td class="px-4 py-3">
                  <div class="flex justify-end gap-1">
                    <button class="btn h-8 px-2" :disabled="busy || !client.mac" @click="wol(client)">唤醒</button>
                    <button class="btn h-8 px-2" @click="select(client)">详情</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="divide-y divide-neutral-100 md:hidden">
          <button v-for="client in clients" :key="client.id" class="flex w-full items-start justify-between gap-3 p-4 text-left text-sm hover:bg-neutral-50" @click="select(client)">
            <div class="min-w-0">
              <div class="truncate font-medium">{{ client.name }}</div>
              <div class="mt-1 text-xs text-neutral-500">{{ client.ip || '未设置 IP' }} · {{ client.mac || '待认领' }}</div>
            </div>
            <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusClass(client.status)">{{ statusText[client.status] ?? client.status }}</span>
          </button>
        </div>

        <div v-if="clients.length === 0" class="p-10 text-center text-sm text-neutral-500">暂无客户端。可以手动添加，或通过 DHCP 请求自动发现。</div>
      </section>

      <aside class="card p-4">
        <div class="flex items-center justify-between gap-3">
          <h2 class="font-medium">{{ editing.id ? '客户端详情' : '新增客户端' }}</h2>
          <span v-if="selected" class="rounded bg-neutral-100 px-2 py-1 text-xs text-neutral-500">ID {{ selected.id }}</span>
        </div>
        <div class="mt-4 space-y-3">
          <div>
            <label class="label">名称</label>
            <input v-model.trim="editing.name" class="input mt-1 w-full" placeholder="例如 PC-001" />
          </div>
          <div>
            <label class="label">IP 地址</label>
            <input v-model.trim="editing.ip" class="input mt-1 w-full" placeholder="可留空，或填写静态绑定 IP" />
          </div>
          <div>
            <label class="label">MAC 地址</label>
            <input v-model.trim="editing.mac" class="input mt-1 w-full" placeholder="可留空，设备认领后自动写入" />
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <div>
              <label class="label">固件</label>
              <select v-model="editing.firmware" class="input mt-1 w-full">
                <option value="unknown">unknown</option>
                <option value="bios">bios</option>
                <option value="uefi32">uefi32</option>
                <option value="uefi64">uefi64</option>
                <option value="ipxe">ipxe</option>
              </select>
            </div>
            <div>
              <label class="label">状态</label>
              <select v-model="editing.status" class="input mt-1 w-full">
                <option value="unknown">未知</option>
                <option value="unassigned">待认领</option>
                <option value="online">在线</option>
                <option value="offline">离线</option>
                <option value="pxe">PXE</option>
                <option value="ipxe">iPXE</option>
              </select>
            </div>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <button class="btn btn-primary" :disabled="busy || !canSave" @click="saveClient">{{ busy ? '保存中...' : '保存' }}</button>
            <button class="btn" :disabled="busy || !editing.id || !editing.mac" @click="clearMac(editing)">清 MAC</button>
            <button class="btn" :disabled="busy || !editing.id || !editing.mac" @click="wol(editing)">唤醒</button>
            <button class="btn btn-danger" :disabled="busy || !editing.id" @click="remove(editing)">删除</button>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api } from '../lib/api'

type Client = {
  id: number
  seq: number
  name: string
  ip: string
  mac: string
  firmware: string
  status: string
  last_boot_file: string
  disk_health: string
  net_speed: string
  created_at: string
  updated_at: string
}

const clients = ref<Client[]>([])
const selected = ref<Client | null>(null)
const editing = reactive<Client>(emptyClient())
const batchPrefix = ref('PC-')
const batchIP = ref('192.168.1.101')
const batchCount = ref(10)
const busy = ref(false)
const message = ref('')
const error = ref(false)
const statusText: Record<string, string> = { unknown: '未知', unassigned: '待认领', online: '在线', offline: '离线', pxe: 'PXE', ipxe: 'iPXE' }
const ipPattern = /^$|^(\d{1,3}\.){3}\d{1,3}$/
const macPattern = /^$|^([0-9A-Fa-f]{2}[:-]?){5}[0-9A-Fa-f]{2}$/
const canSave = computed(() => editing.name.trim().length > 0 && ipPattern.test(editing.ip) && macPattern.test(editing.mac))
const canBatch = computed(() => batchPrefix.value.trim().length > 0 && ipPattern.test(batchIP.value) && batchCount.value >= 1 && batchCount.value <= 1000)

function emptyClient(): Client {
  return { id: 0, seq: 0, name: '', ip: '', mac: '', firmware: 'unknown', status: 'unknown', last_boot_file: '', disk_health: '', net_speed: '', created_at: '', updated_at: '' }
}

async function load() {
  await run(async () => {
    await fetchClients()
    if (selected.value) {
      const current = clients.value.find((item) => item.id === selected.value?.id)
      if (current) select(current)
    }
  })
}

async function fetchClients() {
  const rows = await api<Client[]>('/clients')
  clients.value = Array.isArray(rows) ? rows : []
}

async function run(task: () => Promise<void>, showBusy = true) {
  if (busy.value) return
  busy.value = showBusy
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

function select(client: Client) {
  selected.value = client
  Object.assign(editing, { ...client })
}

function newClient() {
  selected.value = null
  Object.assign(editing, emptyClient(), { name: `客户端${clients.value.length + 1}` })
}

async function saveClient() {
  if (!canSave.value) return
  await run(async () => {
    const path = editing.id ? `/clients/${editing.id}` : '/clients'
    const method = editing.id ? 'PUT' : 'POST'
    const saved = await api<Client>(path, { method, body: JSON.stringify(editing) })
    message.value = '客户端已保存。'
    await reloadAndSelect(saved.id)
  })
}

async function reloadAndSelect(id: number) {
  const rows = await api<Client[]>('/clients')
  clients.value = Array.isArray(rows) ? rows : []
  const current = clients.value.find((item) => item.id === id)
  if (current) select(current)
}

async function batch() {
  if (!canBatch.value) return
  if (!window.confirm(`确认批量创建 ${batchCount.value} 台待认领客户端？`)) return
  await run(async () => {
    const rows = await api<Client[]>('/clients/batch', { method: 'POST', body: JSON.stringify({ prefix: batchPrefix.value, ip_start: batchIP.value, count: batchCount.value }) })
    message.value = `已创建 ${Array.isArray(rows) ? rows.length : 0} 台客户端。`
    await fetchClients()
  })
}

async function clearMac(client: Client) {
  if (!client.id || !client.mac) return
  if (!window.confirm(`确认清除 ${client.name} 的 MAC 绑定？`)) return
  await run(async () => {
    await api(`/clients/${client.id}/clear-mac`, { method: 'POST' })
    message.value = 'MAC 绑定已清除。'
    await reloadAndSelect(client.id)
  })
}

async function wol(client: Client) {
  if (!client.id || !client.mac) return
  await run(async () => {
    await api(`/clients/${client.id}/wol`, { method: 'POST' })
    message.value = '唤醒包已发送。'
  })
}

async function remove(client: Client) {
  if (!client.id) return
  if (!window.confirm(`确认删除客户端 ${client.name}？此操作不可恢复。`)) return
  await run(async () => {
    await api(`/clients/${client.id}`, { method: 'DELETE' })
    selected.value = null
    Object.assign(editing, emptyClient())
    message.value = '客户端已删除。'
    await fetchClients()
  })
}

function statusClass(status: string) {
  if (status === 'online') return 'border-green-200 bg-green-50 text-green-700'
  if (status === 'pxe' || status === 'ipxe') return 'border-blue-200 bg-blue-50 text-blue-700'
  if (status === 'unassigned') return 'border-amber-200 bg-amber-50 text-amber-700'
  return 'border-neutral-200 bg-neutral-50 text-neutral-600'
}

onMounted(load)
</script>
