<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div>
        <h1 class="text-lg font-semibold">用户管理</h1>
        <p class="mt-1 text-sm text-neutral-500">创建后台管理员账号，默认管理员用于兜底维护，不能删除。</p>
      </div>
      <div class="mt-4 grid gap-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_12rem_8rem]">
        <input v-model.trim="username" class="input" autocomplete="off" placeholder="用户名" />
        <input v-model="password" type="password" class="input" autocomplete="new-password" placeholder="密码至少 8 位" />
        <select v-model="role" class="input">
          <option value="admin">管理员</option>
        </select>
        <button class="btn btn-primary" :disabled="creating || !canCreate" @click="create">
          {{ creating ? '创建中...' : '创建用户' }}
        </button>
      </div>
      <p class="mt-2 text-xs text-neutral-500">用户名需为 3-32 位，仅支持字母、数字、点、下划线、短横线和 @。</p>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>

    <section class="card p-5">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="font-semibold">账号列表</h2>
          <p class="mt-1 text-xs text-neutral-500">删除无用账号可立即阻止其继续登录。</p>
        </div>
        <button class="btn" :disabled="loading" @click="load">{{ loading ? '刷新中...' : '刷新' }}</button>
      </div>
      <div class="mt-4 overflow-hidden rounded-md border border-neutral-200">
        <div v-for="u in users" :key="u.id" class="grid gap-3 border-b border-neutral-100 p-3 text-sm last:border-b-0 sm:grid-cols-[minmax(0,1fr)_8rem_9rem] sm:items-center">
          <div class="min-w-0">
            <div class="truncate font-medium" :title="u.username">{{ u.username }}</div>
            <div class="mt-1 text-xs text-neutral-500">创建时间：{{ shortDate(u.created_at) }}</div>
          </div>
          <div class="text-neutral-600">{{ u.role === 'admin' ? '管理员' : u.role }}</div>
          <div class="flex justify-start sm:justify-end">
            <button class="btn btn-danger" :disabled="isDefaultAdmin(u) || deletingId === u.id" @click="remove(u)">
              {{ isDefaultAdmin(u) ? '默认管理员' : deletingId === u.id ? '删除中...' : '删除' }}
            </button>
          </div>
        </div>
        <div v-if="users.length === 0" class="p-4 text-sm text-neutral-500">暂无用户。</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../lib/api'

type User = {
  id: number
  username: string
  role: string
  enabled: boolean
  created_at: string
}

const users = ref<User[]>([])
const username = ref('')
const password = ref('')
const role = ref('admin')
const loading = ref(false)
const creating = ref(false)
const deletingId = ref<number | null>(null)
const message = ref('')
const error = ref(false)
const usernamePattern = /^[A-Za-z0-9._@-]{3,32}$/

const firstUserId = computed(() => users.value.length ? Math.min(...users.value.map((u) => u.id)) : 0)
const canCreate = computed(() => usernamePattern.test(username.value.trim()) && password.value.length >= 8)

async function load() {
  loading.value = true
  try {
    users.value = await api<User[]>('/users')
  } finally {
    loading.value = false
  }
}

async function create() {
  if (creating.value || !canCreate.value) return
  creating.value = true
  error.value = false
  try {
    await api('/users', { method: 'POST', body: JSON.stringify({ username: username.value.trim(), password: password.value, role: role.value }) })
    username.value = ''
    password.value = ''
    message.value = '用户已创建。'
    await load()
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '创建失败'
  } finally {
    creating.value = false
  }
}

async function remove(user: User) {
  if (deletingId.value !== null || isDefaultAdmin(user)) return
  if (!window.confirm(`确认删除用户 ${user.username}？`)) return
  deletingId.value = user.id
  error.value = false
  try {
    await api(`/users/${user.id}`, { method: 'DELETE' })
    message.value = '用户已删除。'
    await load()
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '删除失败'
  } finally {
    deletingId.value = null
  }
}

function isDefaultAdmin(user: User) {
  return user.id === firstUserId.value
}

function shortDate(value: string) {
  return value ? value.slice(0, 10) : '-'
}

onMounted(load)
</script>
