<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-lg font-semibold">客户端操作菜单</h1>
          <p class="mt-1 text-sm text-neutral-500">维护可对客户端执行的服务器端命令模板，支持 %IP%、%MAC%、%NAME% 等变量。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn" :disabled="loading" @click="load">{{ loading ? '刷新中...' : '刷新' }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中...' : '保存操作' }}</button>
        </div>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <section class="card overflow-hidden">
        <div class="flex flex-wrap gap-2 border-b border-neutral-200 p-4">
          <button class="btn" @click="add">添加操作</button>
          <button v-for="template in templates" :key="template.key" class="btn" @click="addTemplate(template)">{{ template.label }}</button>
        </div>
        <div class="divide-y divide-neutral-100">
          <div v-for="(action, index) in actions" :key="actionKey(action, index)" class="grid gap-2 p-4 lg:grid-cols-[5rem_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.4fr)_auto] lg:items-center">
            <input v-model.number="action.sort_order" class="input" type="number" min="1" title="排序" />
            <input v-model.trim="action.name" class="input" placeholder="显示名称" />
            <input v-model.trim="action.command" class="input" placeholder="命令，例如 ping" />
            <input v-model="action.args" class="input" placeholder="参数，例如 -n 1 %IP%" />
            <div class="flex flex-wrap justify-end gap-2">
              <label class="flex items-center gap-2 text-sm"><input v-model="action.enabled" type="checkbox" /> 启用</label>
              <button class="btn h-9 px-2" :disabled="executingId === action.id || !action.id || selectedClientIds.length === 0 || !action.enabled" :title="!action.id ? '保存后才能执行' : ''" @click="execute(action)">
                {{ executingId === action.id ? '执行中' : '执行' }}
              </button>
              <button class="btn btn-danger h-9 px-2" @click="remove(index)">删除</button>
            </div>
          </div>
          <div v-if="actions.length === 0" class="p-10 text-center text-sm text-neutral-500">暂无操作。可以添加模板后按需修改。</div>
        </div>
      </section>

      <aside class="space-y-4">
        <section class="card p-4">
          <h2 class="font-medium">目标客户端</h2>
          <p class="mt-1 text-xs text-neutral-500">选择要执行操作的客户端。命令在服务器端运行，变量会替换为客户端信息。</p>
          <div class="mt-3 max-h-80 space-y-2 overflow-auto">
            <label v-for="client in clients" :key="client.id" class="flex items-start gap-2 rounded-md border border-neutral-200 p-2 text-sm">
              <input v-model="selectedClientIds" class="mt-1" type="checkbox" :value="client.id" />
              <span class="min-w-0">
                <span class="block truncate font-medium">{{ client.name }}</span>
                <span class="block truncate text-xs text-neutral-500">{{ client.ip || '无 IP' }} · {{ client.mac || '无 MAC' }}</span>
              </span>
            </label>
            <div v-if="clients.length === 0" class="text-sm text-neutral-500">暂无客户端。</div>
          </div>
        </section>

        <section v-if="results.length" class="card p-4">
          <h2 class="font-medium">执行结果</h2>
          <div class="mt-3 space-y-2">
            <div v-for="item in results" :key="item.client_id" class="rounded-md border border-neutral-200 p-3 text-xs">
              <div class="font-medium" :class="item.ok ? 'text-green-700' : 'text-red-700'">{{ item.client }} · {{ item.ok ? '成功' : item.error }}</div>
              <pre v-if="item.output" class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded bg-neutral-50 p-2 font-mono text-neutral-700">{{ item.output }}</pre>
            </div>
          </div>
        </section>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../lib/api'

type Action = { id?: number; sort_order: number; name: string; command: string; args: string; enabled: boolean }
type Client = { id: number; name: string; ip: string; mac: string }
type ActionResult = { client_id: number; client: string; ok: boolean; output?: string; error?: string }
type ActionTemplate = { key: string; label: string; name: string; command: string; args: string }

const actions = ref<Action[]>([])
const clients = ref<Client[]>([])
const templates = ref<ActionTemplate[]>([])
const selectedClientIds = ref<number[]>([])
const results = ref<ActionResult[]>([])
const loading = ref(false)
const saving = ref(false)
const executingId = ref<number | null>(null)
const message = ref('')
const error = ref(false)
const sortedActions = computed(() => [...actions.value].sort((a, b) => a.sort_order - b.sort_order))

async function load() {
  loading.value = true
  error.value = false
  try {
    const [actionRows, clientRows, templateRows] = await Promise.all([api<Action[]>('/actions'), api<Client[]>('/clients'), api<ActionTemplate[]>('/actions/templates')])
    actions.value = Array.isArray(actionRows) ? actionRows : []
    clients.value = Array.isArray(clientRows) ? clientRows : []
    templates.value = Array.isArray(templateRows) ? templateRows : []
    normalizeOrder()
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '读取失败'
  } finally {
    loading.value = false
  }
}

function add() {
  actions.value.push({ sort_order: actions.value.length + 1, name: 'New Action', command: '', args: '', enabled: true })
}

function addTemplate(template: ActionTemplate) {
  actions.value.push({ sort_order: actions.value.length + 1, name: template.name, command: template.command, args: template.args, enabled: true })
}

function remove(index: number) {
  actions.value.splice(index, 1)
  normalizeOrder()
}

async function save() {
  if (saving.value) return
  saving.value = true
  error.value = false
  try {
    const rows = await api<Action[]>('/actions', { method: 'PUT', body: JSON.stringify(sortedActions.value.map(sanitizeAction)) })
    actions.value = Array.isArray(rows) ? rows : []
    normalizeOrder()
    message.value = '客户端操作已保存。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function execute(action: Action) {
  if (!action.id || selectedClientIds.value.length === 0 || executingId.value !== null) return
  executingId.value = action.id
  error.value = false
  results.value = []
  try {
    const rows = await api<ActionResult[]>(`/actions/${action.id}/execute`, { method: 'POST', body: JSON.stringify({ client_ids: selectedClientIds.value }) })
    results.value = Array.isArray(rows) ? rows : []
    message.value = `执行完成：${results.value.length} 台客户端。`
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '执行失败'
  } finally {
    executingId.value = null
  }
}

function sanitizeAction(action: Action) {
  return { ...action, sort_order: Number(action.sort_order) || 1, name: action.name.trim(), command: action.command.trim(), args: action.args || '' }
}

function normalizeOrder() {
  actions.value = sortedActions.value.map((item, index) => ({ ...item, sort_order: index + 1 }))
}

function actionKey(action: Action, index: number) {
  return action.id ? `id-${action.id}` : `new-${index}`
}

onMounted(load)
</script>
