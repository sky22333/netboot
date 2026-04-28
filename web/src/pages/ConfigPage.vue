<template>
  <div class="card p-5">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold">服务配置</h1>
        <p class="text-sm text-neutral-500">配置 DHCP、TFTP、HTTP Boot 和安全选项。</p>
      </div>
      <button class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中...' : '保存配置' }}</button>
    </div>
    <div v-if="config" class="mt-6 grid gap-5 lg:grid-cols-2">
      <div class="space-y-3">
        <h2 class="font-medium">网络</h2>
        <label class="label">监听 IP</label>
        <input v-model="config.server.listen_ip" class="input w-full" />
        <p class="text-xs text-neutral-500">0.0.0.0 表示监听所有网卡，适合 DHCP/ProxyDHCP 接收广播请求；通告 IP 才是客户端访问 TFTP/HTTP 的服务地址。</p>
        <label class="label">通告 IP</label>
        <input v-model="config.server.advertise_ip" class="input w-full" />
      </div>
      <div class="space-y-3">
        <h2 class="font-medium">DHCP</h2>
        <label class="flex items-center gap-2 text-sm"><input v-model="config.dhcp.enabled" type="checkbox" /> 启用 DHCP/ProxyDHCP</label>
        <select v-model="config.dhcp.mode" class="input w-full"><option value="proxy">ProxyDHCP</option><option value="dhcp">完整 DHCP</option></select>
        <div v-if="config.dhcp.mode === 'proxy'" class="alert">ProxyDHCP 不分配 IP，客户端 IP、网关和 DNS 仍由现有路由器/DHCP 服务提供。</div>
        <div v-else class="alert">完整 DHCP 会向局域网分配 IP，启动前请确认没有路由器或其他 DHCP 服务在同网段工作。</div>
        <label class="label">普通 DHCP 客户端</label>
        <select v-model="config.dhcp.non_pxe_action" class="input w-full">
          <option value="network_only">仅分配网络参数</option>
          <option value="ignore">忽略普通客户端</option>
        </select>
        <input v-model="config.dhcp.subnet_mask" class="input w-full" placeholder="子网掩码" />
        <p v-if="config.dhcp.mode === 'proxy'" class="text-xs text-neutral-500">子网掩码用于计算定向广播地址，例如通告 IP 为 10.43.180.193 且掩码为 255.255.255.0 时，会自动计算 10.43.180.255。</p>
        <template v-if="config.dhcp.mode === 'dhcp'">
          <input v-model="config.dhcp.pool_start" class="input w-full" placeholder="地址池起始" />
          <input v-model="config.dhcp.pool_end" class="input w-full" placeholder="地址池结束" />
          <input v-model="config.dhcp.router" class="input w-full" placeholder="网关" />
          <input v-model="dnsText" class="input w-full" placeholder="DNS，多个用逗号分隔" />
          <label class="flex items-center gap-2 text-sm"><input v-model="config.dhcp.detect_conflicts" type="checkbox" /> 启动完整 DHCP 前探测冲突</label>
        </template>
      </div>
      <div class="space-y-3">
        <h2 class="font-medium">TFTP</h2>
        <label class="flex items-center gap-2 text-sm"><input v-model="config.tftp.enabled" type="checkbox" /> 启用 TFTP</label>
        <input v-model="config.tftp.root" class="input w-full" />
        <div class="grid gap-2 sm:grid-cols-3">
          <input v-model.number="config.tftp.block_size_max" class="input w-full" type="number" placeholder="最大块大小" />
          <input v-model.number="config.tftp.retry_count" class="input w-full" type="number" placeholder="重试次数" />
          <input v-model.number="config.tftp.timeout_seconds" class="input w-full" type="number" placeholder="超时秒数" />
        </div>
        <label class="flex items-center gap-2 text-sm"><input v-model="config.tftp.allow_upload" type="checkbox" /> 允许 TFTP 上传</label>
        <input v-model.number="config.tftp.max_upload_bytes" class="input w-full" type="number" placeholder="上传大小限制，0 表示不限制" />
      </div>
      <div class="space-y-3">
        <h2 class="font-medium">HTTP Boot</h2>
        <label class="flex items-center gap-2 text-sm"><input v-model="config.httpboot.enabled" type="checkbox" /> 启用 HTTP Boot</label>
        <input v-model="config.httpboot.addr" class="input w-full" />
        <input v-model="config.httpboot.root" class="input w-full" />
        <label class="flex items-center gap-2 text-sm"><input v-model="config.httpboot.directory_listing" type="checkbox" /> 允许目录浏览</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="config.httpboot.range_requests" type="checkbox" /> 允许 Range 断点请求</label>
      </div>
      <div class="space-y-3">
        <h2 class="font-medium">BitTorrent</h2>
        <label class="flex items-center gap-2 text-sm"><input v-model="config.torrent.enabled" type="checkbox" /> 启用内置 Tracker</label>
        <input v-model="config.torrent.addr" class="input w-full" placeholder=":6969" />
      </div>
    </div>
    <p v-if="message" class="mt-4 text-sm" :class="error ? 'text-red-600' : 'text-neutral-600'">{{ message }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../lib/api'

const config = ref<any>()
const message = ref('')
const error = ref(false)
const saving = ref(false)
const dnsText = computed({
  get: () => config.value?.dhcp?.dns?.join(', ') ?? '',
  set: (value: string) => { if (config.value) config.value.dhcp.dns = value.split(',').map(v => v.trim()).filter(Boolean) }
})
async function load() { config.value = await api('/config') }
async function save() {
  if (config.value?.dhcp?.enabled && config.value?.dhcp?.mode === 'dhcp') {
    const ok = window.confirm('完整 DHCP 会向局域网分配 IP。请确认当前网络没有其他 DHCP 服务，是否继续保存？')
    if (!ok) return
  }
  saving.value = true
  error.value = false
  try {
    await api('/config/validate', { method: 'POST', body: JSON.stringify(config.value) })
    config.value = await api('/config', { method: 'PUT', body: JSON.stringify(config.value) })
    message.value = '配置已保存，相关服务需要重启后生效。'
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}
onMounted(load)
</script>
