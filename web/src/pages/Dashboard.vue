<template>
  <div class="space-y-6">
    <Skeleton
      v-if="!status && !error"
      class="h-32 w-full"
      aria-label="正在加载"
    />
    <h1 class="text-lg font-semibold">运行概览</h1>
    <section class="grid gap-4 md:grid-cols-4">
      <Card v-for="(value, key) in status?.services" :key="key" class="p-4">
        <div class="text-sm text-muted-foreground">
          {{ labels[key] ?? key }}
        </div>
        <div class="mt-2 flex items-center gap-2 text-lg font-semibold">
          <span
            class="h-2.5 w-2.5 rounded-full"
            :class="value === 'running' ? 'bg-green-500' : 'bg-neutral-300'"
          />
          {{
            value === "running"
              ? "运行中"
              : value === "failed"
                ? "运行失败"
                : "已停止"
          }}
        </div>
      </Card>
    </section>
    <Card class="p-6">
      <div
        class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between"
      >
        <div>
          <h2 class="text-lg font-semibold">服务控制</h2>
          <p class="mt-1 text-sm text-muted-foreground">
            启动或停止当前已启用的 PXE 服务。
          </p>
        </div>
        <div class="flex gap-2">
          <Button variant="default" :disabled="!canStart" @click="start">{{
            busy ? "处理中..." : anyRunning ? "已启动" : "启动服务"
          }}</Button>
          <Button variant="outline" :disabled="!canStop" @click="stop">{{
            busy ? "处理中..." : anyRunning ? "停止服务" : "已停止"
          }}</Button>
        </div>
      </div>
      <Feedback :message="message" :error="error" />
    </Card>
    <Card class="p-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="font-semibold">实时事件</h2>
          <p class="mt-0.5 text-xs text-muted-foreground">
            最近事件，完整记录见“运行日志”。
          </p>
        </div>
        <span class="shrink-0 text-xs text-muted-foreground">{{
          connected ? "实时连接正常" : "连接重试中"
        }}</span>
      </div>
      <div
        class="mt-3 divide-y divide-neutral-100 rounded-md border border-border"
      >
        <div
          v-for="event in compactEvents"
          :key="event.id"
          class="grid min-h-8 grid-cols-[4.8rem_5.2rem_1fr] items-center gap-2 px-2.5 py-1.5 text-xs"
        >
          <span class="text-muted-foreground">{{ shortTime(event.time) }}</span>
          <span class="truncate font-medium text-neutral-700">{{
            event.source
          }}</span>
          <span class="truncate text-neutral-700" :title="event.message">{{
            event.message
          }}</span>
        </div>
        <div
          v-if="compactEvents.length === 0"
          class="px-2.5 py-2 text-xs text-muted-foreground"
        >
          暂无事件。
        </div>
      </div>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { usePageRefresh } from "@/lib/pageRefresh";
import Feedback from "@/components/Feedback.vue";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { confirmAction } from "@/lib/confirm";
import { computed, onMounted, ref } from "vue";
import { api } from "../lib/api";
import { useEventLog } from "../lib/eventLog";

const labels: Record<string, string> = {
  dhcp: "完整 DHCP",
  proxy_dhcp_67: "ProxyDHCP 发现",
  proxy_dhcp: "ProxyDHCP 4011",
  tftp: "TFTP",
  httpboot: "HTTP Boot",
  smb: "SMB 共享",
};
const status = ref<any>();
const { recent, connected, load: loadEvents } = useEventLog();
const compactEvents = computed(() => recent.value.slice(-6));
const busy = ref(false);
const message = ref("");
const error = ref(false);
const services = computed<Record<string, string>>(
  () => status.value?.services ?? {},
);
const anyRunning = computed(() =>
  Object.values(services.value).some((value) => value === "running"),
);
const canStart = computed(() => !busy.value && !anyRunning.value);
const canStop = computed(() => !busy.value && anyRunning.value);

async function load() {
  try {
    status.value = await api("/status");
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "状态刷新失败";
  }
}
async function start() {
  if (!canStart.value) return;
  if (
    !(await confirmAction(
      "确认启动已启用的 PXE 服务？完整 DHCP、TFTP、HTTP 可能需要管理员权限并影响当前局域网。",
    ))
  )
    return;
  busy.value = true;
  error.value = false;
  message.value = "";
  try {
    status.value = await api("/services/start", { method: "POST" });
    message.value = "服务启动请求已完成。";
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "启动失败";
  } finally {
    await load();
    busy.value = false;
  }
}
async function stop() {
  if (!canStop.value) return;
  if (
    !(await confirmAction(
      "确认停止所有 PXE 服务？正在启动或传输的客户端可能会中断。",
    ))
  )
    return;
  busy.value = true;
  error.value = false;
  message.value = "";
  try {
    status.value = await api("/services/stop", { method: "POST" });
    message.value = "服务已停止。";
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "停止失败";
  } finally {
    await load();
    busy.value = false;
  }
}
function shortTime(value: string) {
  const match = value.match(/T(\d{2}:\d{2}:\d{2})/);
  return match?.[1] ?? value.slice(0, 19);
}

onMounted(() => {
  refreshAll();
});

async function refreshAll() {
  error.value = false;
  message.value = "";
  await Promise.all([load(), loadEvents(200)]);
}
usePageRefresh(refreshAll);
</script>
