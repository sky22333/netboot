<template>
  <div class="space-y-6">
    <Skeleton
      v-if="!config && !error"
      class="h-32 w-full"
      aria-label="正在加载"
    />
    <Card class="p-6">
      <div
        class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between"
      >
        <div>
          <h1 class="text-lg font-semibold">服务配置</h1>
          <p class="mt-1 text-sm text-muted-foreground">
            设置网络、启动文件和传输服务。
          </p>
        </div>
        <Button variant="default" :disabled="saving || !config" @click="save">{{
          saving ? "保存中..." : "保存配置"
        }}</Button>
      </div>
      <Feedback :message="message" :error="error" />
    </Card>

    <div v-if="config" class="grid gap-4 xl:grid-cols-2">
      <Card class="p-6">
        <h2 class="font-semibold">网络</h2>
        <div class="mt-4 space-y-4">
          <div>
            <Label for="configpage-1">监听 IP</Label>
            <Input
              id="configpage-1"
              v-model.trim="config.server.listen_ip"
              class="mt-1 w-full"
            />
            <p class="text-xs text-muted-foreground mt-1">
              0.0.0.0 表示监听所有网卡；通告 IP 才是客户端访问 TFTP/HTTP
              的地址。
            </p>
          </div>
          <div>
            <Label for="configpage-2">通告 IP</Label>
            <Input
              id="configpage-2"
              v-model.trim="config.server.advertise_ip"
              class="mt-1 w-full"
            />
          </div>
        </div>
      </Card>

      <Card class="p-6">
        <h2 class="font-semibold">DHCP</h2>
        <div class="mt-4 space-y-4">
          <Label for="configpage-3" class="flex items-center gap-2 text-sm"
            ><Switch id="configpage-3" v-model="config.dhcp.enabled" /> 启用
            DHCP/ProxyDHCP</Label
          >
          <div class="grid gap-2 sm:grid-cols-2">
            <div>
              <Label for="configpage-4">模式</Label>
              <NativeSelect
                id="configpage-4"
                v-model="config.dhcp.mode"
                class="mt-1 w-full"
              >
                <NativeSelectOption value="proxy">ProxyDHCP</NativeSelectOption>
                <NativeSelectOption value="dhcp">完整 DHCP</NativeSelectOption>
              </NativeSelect>
            </div>
            <div>
              <Label for="configpage-5">普通 DHCP 客户端</Label>
              <NativeSelect
                id="configpage-5"
                v-model="config.dhcp.non_pxe_action"
                class="mt-1 w-full"
              >
                <NativeSelectOption value="network_only"
                  >仅分配网络参数</NativeSelectOption
                >
                <NativeSelectOption value="ignore"
                  >忽略普通客户端</NativeSelectOption
                >
              </NativeSelect>
            </div>
          </div>
          <Alert
            ><AlertDescription>{{
              config.dhcp.mode === "proxy"
                ? "ProxyDHCP 只提供启动信息，IP 仍由现有路由器或 DHCP 服务分配。"
                : "完整 DHCP 会分配 IP，建议只在隔离网络中使用，避免与现有 DHCP 冲突。"
            }}</AlertDescription></Alert
          >
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-6">子网掩码</Label
              ><Input
                id="configpage-6"
                v-model.trim="config.dhcp.subnet_mask"
                class="w-full"
                placeholder="子网掩码"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-7">租约（秒）</Label
              ><Input
                id="configpage-7"
                v-model.number="config.dhcp.lease_time_seconds"
                class="w-full"
                type="number"
                min="300"
                placeholder="租约秒数"
              />
            </div>
          </div>
          <template v-if="config.dhcp.mode === 'dhcp'">
            <div class="grid gap-2 sm:grid-cols-2">
              <div class="min-w-0 space-y-2">
                <Label for="configpage-8">起始地址</Label
                ><Input
                  id="configpage-8"
                  v-model.trim="config.dhcp.pool_start"
                  class="w-full"
                  placeholder="地址池起始"
                />
              </div>
              <div class="min-w-0 space-y-2">
                <Label for="configpage-9">结束地址</Label
                ><Input
                  id="configpage-9"
                  v-model.trim="config.dhcp.pool_end"
                  class="w-full"
                  placeholder="地址池结束"
                />
              </div>
            </div>
            <div class="grid gap-2 sm:grid-cols-2">
              <div class="min-w-0 space-y-2">
                <Label for="configpage-10">网关</Label
                ><Input
                  id="configpage-10"
                  v-model.trim="config.dhcp.router"
                  class="w-full"
                  placeholder="网关"
                />
              </div>
              <div class="min-w-0 space-y-2">
                <Label for="configpage-11">DNS（逗号分隔）</Label
                ><Input
                  id="configpage-11"
                  v-model="dnsText"
                  class="w-full"
                  placeholder="DNS，多个用逗号分隔"
                />
              </div>
            </div>
            <Label for="configpage-12" class="flex items-center gap-2 text-sm"
              ><Switch
                id="configpage-12"
                v-model="config.dhcp.detect_conflicts"
              />
              启动完整 DHCP 前探测冲突</Label
            >
          </template>
        </div>
      </Card>

      <Card class="p-6">
        <h2 class="font-semibold">TFTP</h2>
        <div class="mt-4 space-y-4">
          <Label for="configpage-13" class="flex items-center gap-2 text-sm"
            ><Switch id="configpage-13" v-model="config.tftp.enabled" /> 启用
            TFTP</Label
          >
          <div class="space-y-2">
            <Label for="configpage-14">文件目录</Label
            ><Input
              id="configpage-14"
              v-model.trim="config.tftp.root"
              placeholder="TFTP 根目录"
            />
          </div>
          <div class="grid gap-2 sm:grid-cols-3">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-15">最大并发</Label
              ><Input
                id="configpage-15"
                v-model.number="config.tftp.max_transfers"
                class="w-full"
                type="number"
                min="1"
                placeholder="最大并发"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-16">数据块（字节）</Label
              ><Input
                id="configpage-16"
                v-model.number="config.tftp.block_size_max"
                class="w-full"
                type="number"
                min="512"
                placeholder="最大块大小"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-17">超时（秒）</Label
              ><Input
                id="configpage-17"
                v-model.number="config.tftp.timeout_seconds"
                class="w-full"
                type="number"
                min="1"
                placeholder="超时秒数"
              />
            </div>
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-18">重试次数</Label
              ><Input
                id="configpage-18"
                v-model.number="config.tftp.retry_count"
                class="w-full"
                type="number"
                min="1"
                placeholder="重试次数"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-19">上传上限（字节）</Label
              ><Input
                id="configpage-19"
                v-model.number="config.tftp.max_upload_bytes"
                class="w-full"
                type="number"
                min="0"
                placeholder="上传限制字节"
              />
            </div>
          </div>
          <Label for="configpage-20" class="flex items-center gap-2 text-sm"
            ><Switch id="configpage-20" v-model="config.tftp.allow_upload" />
            允许 TFTP 上传</Label
          >
        </div>
      </Card>

      <Card class="p-6">
        <h2 class="font-semibold">HTTP Boot</h2>
        <div class="mt-4 space-y-4">
          <Label for="configpage-21" class="flex items-center gap-2 text-sm"
            ><Switch id="configpage-21" v-model="config.httpboot.enabled" />
            启用 HTTP Boot</Label
          >
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-22">监听地址</Label
              ><Input
                id="configpage-22"
                v-model.trim="config.httpboot.addr"
                class="w-full"
                placeholder=":80"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-23">文件目录</Label
              ><Input
                id="configpage-23"
                v-model.trim="config.httpboot.root"
                class="w-full"
                placeholder="HTTP Boot 根目录"
              />
            </div>
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <Label for="configpage-24" class="flex items-center gap-2 text-sm"
              ><Switch
                id="configpage-24"
                v-model="config.httpboot.directory_listing"
              />
              允许目录浏览</Label
            >
            <Label for="configpage-25" class="flex items-center gap-2 text-sm"
              ><Switch
                id="configpage-25"
                v-model="config.httpboot.range_requests"
              />
              允许 Range 断点请求</Label
            >
          </div>
        </div>
      </Card>

      <Card class="p-6">
        <h2 class="font-semibold">启动文件</h2>
        <div class="mt-4 space-y-4">
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-26">BIOS</Label
              ><Input
                id="configpage-26"
                v-model.trim="config.boot_files.bios"
                class="w-full"
                placeholder="BIOS，例如 undionly.kpxe"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-27">UEFI x64</Label
              ><Input
                id="configpage-27"
                v-model.trim="config.boot_files.uefi_x64"
                class="w-full"
                placeholder="UEFI x64，例如 ipxe-x86_64.efi"
              />
            </div>
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-28">UEFI IA32</Label
              ><Input
                id="configpage-28"
                v-model.trim="config.boot_files.uefi_ia32"
                class="w-full"
                placeholder="UEFI IA32，自备，可留空"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-29">UEFI ARM64</Label
              ><Input
                id="configpage-29"
                v-model.trim="config.boot_files.uefi_arm64"
                class="w-full"
                placeholder="UEFI ARM64，例如 ipxe-arm64.efi"
              />
            </div>
          </div>
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-30">UEFI ARM32</Label
              ><Input
                id="configpage-30"
                v-model.trim="config.boot_files.uefi_arm32"
                class="w-full"
                placeholder="UEFI ARM32，自备，可留空"
              />
            </div>
          </div>
          <p class="text-xs text-muted-foreground">
            填写 TFTP 目录内对应架构的固件文件名。iPXE 后续脚本使用 HTTP
            目录下的 boot.ipxe。
          </p>
        </div>
      </Card>

      <Card class="p-6">
        <h2 class="font-semibold">SMB 共享</h2>
        <div class="mt-4 space-y-4">
          <Label for="configpage-31" class="flex items-center gap-2 text-sm"
            ><Switch id="configpage-31" v-model="config.smb.enabled" /> 启用 SMB
            共享</Label
          >
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="min-w-0 space-y-2">
              <Label for="configpage-32">共享名称</Label
              ><Input
                id="configpage-32"
                v-model.trim="config.smb.share_name"
                class="w-full"
                placeholder="共享名，例如 pxe"
              />
            </div>
            <div class="min-w-0 space-y-2">
              <Label for="configpage-33">访问权限</Label
              ><NativeSelect
                id="configpage-33"
                v-model="config.smb.permissions"
                class="w-full"
              >
                <NativeSelectOption value="read">只读</NativeSelectOption>
                <NativeSelectOption value="full">完全控制</NativeSelectOption>
              </NativeSelect>
            </div>
          </div>
          <div class="min-w-0 space-y-2">
            <Label for="configpage-34">共享目录</Label
            ><Input
              id="configpage-34"
              v-model.trim="config.smb.root"
              class="w-full"
              placeholder="共享目录"
            />
          </div>
          <p class="text-xs text-muted-foreground">
            Windows 自动创建共享；其他系统需手动配置 Samba。
          </p>
        </div>
      </Card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { onBeforeRouteLeave } from "vue-router";
import { usePageRefresh } from "@/lib/pageRefresh";
import { Alert, AlertDescription } from "@/components/ui/alert";
import Feedback from "@/components/Feedback.vue";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { confirmAction } from "@/lib/confirm";
import { computed, onMounted, ref } from "vue";
import { api } from "../lib/api";
import type { ServiceConfig } from "../lib/types";

const config = ref<ServiceConfig | null>(null);
const message = ref("");
const error = ref(false);
const saving = ref(false);
const savedConfig = ref("");
const dirty = computed(
  () => !!config.value && JSON.stringify(config.value) !== savedConfig.value,
);
const dnsText = computed({
  get: () => config.value?.dhcp?.dns?.join(", ") ?? "",
  set: (value: string) => {
    if (config.value)
      config.value.dhcp.dns = value
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean);
  },
});

async function load() {
  error.value = false;
  message.value = "";
  try {
    config.value = await api<ServiceConfig>("/config");
    savedConfig.value = JSON.stringify(config.value);
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "读取配置失败";
  }
}

async function save() {
  if (saving.value || !config.value) return;
  if (config.value.dhcp.enabled && config.value.dhcp.mode === "dhcp") {
    const ok = await confirmAction(
      "完整 DHCP 会向局域网分配 IP。请确认当前网络没有其他 DHCP 服务，是否继续保存？",
    );
    if (!ok) return;
  }
  saving.value = true;
  error.value = false;
  message.value = "";
  try {
    await api("/config/validate", {
      method: "POST",
      body: JSON.stringify(config.value),
    });
    config.value = await api<ServiceConfig>("/config", {
      method: "PUT",
      body: JSON.stringify(config.value),
    });
    savedConfig.value = JSON.stringify(config.value);
    message.value = "已保存，重启服务后生效。";
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "保存失败";
  } finally {
    saving.value = false;
  }
}

async function allowLeave() {
  return !dirty.value || (await confirmAction("配置未保存，是否放弃？"));
}
onBeforeRouteLeave(allowLeave);
usePageRefresh(async () => {
  if (!saving.value && (await allowLeave())) await load();
});
onMounted(load);
</script>
