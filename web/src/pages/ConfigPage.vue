<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-lg font-semibold">服务配置</h1>
          <p class="mt-1 text-sm text-neutral-500">配置 PXE 服务的网络、DHCP、TFTP、HTTP Boot、SMB 和 Tracker。</p>
        </div>
        <button class="btn btn-primary" :disabled="saving || !config" @click="save">{{ saving ? '保存中...' : '保存配置' }}</button>
      </div>
      <p v-if="message" class="mt-3 text-sm" :class="error ? 'text-red-600' : 'text-neutral-500'">{{ message }}</p>
    </section>

    <div v-if="config" class="grid gap-4 xl:grid-cols-2">
      <section class="card p-5">
        <h2 class="font-semibold">网络</h2>
        <div class="mt-4 space-y-3">
          <div>
            <label class="label">监听 IP</label>
            <input v-model.trim="config.server.listen_ip" class="input mt-1 w-full" />
            <p class="field-hint mt-1">0.0.0.0 表示监听所有网卡；通告 IP 才是客户端访问 TFTP/HTTP 的地址。</p>
          </div>
          <div>
            <label class="label">通告 IP</label>
            <input v-model.trim="config.server.advertise_ip" class="input mt-1 w-full" />
          </div>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="font-semibold">DHCP</h2>
        <div class="mt-4 space-y-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="config.dhcp.enabled" type="checkbox" /> 启用 DHCP/ProxyDHCP</label>
          <div class="grid gap-2 sm:grid-cols-2">
            <div>
              <label class="label">模式</label>
              <select v-model="config.dhcp.mode" class="input mt-1 w-full">
                <option value="proxy">ProxyDHCP</option>
                <option value="dhcp">完整 DHCP</option>
              </select>
            </div>
            <div>
              <label class="label">普通 DHCP 客户端</label>
              <select v-model="config.dhcp.non_pxe_action" class="input mt-1 w-full">
                <option value="network_only">仅分配网络参数</option>
                <option value="ignore">忽略普通客户端</option>
              </select>
            </div>
          </div>
          <div class="alert">{{ config.dhcp.mode === 'proxy' ? 'ProxyDHCP 只提供启动信息，IP 仍由现有路由器或 DHCP 服务分配。' : '完整 DHCP 会分配 IP，建议只在隔离网络中使用，避免与现有 DHCP 冲突。' }}</div>
          <div class="grid gap-2 sm:grid-cols-2">
            <input v-model.trim="config.dhcp.subnet_mask" class="input w-full" placeholder="子网掩码" />
            <input v-model.number="config.dhcp.lease_time_seconds" class="input w-full" type="number" min="300" placeholder="租约秒数" />
          </div>
          <template v-if="config.dhcp.mode === 'dhcp'">
            <div class="grid gap-2 sm:grid-cols-2">
              <input v-model.trim="config.dhcp.pool_start" class="input w-full" placeholder="地址池起始" />
              <input v-model.trim="config.dhcp.pool_end" class="input w-full" placeholder="地址池结束" />
            </div>
            <div class="grid gap-2 sm:grid-cols-2">
              <input v-model.trim="config.dhcp.router" class="input w-full" placeholder="网关" />
              <input v-model="dnsText" class="input w-full" placeholder="DNS，多个用逗号分隔" />
            </div>
            <label class="flex items-center gap-2 text-sm"><input v-model="config.dhcp.detect_conflicts" type="checkbox" /> 启动完整 DHCP 前探测冲突</label>
          </template>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="font-semibold">TFTP</h2>
        <div class="mt-4 space-y-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="config.tftp.enabled" type="checkbox" /> 启用 TFTP</label>
          <input v-model.trim="config.tftp.root" class="input w-full" placeholder="TFTP 根目录" />
          <div class="grid gap-2 sm:grid-cols-3">
            <input v-model.number="config.tftp.max_transfers" class="input w-full" type="number" min="1" placeholder="最大并发" />
            <input v-model.number="config.tftp.block_size_max" class="input w-full" type="number" min="512" placeholder="最大块大小" />
            <input v-model.number="config.tftp.timeout_seconds" class="input w-full" type="number" min="1" placeholder="超时秒数" />
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <input v-model.number="config.tftp.retry_count" class="input w-full" type="number" min="1" placeholder="重试次数" />
            <input v-model.number="config.tftp.max_upload_bytes" class="input w-full" type="number" min="0" placeholder="上传限制字节" />
          </div>
          <label class="flex items-center gap-2 text-sm"><input v-model="config.tftp.allow_upload" type="checkbox" /> 允许 TFTP 上传</label>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="font-semibold">HTTP Boot</h2>
        <div class="mt-4 space-y-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="config.httpboot.enabled" type="checkbox" /> 启用 HTTP Boot</label>
          <div class="grid gap-2 sm:grid-cols-2">
            <input v-model.trim="config.httpboot.addr" class="input w-full" placeholder=":80" />
            <input v-model.trim="config.httpboot.root" class="input w-full" placeholder="HTTP Boot 根目录" />
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <label class="flex items-center gap-2 text-sm"><input v-model="config.httpboot.directory_listing" type="checkbox" /> 允许目录浏览</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="config.httpboot.range_requests" type="checkbox" /> 允许 Range 断点请求</label>
          </div>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="font-semibold">SMB 共享</h2>
        <div class="mt-4 space-y-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="config.smb.enabled" type="checkbox" /> 启用 SMB 共享</label>
          <div class="grid gap-2 sm:grid-cols-2">
            <input v-model.trim="config.smb.share_name" class="input w-full" placeholder="共享名，例如 pxe" />
            <select v-model="config.smb.permissions" class="input w-full">
              <option value="read">只读</option>
              <option value="full">完全控制</option>
            </select>
          </div>
          <input v-model.trim="config.smb.root" class="input w-full" placeholder="共享目录" />
          <p class="field-hint">Windows 可自动创建系统共享；Linux/macOS/OpenWrt/Armbian 需要手动配置 Samba 或系统共享。</p>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="font-semibold">BitTorrent Tracker</h2>
        <div class="mt-4 space-y-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="config.torrent.enabled" type="checkbox" /> 启用内置 Tracker</label>
          <input v-model.trim="config.torrent.addr" class="input w-full" placeholder=":6969" />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../lib/api'
import type { ServiceConfig } from '../lib/types'

const config = ref<ServiceConfig | null>(null)
const message = ref('')
const error = ref(false)
const saving = ref(false)
const dnsText = computed({
  get: () => config.value?.dhcp?.dns?.join(', ') ?? '',
  set: (value: string) => { if (config.value) config.value.dhcp.dns = value.split(',').map((v) => v.trim()).filter(Boolean) }
})

async function load() {
  try {
    config.value = await api<ServiceConfig>('/config')
  } catch (e) {
    error.value = true
    message.value = e instanceof Error ? e.message : '读取配置失败'
  }
}

async function save() {
  if (saving.value || !config.value) return
  if (config.value.dhcp.enabled && config.value.dhcp.mode === 'dhcp') {
    const ok = window.confirm('完整 DHCP 会向局域网分配 IP。请确认当前网络没有其他 DHCP 服务，是否继续保存？')
    if (!ok) return
  }
  saving.value = true
  error.value = false
  try {
    await api('/config/validate', { method: 'POST', body: JSON.stringify(config.value) })
    config.value = await api<ServiceConfig>('/config', { method: 'PUT', body: JSON.stringify(config.value) })
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
