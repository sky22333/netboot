<template>
  <div class="card p-5">
    <h1 class="text-lg font-semibold">用户管理</h1>
    <div class="mt-4 grid gap-2 md:grid-cols-4">
      <input v-model="username" class="input" placeholder="用户名" />
      <input v-model="password" type="password" class="input" placeholder="密码至少 8 位" />
      <select v-model="role" class="input"><option value="admin">管理员</option></select>
      <button class="btn btn-primary" @click="create">创建用户</button>
    </div>
    <div class="mt-4 divide-y rounded-md border">
      <div v-for="u in users" :key="u.id" class="flex items-center justify-between p-3 text-sm">
        <span>{{ u.username }} / {{ u.role }}</span>
        <button class="btn" @click="toggle(u)">{{ u.enabled ? '禁用' : '启用' }}</button>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../lib/api'
const users = ref<any[]>([])
const username = ref('')
const password = ref('')
const role = ref('admin')
async function load() { users.value = await api('/users') }
async function create() { await api('/users', { method: 'POST', body: JSON.stringify({ username: username.value, password: password.value, role: role.value }) }); username.value = ''; password.value = ''; await load() }
async function toggle(u: any) { await api(`/users/${u.id}/enabled`, { method: 'POST', body: JSON.stringify({ enabled: !u.enabled }) }); await load() }
onMounted(load)
</script>
