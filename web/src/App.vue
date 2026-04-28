<template>
  <div v-if="authMode !== 'ready'" class="flex min-h-screen items-center justify-center bg-neutral-50 p-4">
    <div class="card w-full max-w-md p-6">
      <div class="mb-6">
        <div class="text-2xl font-semibold">pxe</div>
        <p class="mt-1 text-sm text-neutral-500">{{ authMode === 'setup' ? '首次使用，请创建管理员账号。' : '请登录管理控制台。' }}</p>
      </div>
      <div class="space-y-3">
        <label class="label">用户名</label>
        <input v-model.trim="username" class="input w-full" placeholder="admin" />
        <p v-if="authMode === 'setup'" class="text-xs text-neutral-500">3-32 位，仅支持字母、数字、点、下划线、短横线和 @。</p>
        <label class="label">密码</label>
        <input v-model="password" class="input w-full" type="password" placeholder="至少 8 位" @keydown.enter="submitAuth" />
        <button class="btn btn-primary w-full" :disabled="authBusy" @click="submitAuth">{{ authBusy ? '处理中...' : authMode === 'setup' ? '创建账号' : '登录' }}</button>
        <p v-if="authError" class="text-sm text-red-600">{{ authError }}</p>
      </div>
    </div>
  </div>
  <div v-if="authMode === 'ready'" class="min-h-screen">
    <aside class="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r border-neutral-200 bg-white lg:flex">
      <div class="flex h-16 items-center border-b px-5">
        <div>
          <div class="text-lg font-semibold">pxe</div>
          <div class="text-xs text-neutral-500">网络启动管理控制台</div>
        </div>
      </div>
      <nav class="flex-1 space-y-1 p-3">
        <RouterLink v-for="item in nav" :key="item.path" :to="item.path" class="flex items-center gap-3 rounded-md px-3 py-2 text-sm text-neutral-700 transition hover:bg-neutral-100" active-class="bg-neutral-950 text-white hover:bg-neutral-950">
          <component :is="item.icon" class="h-4 w-4" />
          {{ item.name }}
        </RouterLink>
      </nav>
      <div class="border-t px-3 py-2">
        <a class="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-neutral-500 transition hover:bg-neutral-100 hover:text-neutral-950" href="https://github.com/sky22333/netboot" target="_blank" rel="noreferrer">
          <Github class="h-3.5 w-3.5" />
          GitHub
        </a>
      </div>
    </aside>
    <div v-if="mobileOpen" class="fixed inset-0 z-40 lg:hidden">
      <button class="absolute inset-0 bg-black/30" aria-label="关闭导航" @click="mobileOpen = false"></button>
      <aside class="absolute inset-y-0 left-0 flex w-72 max-w-[82vw] flex-col border-r border-neutral-200 bg-white shadow-xl">
        <div class="flex h-16 items-center justify-between border-b px-5">
          <div>
            <div class="text-lg font-semibold">pxe</div>
            <div class="text-xs text-neutral-500">网络启动管理控制台</div>
          </div>
          <button class="btn h-9 w-9 p-0" aria-label="关闭导航" @click="mobileOpen = false">
            <X class="h-4 w-4" />
          </button>
        </div>
        <nav class="flex-1 space-y-1 p-3">
          <RouterLink v-for="item in nav" :key="item.path" :to="item.path" class="flex items-center gap-3 rounded-md px-3 py-2 text-sm text-neutral-700 transition hover:bg-neutral-100" active-class="bg-neutral-950 text-white hover:bg-neutral-950" @click="mobileOpen = false">
            <component :is="item.icon" class="h-4 w-4" />
            {{ item.name }}
          </RouterLink>
        </nav>
        <div class="border-t px-3 py-2">
          <a class="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-neutral-500 transition hover:bg-neutral-100 hover:text-neutral-950" href="https://github.com/sky22333/netboot" target="_blank" rel="noreferrer">
            <Github class="h-3.5 w-3.5" />
            GitHub
          </a>
        </div>
      </aside>
    </div>
    <div class="lg:pl-64">
      <header class="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-neutral-200 bg-white/90 px-4 backdrop-blur">
        <div class="flex min-w-0 items-center gap-3">
          <button class="btn h-9 w-9 p-0 lg:hidden" aria-label="打开导航" @click="mobileOpen = true">
            <Menu class="h-4 w-4" />
          </button>
          <div class="min-w-0">
            <div class="text-sm text-neutral-500">当前页面</div>
            <div class="truncate font-medium">{{ title }}</div>
          </div>
        </div>
        <button class="btn btn-primary" :disabled="refreshing" @click="refresh">{{ refreshing ? '已刷新' : '刷新状态' }}</button>
      </header>
      <main class="p-4 lg:p-6">
        <RouterView v-slot="{ Component }">
          <Transition name="page" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Activity, Files, Gauge, Github, HardDrive, ListTree, Menu, Network, ScrollText, Settings, TerminalSquare, Users, X } from 'lucide-vue-next'
import { api } from './lib/api'

const nav = [
  { path: '/', name: '仪表盘', icon: Gauge },
  { path: '/config', name: '服务配置', icon: Settings },
  { path: '/clients', name: '客户端', icon: Network },
  { path: '/menus', name: '启动菜单', icon: ListTree },
  { path: '/files', name: '文件管理', icon: Files },
  { path: '/netboot', name: 'netboot.xyz', icon: HardDrive },
  { path: '/actions', name: '操作菜单', icon: TerminalSquare },
  { path: '/users', name: '用户', icon: Users },
  { path: '/logs', name: '日志', icon: ScrollText },
  { path: '/diagnostics', name: '系统诊断', icon: Activity }
]

const route = useRoute()
const title = computed(() => nav.find((item) => item.path === route.path)?.name ?? 'pxe')
const authMode = ref<'loading' | 'setup' | 'login' | 'ready'>('loading')
const username = ref('admin')
const password = ref('')
const authError = ref('')
const authBusy = ref(false)
const refreshing = ref(false)
const mobileOpen = ref(false)

function refresh() {
  window.dispatchEvent(new CustomEvent('pxe-refresh'))
  refreshing.value = true
  window.setTimeout(() => { refreshing.value = false }, 900)
}

async function checkAuth() {
  try {
    const setup = await api<{ has_user: boolean }>('/setup/status')
    if (!setup.has_user) {
      authMode.value = 'setup'
      return
    }
    await api('/status')
    authMode.value = 'ready'
  } catch (e) {
    authError.value = e instanceof Error ? e.message : '无法连接到后端服务'
    authMode.value = authError.value.includes('请先登录') ? 'login' : 'login'
  }
}

async function submitAuth() {
  if (authBusy.value) return
  authBusy.value = true
  authError.value = ''
  try {
    if (authMode.value === 'setup') {
      await api('/setup', { method: 'POST', body: JSON.stringify({ username: username.value, password: password.value }) })
    }
    await api('/auth/login', { method: 'POST', body: JSON.stringify({ username: username.value, password: password.value }) })
    authMode.value = 'ready'
  } catch (error) {
    authError.value = error instanceof Error ? error.message : '操作失败'
  } finally {
    authBusy.value = false
  }
}

onMounted(checkAuth)
</script>
