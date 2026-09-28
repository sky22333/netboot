<template>
  <div class="space-y-6">
    <Skeleton
      v-if="busy && clients.length === 0"
      class="h-16 w-full"
      aria-label="正在加载"
    />
    <Card class="p-6">
      <div
        class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between"
      >
        <div>
          <h1 class="text-lg font-semibold">设备管理</h1>
          <p class="mt-1 text-sm text-muted-foreground">
            管理设备、绑定地址和远程唤醒。
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <Button variant="default" :disabled="busy" @click="newClient"
            >添加设备</Button
          >
        </div>
      </div>
      <div
        class="mt-4 grid items-end gap-4 lg:grid-cols-[minmax(0,1fr)_10rem_8rem_auto]"
      >
        <div class="min-w-0 space-y-2">
          <Label for="clientspage-1">名称前缀</Label
          ><Input
            id="clientspage-1"
            v-model.trim="batchPrefix"
            placeholder="名称前缀，例如 PC-"
          />
        </div>
        <div class="min-w-0 space-y-2">
          <Label for="clientspage-2">起始 IP</Label
          ><Input
            id="clientspage-2"
            v-model.trim="batchIP"
            placeholder="起始 IP"
          />
        </div>
        <div class="min-w-0 space-y-2">
          <Label for="clientspage-3">添加数量</Label
          ><Input
            id="clientspage-3"
            v-model.number="batchCount"
            type="number"
            min="1"
            max="1000"
          />
        </div>
        <Button variant="outline" :disabled="busy || !canBatch" @click="batch"
          >批量添加</Button
        >
      </div>
      <Feedback :message="message" :error="error" />
    </Card>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Card class="overflow-hidden">
        <div class="hidden overflow-x-auto md:block">
          <Table class="w-full min-w-[880px] table-fixed text-sm">
            <TableHeader
              class="border-b border-border bg-muted/40 text-left text-xs font-medium text-muted-foreground"
            >
              <TableRow>
                <TableHead class="w-[18%] px-4 py-3">名称</TableHead>
                <TableHead class="w-[16%] px-4 py-3">IP</TableHead>
                <TableHead class="w-[20%] px-4 py-3">MAC</TableHead>
                <TableHead class="w-[12%] px-4 py-3">状态</TableHead>
                <TableHead class="w-[16%] px-4 py-3">健康</TableHead>
                <TableHead class="w-[18%] px-4 py-3 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody class="divide-y divide-neutral-100">
              <TableRow
                v-for="client in clients"
                :key="client.id"
                class="hover:bg-muted/40"
                :class="selected?.id === client.id ? 'bg-muted/40' : ''"
              >
                <TableCell class="min-w-0 px-4 py-3">
                  <Button
                    variant="ghost"
                    class="max-w-full truncate font-medium h-auto whitespace-normal"
                    @click="select(client)"
                    >{{ client.name }}</Button
                  >
                </TableCell>
                <TableCell class="px-4 py-3 text-muted-foreground">{{
                  client.observed_ip || client.ip || "-"
                }}</TableCell>
                <TableCell class="px-4 py-3 text-muted-foreground">{{
                  client.mac || "待绑定"
                }}</TableCell>
                <TableCell class="px-4 py-3">
                  <Badge
                    variant="outline"
                    class="rounded-full border px-2 py-0.5 text-xs"
                    :class="statusClass(client.status)"
                    >{{ statusText[client.status] ?? client.status }}</Badge
                  >
                </TableCell>
                <TableCell class="px-4 py-3 text-muted-foreground"
                  >{{ client.disk_health || "-" }} /
                  {{ client.net_speed || "-" }}</TableCell
                >
                <TableCell class="px-4 py-3">
                  <div class="flex flex-nowrap justify-end gap-1">
                    <Button
                      variant="outline"
                      class="h-8 min-w-12 px-2"
                      :disabled="busy || !client.mac"
                      @click="wol(client)"
                      >唤醒</Button
                    >
                    <Button
                      variant="outline"
                      class="h-8 min-w-12 px-2"
                      @click="select(client)"
                      >详情</Button
                    >
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <div class="divide-y divide-neutral-100 md:hidden">
          <Button
            variant="ghost"
            v-for="client in clients"
            :key="client.id"
            class="flex w-full items-start justify-between gap-3 p-4 text-left text-sm hover:bg-muted/40 h-auto whitespace-normal"
            @click="select(client)"
          >
            <div class="min-w-0">
              <div class="truncate font-medium">{{ client.name }}</div>
              <div class="mt-1 text-xs text-muted-foreground">
                {{ client.observed_ip || client.ip || "暂无 IP" }} ·
                {{ client.mac || "待绑定" }}
              </div>
            </div>
            <Badge
              variant="outline"
              class="rounded-full border px-2 py-0.5 text-xs"
              :class="statusClass(client.status)"
              >{{ statusText[client.status] ?? client.status }}</Badge
            >
          </Button>
        </div>

        <div
          v-if="clients.length === 0 && !busy"
          class="p-10 text-center text-sm text-muted-foreground"
        >
          暂无设备，可手动添加或等待自动发现。
        </div>
      </Card>

      <Card class="p-4">
        <div class="flex items-center justify-between gap-3">
          <h2 class="font-medium">
            {{ editing.id ? "设备详情" : "添加设备" }}
          </h2>
          <Badge
            variant="outline"
            v-if="selected"
            class="rounded bg-neutral-100 px-2 py-1 text-xs text-muted-foreground"
            >ID {{ selected.id }}</Badge
          >
        </div>
        <div class="mt-4 space-y-4">
          <div>
            <Label for="clientspage-4">名称</Label>
            <Input
              id="clientspage-4"
              v-model.trim="editing.name"
              class="mt-1 w-full"
              placeholder="例如 PC-001"
            />
          </div>
          <div>
            <Label for="clientspage-5">静态绑定 IP</Label>
            <Input
              id="clientspage-5"
              v-model.trim="editing.ip"
              class="mt-1 w-full"
              placeholder="可留空，或填写静态绑定 IP"
            />
          </div>
          <div>
            <Label for="clientspage-6">MAC 地址</Label>
            <Input
              id="clientspage-6"
              v-model.trim="editing.mac"
              class="mt-1 w-full"
              placeholder="可留空，由管理员填写设备 MAC"
            />
          </div>
          <div class="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">
            <p>最近观测 IP：{{ editing.observed_ip || "-" }}</p>
            <p class="mt-1">
              固件：{{
                editing.firmware === "unknown" ? "未知" : editing.firmware
              }}
              · 状态：{{ statusText[editing.status] ?? editing.status }}
            </p>
            <p class="mt-1">观测信息由设备请求自动更新。</p>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <Button
              variant="default"
              :disabled="busy || !canSave"
              @click="saveClient"
              >{{ busy ? "保存中..." : "保存" }}</Button
            >
            <Button
              variant="outline"
              :disabled="busy || !editing.id || !editing.mac"
              @click="clearMac(editing)"
              >解除绑定</Button
            >
            <Button
              variant="outline"
              :disabled="busy || !editing.id || !editing.mac"
              @click="wol(editing)"
              >唤醒</Button
            >
            <Button
              variant="destructive"
              :disabled="busy || !editing.id"
              @click="remove(editing)"
              >删除</Button
            >
          </div>
        </div>
      </Card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { usePageRefresh } from "@/lib/pageRefresh";
import Feedback from "@/components/Feedback.vue";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { confirmAction } from "@/lib/confirm";
import { computed, onMounted, reactive, ref } from "vue";
import { api } from "../lib/api";

type Client = {
  id: number;
  seq: number;
  name: string;
  ip: string;
  observed_ip: string;
  mac: string;
  firmware: string;
  status: string;
  disk_health: string;
  net_speed: string;
  created_at: string;
  updated_at: string;
};

const clients = ref<Client[]>([]);
const selected = ref<Client | null>(null);
const editing = reactive<Client>(emptyClient());
const batchPrefix = ref("PC-");
const batchIP = ref("192.168.1.101");
const batchCount = ref(10);
const busy = ref(false);
const message = ref("");
const error = ref(false);
const statusText: Record<string, string> = {
  unknown: "未知",
  unassigned: "待绑定",
  online: "在线",
  offline: "离线",
  pxe: "PXE",
  ipxe: "iPXE",
};
const ipPattern = /^$|^(\d{1,3}\.){3}\d{1,3}$/;
const macPattern = /^$|^([0-9A-Fa-f]{2}[:-]?){5}[0-9A-Fa-f]{2}$/;
const canSave = computed(
  () =>
    editing.name.trim().length > 0 &&
    ipPattern.test(editing.ip) &&
    macPattern.test(editing.mac),
);
const canBatch = computed(
  () =>
    batchPrefix.value.trim().length > 0 &&
    ipPattern.test(batchIP.value) &&
    batchCount.value >= 1 &&
    batchCount.value <= 1000,
);

function emptyClient(): Client {
  return {
    id: 0,
    seq: 0,
    name: "",
    ip: "",
    observed_ip: "",
    mac: "",
    firmware: "unknown",
    status: "unknown",
    disk_health: "",
    net_speed: "",
    created_at: "",
    updated_at: "",
  };
}

async function load() {
  await run(async () => {
    await fetchClients();
    if (selected.value) {
      const current = clients.value.find(
        (item) => item.id === selected.value?.id,
      );
      if (current) select(current);
    }
  });
}

async function fetchClients() {
  const rows = await api<Client[]>("/clients");
  clients.value = Array.isArray(rows) ? rows : [];
}

async function run(task: () => Promise<void>, showBusy = true) {
  if (busy.value) return;
  busy.value = showBusy;
  error.value = false;
  message.value = "";
  try {
    await task();
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    busy.value = false;
  }
}

function select(client: Client) {
  selected.value = client;
  Object.assign(editing, { ...client });
}

function newClient() {
  selected.value = null;
  Object.assign(editing, emptyClient(), {
    name: `客户端${clients.value.length + 1}`,
  });
}

async function saveClient() {
  if (!canSave.value) return;
  await run(async () => {
    const path = editing.id ? `/clients/${editing.id}` : "/clients";
    const method = editing.id ? "PUT" : "POST";
    const saved = await api<Client>(path, {
      method,
      body: JSON.stringify(editing),
    });
    message.value = "设备已保存";
    await reloadAndSelect(saved.id);
  });
}

async function reloadAndSelect(id: number) {
  const rows = await api<Client[]>("/clients");
  clients.value = Array.isArray(rows) ? rows : [];
  const current = clients.value.find((item) => item.id === id);
  if (current) select(current);
}

async function batch() {
  if (!canBatch.value) return;
  if (
    !(await confirmAction(`确认批量创建 ${batchCount.value} 台待绑定客户端？`))
  )
    return;
  await run(async () => {
    const rows = await api<Client[]>("/clients/batch", {
      method: "POST",
      body: JSON.stringify({
        prefix: batchPrefix.value,
        ip_start: batchIP.value,
        count: batchCount.value,
      }),
    });
    message.value = `已创建 ${Array.isArray(rows) ? rows.length : 0} 台客户端。`;
    await fetchClients();
  });
}

async function clearMac(client: Client) {
  if (!client.id || !client.mac) return;
  if (!(await confirmAction(`确认清除 ${client.name} 的 MAC 绑定？`))) return;
  await run(async () => {
    await api(`/clients/${client.id}/clear-mac`, { method: "POST" });
    message.value = "MAC 绑定已清除。";
    await reloadAndSelect(client.id);
  });
}

type WOLResponse = { sent?: number };

async function wol(client: Client) {
  if (!client.id || !client.mac) return;
  await run(async () => {
    const res = await api<WOLResponse>(`/clients/${client.id}/wol`, {
      method: "POST",
    });
    message.value = `唤醒包已发送${res.sent ? `（${res.sent} 个目标）` : ""}。`;
  });
}

async function remove(client: Client) {
  if (!client.id) return;
  if (!(await confirmAction(`确认删除客户端 ${client.name}？此操作不可恢复。`)))
    return;
  await run(async () => {
    await api(`/clients/${client.id}`, { method: "DELETE" });
    selected.value = null;
    Object.assign(editing, emptyClient());
    message.value = "设备已删除";
    await fetchClients();
  });
}

function statusClass(status: string) {
  if (status === "online") return "border-green-200 bg-green-50 text-green-700";
  if (status === "pxe" || status === "ipxe")
    return "border-blue-200 bg-blue-50 text-blue-700";
  if (status === "unassigned")
    return "border-amber-200 bg-amber-50 text-amber-700";
  return "border-border bg-muted/40 text-muted-foreground";
}

usePageRefresh(load);
onMounted(load);
</script>
