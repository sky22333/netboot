<template>
  <div class="space-y-4">
    <div class="card p-5">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-lg font-semibold">系统诊断</h1>
          <p class="mt-1 text-sm text-neutral-500">检查运行路径、权限、网卡和 DHCP 冲突。实时日志请到日志页面查看。</p>
        </div>
        <div class="flex gap-2">
          <RouterLink class="btn" to="/logs">查看日志</RouterLink>
          <button class="btn" :disabled="loading" @click="load">{{ loading ? '诊断中...' : '重新诊断' }}</button>
        </div>
      </div>
      <p v-if="error" class="mt-3 text-sm text-red-600">{{ error }}</p>
    </div>

    <section class="grid gap-4 lg:grid-cols-3">
      <div class="card p-4">
        <div class="text-sm text-neutral-500">权限状态</div>
        <div class="mt-2 flex items-center gap-2 font-semibold">
          <span class="h-2.5 w-2.5 rounded-full" :class="data?.is_admin ? 'bg-green-500' : 'bg-amber-500'" />
          {{ data?.is_admin ? '具备低端口权限' : '可能缺少低端口权限' }}
        </div>
      </div>
      <div class="card p-4">
        <div class="text-sm text-neutral-500">管理端地址</div>
        <div class="mt-2 truncate font-semibold">{{ data?.admin_addr || '-' }}</div>
      </div>
      <div class="card p-4">
        <div class="text-sm text-neutral-500">DHCP 冲突探测</div>
        <div class="mt-2 font-semibold" :class="dhcpServers.length ? 'text-amber-700' : 'text-green-700'">
          {{ dhcpServers.length ? `发现 ${dhcpServers.length} 个 DHCP 服务` : '未发现额外 DHCP 服务' }}
        </div>
        <div class="mt-1 text-xs text-neutral-500">已排除本程序通告 IP。</div>
      </div>
    </section>

    <section class="card p-5">
      <h2 class="font-semibold">运行路径</h2>
      <div class="mt-3 grid gap-2 text-sm">
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-xs text-neutral-500">数据目录</div>
          <div class="mt-1 break-all font-medium">{{ data?.data_dir || '-' }}</div>
        </div>
        <div class="rounded-md border border-neutral-200 p-3">
          <div class="text-xs text-neutral-500">数据库</div>
          <div class="mt-1 break-all font-medium">{{ data?.db || '-' }}</div>
        </div>
      </div>
    </section>

    <section class="card p-5">
      <h2 class="font-semibold">网卡信息</h2>
      <div class="mt-3 divide-y divide-neutral-100 rounded-md border border-neutral-200">
        <div v-for="item in interfaces" :key="item.name" class="grid gap-2 p-3 text-sm lg:grid-cols-[14rem_minmax(0,1fr)]">
          <div class="min-w-0">
            <div class="truncate font-medium" :title="item.name">{{ item.name }}</div>
            <div class="mt-1 flex flex-wrap gap-1">
              <span v-for="flag in flagList(item.flags)" :key="flag" class="rounded border border-neutral-200 px-1.5 py-0.5 text-[11px] text-neutral-500">{{ flag }}</span>
            </div>
          </div>
          <div class="flex min-w-0 flex-wrap gap-1">
            <span v-for="ip in item.ips" :key="ip" class="rounded border border-neutral-200 px-2 py-0.5 text-xs text-neutral-600">{{ ip }}</span>
          </div>
        </div>
        <div v-if="interfaces.length === 0" class="p-4 text-sm text-neutral-500">未读取到网卡信息。</div>
      </div>
    </section>

    <section v-if="dhcpServers.length" class="card p-5">
      <h2 class="font-semibold">探测到的 DHCP 服务</h2>
      <p class="mt-1 text-xs text-neutral-500">{{ data?.dhcp_probe_note }}</p>
      <div class="mt-3 flex flex-wrap gap-2">
        <span v-for="server in dhcpServers" :key="server" class="rounded-md border border-amber-200 bg-amber-50 px-2.5 py-1 text-sm text-amber-800">{{ server }}</span>
      </div>
    </section>
    <section v-else class="card p-5">
      <h2 class="font-semibold">DHCP 探测说明</h2>
      <p class="mt-2 text-sm text-neutral-600">{{ data?.dhcp_probe_note }}</p>
    </section>

    <section class="card p-5">
      <h2 class="font-semibold">建议</h2>
      <div class="mt-3 grid gap-2">
        <div v-for="item in suggestions" :key="item" class="rounded-md border border-neutral-200 p-3 text-sm text-neutral-700">{{ item }}</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../lib/api'

const data = ref<any>()
const loading = ref(false)
const error = ref('')
const interfaces = computed(() => Array.isArray(data.value?.interfaces) ? data.value.interfaces : [])
const dhcpServers = computed(() => Array.isArray(data.value?.dhcp_servers) ? data.value.dhcp_servers : [])
const suggestions = computed(() => Array.isArray(data.value?.suggestions) ? data.value.suggestions : [])

function flagList(flags: string) {
  return String(flags || '').split('|').filter(Boolean)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api('/diagnostics')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '诊断失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
