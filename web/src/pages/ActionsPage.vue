<template>
  <div class="card p-5">
    <div class="flex items-center justify-between"><h1 class="text-lg font-semibold">客户端操作菜单</h1><button class="btn btn-primary" @click="save">保存</button></div>
    <div class="mt-4 grid gap-2 md:grid-cols-2">
      <input v-model="clientIDs" class="input" placeholder="执行客户端 ID，逗号分隔" />
      <p class="text-sm text-neutral-500">{{ message }}</p>
    </div>
    <div class="mt-4 space-y-2">
      <div v-for="a in actions" :key="a.id || a.sort_order" class="grid gap-2 md:grid-cols-5">
        <input v-model.number="a.sort_order" class="input" type="number" />
        <input v-model="a.name" class="input" placeholder="名称" />
        <input v-model="a.command" class="input" placeholder="命令" />
        <input v-model="a.args" class="input" placeholder="参数，支持 %IP% %MAC%" />
        <label class="flex items-center gap-2 text-sm"><input v-model="a.enabled" type="checkbox" /> 启用 <button v-if="a.id" class="btn" @click.prevent="execute(a.id)">执行</button></label>
      </div>
    </div>
    <button class="btn mt-4" @click="add">添加操作</button>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../lib/api'
const actions = ref<any[]>([])
const clientIDs = ref('')
const message = ref('')
async function load() { actions.value = await api('/actions') }
function add() { actions.value.push({ sort_order: actions.value.length + 1, name: '新操作', command: 'cmd', args: '', enabled: true }) }
async function save() { actions.value = await api('/actions', { method: 'PUT', body: JSON.stringify(actions.value) }) }
async function execute(id: number) { const ids = clientIDs.value.split(',').map(v => Number(v.trim())).filter(Boolean); const r: any = await api(`/actions/${id}/execute`, { method: 'POST', body: JSON.stringify({ client_ids: ids }) }); message.value = `执行完成：${r.length} 项` }
onMounted(load)
</script>
