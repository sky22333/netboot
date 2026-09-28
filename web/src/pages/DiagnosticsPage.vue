<template>
  <div class="space-y-6">
    <Card class="p-6">
      <div
        class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
      >
        <div>
          <h1 class="text-lg font-semibold">系统诊断</h1>
          <p class="mt-1 text-sm text-muted-foreground">
            检查运行路径、权限、网卡和 DHCP 冲突。运行日志请到日志页面查看。
          </p>
        </div>
        <div class="flex gap-2">
          <Button variant="outline" as-child
            ><RouterLink to="/logs">查看日志</RouterLink></Button
          >

          <Button variant="default" :disabled="probing" @click="probeDHCP">{{
            probing ? "探测中..." : "DHCP 探测"
          }}</Button>
        </div>
      </div>
      <p v-if="error" class="mt-3 text-sm text-red-600">{{ error }}</p>
    </Card>

    <section class="grid gap-4 lg:grid-cols-3">
      <Card class="p-4">
        <div class="text-sm text-muted-foreground">权限状态</div>
        <div class="mt-2 flex items-center gap-2 font-semibold">
          <span
            class="h-2.5 w-2.5 rounded-full"
            :class="
              permission.status === 'ok'
                ? 'bg-green-500'
                : permission.status === 'warning'
                  ? 'bg-amber-500'
                  : 'bg-neutral-300'
            "
          />
          {{ permission.label }}
        </div>
        <div class="mt-1 text-xs text-muted-foreground">
          {{ permission.detail }}
        </div>
      </Card>
      <Card class="p-4">
        <div class="text-sm text-muted-foreground">管理端地址</div>
        <div class="mt-2 truncate font-semibold">
          {{ data?.admin_addr || "-" }}
        </div>
      </Card>
      <Card class="p-4">
        <div class="text-sm text-muted-foreground">DHCP 冲突探测</div>
        <div class="mt-2 font-semibold" :class="dhcpStatusClass">
          {{ dhcpStatusText }}
        </div>
        <div
          v-if="exclusions.length"
          class="mt-1 text-xs text-muted-foreground"
        >
          已排除本程序通告 IP：{{ exclusions.join(", ") }}
        </div>
      </Card>
    </section>

    <Card class="p-6">
      <h2 class="font-semibold">运行路径</h2>
      <div class="mt-3 grid gap-2 text-sm">
        <div class="rounded-md border border-border p-3">
          <div class="text-xs text-muted-foreground">数据目录</div>
          <div class="mt-1 break-all font-medium">
            {{ data?.data_dir || "-" }}
          </div>
        </div>
        <div class="rounded-md border border-border p-3">
          <div class="text-xs text-muted-foreground">数据库</div>
          <div class="mt-1 break-all font-medium">{{ data?.db || "-" }}</div>
        </div>
      </div>
    </Card>

    <Card class="p-6">
      <h2 class="font-semibold">网卡信息</h2>
      <div
        class="mt-3 divide-y divide-neutral-100 rounded-md border border-border"
      >
        <div
          v-for="item in interfaces"
          :key="item.name"
          class="grid gap-2 p-3 text-sm lg:grid-cols-[14rem_minmax(0,1fr)]"
        >
          <div class="min-w-0">
            <div class="truncate font-medium" :title="item.name">
              {{ item.name }}
            </div>
            <div class="mt-1 flex flex-wrap gap-1">
              <Badge
                variant="outline"
                v-for="flag in flagList(item.flags)"
                :key="flag"
                class="rounded border border-border px-1.5 py-0.5 text-[11px] text-muted-foreground"
                >{{ flag }}</Badge
              >
            </div>
          </div>
          <div class="flex min-w-0 flex-wrap gap-1 overflow-hidden">
            <Badge
              variant="outline"
              v-for="ip in item.ips"
              :key="ip"
              class="rounded border border-border px-2 py-0.5 text-xs text-muted-foreground"
              >{{ ip }}</Badge
            >
          </div>
        </div>
        <div
          v-if="interfaces.length === 0"
          class="p-4 text-sm text-muted-foreground"
        >
          未读取到网卡信息。
        </div>
      </div>
    </Card>

    <Card class="p-6">
      <h2 class="font-semibold">DHCP 网卡探测结果</h2>
      <p class="mt-1 text-xs text-muted-foreground">
        {{ dhcp.dhcp_probe_note }}
      </p>
      <div
        class="mt-3 divide-y divide-neutral-100 rounded-md border border-border"
      >
        <div
          v-for="probe in dhcpProbes"
          :key="`${probe.interface}-${probe.ip}`"
          class="grid gap-3 p-3 text-sm lg:grid-cols-[18rem_minmax(0,1fr)]"
        >
          <div class="min-w-0">
            <div class="truncate font-medium" :title="probe.interface">
              {{ probe.interface }}
            </div>
            <div class="mt-1 text-xs text-muted-foreground">
              探测地址：{{ probe.ip }} → {{ probe.broadcast }}
            </div>
          </div>
          <div class="min-w-0">
            <div v-if="probeServers(probe).length" class="flex flex-wrap gap-2">
              <Badge
                variant="outline"
                v-for="server in probeServers(probe)"
                :key="server"
                class="rounded-md border border-amber-200 bg-amber-50 px-2.5 py-1 text-sm text-amber-800"
                >{{ server }}</Badge
              >
            </div>
            <div
              v-else-if="probe.error"
              class="break-words text-xs text-amber-700"
            >
              {{ probe.error }}
            </div>
            <div v-else class="text-xs text-muted-foreground">
              未收到 DHCP Offer。
            </div>
          </div>
        </div>
        <div
          v-if="dhcpProbeState === 'idle'"
          class="p-4 text-sm text-muted-foreground"
        >
          尚未探测，点击“DHCP 探测”开始。
        </div>
        <div v-else-if="probing" class="p-4 text-sm text-muted-foreground">
          正在探测 DHCP 服务…
        </div>
        <div
          v-else-if="dhcpProbes.length === 0"
          class="p-4 text-sm text-muted-foreground"
        >
          未找到可探测的 IPv4 网卡。
        </div>
      </div>
    </Card>

    <Card class="p-6">
      <h2 class="font-semibold">建议</h2>
      <div class="mt-3 grid gap-2">
        <div
          v-for="item in suggestions"
          :key="item"
          class="rounded-md border border-border p-3 text-sm text-neutral-700"
        >
          {{ item }}
        </div>
      </div>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { usePageRefresh } from "@/lib/pageRefresh";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { api } from "../lib/api";

type Permission = {
  admin_like: boolean;
  status: "ok" | "warning" | "unknown";
  label: string;
  detail: string;
};

type Diagnostics = {
  data_dir: string;
  db: string;
  admin_addr: string;
  is_admin: boolean;
  permission: Permission;
  interfaces: Array<{ name: string; flags: string; ips: string[] }>;
  suggestions: string[];
};

type DHCPDiagnostics = {
  dhcp_servers: string[];
  dhcp_interface_probes: Array<{
    interface: string;
    ip: string;
    broadcast: string;
    servers: string[];
    error?: string;
  }>;
  dhcp_probe_exclusions: string[];
  dhcp_probe_note: string;
};

function emptyDiagnostics(): Diagnostics {
  return {
    data_dir: "",
    db: "",
    admin_addr: "",
    is_admin: false,
    permission: {
      admin_like: false,
      status: "unknown",
      label: "等待诊断",
      detail: "点击重新诊断后会显示当前权限状态。",
    },
    interfaces: [],
    suggestions: [],
  };
}

const data = ref<Diagnostics>(emptyDiagnostics());
const dhcp = ref<DHCPDiagnostics>(emptyDHCPDiagnostics());
const loading = ref(false);
const probing = ref(false);
const error = ref("");
const dhcpProbeState = ref<"idle" | "done">("idle");
const interfaces = computed(() => data.value.interfaces);
const dhcpServers = computed(() => dhcp.value.dhcp_servers);
const dhcpProbes = computed(() => dhcp.value.dhcp_interface_probes);
const suggestions = computed(() => data.value.suggestions);
const exclusions = computed(() => dhcp.value.dhcp_probe_exclusions);
const permission = computed(() => data.value.permission);
const dhcpStatusText = computed(() => {
  if (probing.value) return "正在探测...";
  if (dhcpProbeState.value === "idle") return "尚未执行探测";
  return dhcpServers.value.length
    ? `发现 ${dhcpServers.value.length} 个 DHCP 服务`
    : "未发现额外 DHCP 服务";
});
const dhcpStatusClass = computed(() => {
  if (probing.value || dhcpProbeState.value === "idle")
    return "text-muted-foreground";
  return dhcpServers.value.length ? "text-amber-700" : "text-green-700";
});

function emptyDHCPDiagnostics(): DHCPDiagnostics {
  return {
    dhcp_servers: [],
    dhcp_interface_probes: [],
    dhcp_probe_exclusions: [],
    dhcp_probe_note:
      "DHCP 探测需要手动触发，不会在进入页面时自动发送网络探测包。",
  };
}

function flagList(flags: string) {
  return String(flags || "")
    .split("|")
    .filter(Boolean);
}

function probeServers(probe: { servers?: string[] | null }) {
  return Array.isArray(probe.servers) ? probe.servers : [];
}

async function load() {
  loading.value = true;
  error.value = "";
  data.value = emptyDiagnostics();
  try {
    data.value = await api<Diagnostics>("/diagnostics");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "诊断失败";
  } finally {
    loading.value = false;
  }
}

async function probeDHCP() {
  if (probing.value) return;
  probing.value = true;
  error.value = "";
  dhcp.value = emptyDHCPDiagnostics();
  try {
    dhcp.value = await api<DHCPDiagnostics>("/diagnostics/dhcp");
    dhcpProbeState.value = "done";
  } catch (e) {
    error.value = e instanceof Error ? e.message : "DHCP 探测失败";
  } finally {
    probing.value = false;
  }
}
usePageRefresh(load);
onMounted(load);
</script>
