<template>
  <div class="space-y-6">
    <div>
      <h1 class="text-lg font-semibold">固件下载</h1>
      <p class="mt-1 text-sm text-muted-foreground">
        选择来源和设备架构，下载到服务器。
      </p>
    </div>
    <Feedback :message="message" :error="error" />
    <Skeleton
      v-if="loading && !sources.length"
      class="h-72 w-full"
      aria-label="正在加载固件"
    />
    <Tabs v-if="sources.length" v-model="activeSource" class="gap-4">
      <TabsList class="h-10 w-full sm:w-80" aria-label="固件来源">
        <TabsTrigger
          v-for="source in sources"
          :key="source.id"
          :value="source.id"
          :disabled="busy"
        >
          <Cpu v-if="source.id === 'project'" /><Globe v-else />{{
            source.name
          }}
        </TabsTrigger>
      </TabsList>
      <TabsContent
        v-for="source in sources"
        :key="source.id"
        :value="source.id"
        class="space-y-4"
      >
        <Card>
          <div
            class="flex flex-col gap-4 border-b p-5 sm:flex-row sm:items-center sm:justify-between"
          >
            <div class="min-w-0 space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="font-semibold">{{ source.name }}</h2>
                <Badge v-if="source.id === 'project'" variant="secondary"
                  >最新稳定版</Badge
                >
              </div>
              <p class="text-sm text-muted-foreground">
                {{ source.description }}
              </p>
              <a
                :href="source.website"
                target="_blank"
                rel="noreferrer"
                class="inline-flex items-center gap-1 text-xs text-muted-foreground underline-offset-4 hover:underline"
                >{{
                  source.id === "project" ? "查看 GitHub Release" : "访问官网"
                }}<ExternalLink class="size-3"
              /></a>
            </div>
            <div class="flex shrink-0 gap-2">
              <Button
                v-if="downloading"
                variant="ghost"
                @click="downloadController?.abort()"
                >取消下载</Button
              >
              <Button
                :disabled="busy || loading || !source.files.length"
                @click="download(source, source.files)"
                ><LoaderCircle
                  v-if="downloading"
                  class="animate-spin"
                /><Download v-else />{{
                  downloading ? "下载中" : "下载全部"
                }}</Button
              >
            </div>
          </div>
          <Table>
            <TableHeader
              ><TableRow
                ><TableHead class="px-5">启动路径</TableHead
                ><TableHead class="hidden px-5 sm:table-cell"
                  >本地状态</TableHead
                ><TableHead class="px-5 text-right">操作</TableHead></TableRow
              ></TableHeader
            >
            <TableBody>
              <TableRow v-for="file in source.files" :key="file.name">
                <TableCell class="px-5 py-4 whitespace-normal">
                  <div
                    class="break-all font-mono text-xs font-medium sm:text-sm"
                  >
                    {{ file.name }}
                  </div>
                  <div class="mt-2 flex flex-wrap items-center gap-2">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      :aria-label="`复制 ${file.name}`"
                      @click="copyPath(file.name)"
                      ><Copy
                    /></Button>
                    <Badge variant="outline">{{ file.architecture }}</Badge
                    ><span class="text-xs text-muted-foreground sm:hidden">{{
                      file.exists ? formatSize(file.size) : "未下载"
                    }}</span>
                  </div>
                  <p
                    v-if="resultFor(source.id, file.name)?.error"
                    class="mt-2 max-w-xl break-words text-xs text-destructive"
                    role="alert"
                  >
                    {{ resultFor(source.id, file.name)?.error }}
                  </p>
                </TableCell>
                <TableCell class="hidden px-5 py-4 sm:table-cell">
                  <Badge :variant="file.exists ? 'secondary' : 'outline'">{{
                    file.exists ? "已下载" : "未下载"
                  }}</Badge>
                  <p
                    v-if="file.exists"
                    class="mt-2 text-xs text-muted-foreground"
                  >
                    {{ formatSize(file.size) }} ·
                    {{ formatTime(file.modified) }}
                  </p>
                </TableCell>
                <TableCell class="px-5 py-4 text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :disabled="busy || loading"
                    @click="download(source, [file])"
                    ><LoaderCircle
                      v-if="pending.includes(file.name)"
                      class="animate-spin"
                    /><RefreshCw v-else-if="file.exists" /><Download v-else />{{
                      file.exists ? "更新" : "下载"
                    }}</Button
                  >
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
          <div
            class="space-y-2 border-t bg-muted/30 px-5 py-4 text-xs text-muted-foreground"
          >
            <div class="flex items-center gap-2">
              <FolderOpen class="size-4 shrink-0" /><span>保存目录</span>
            </div>
            <p class="break-all font-mono">{{ source.directory }}</p>
          </div>
        </Card>
        <Card class="p-5">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex items-center gap-1">
              <h2 class="text-sm font-medium">下载后如何使用</h2>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="启动说明"
                title="启动说明"
                @click="helpOpen = true"
                ><CircleHelp
              /></Button>
            </div>
            <Button variant="ghost" size="sm" as-child
              ><RouterLink to="/config">服务配置<ArrowRight /></RouterLink
            ></Button>
          </div>
          <p class="mt-1 text-sm text-muted-foreground">
            复制所需启动路径，填入“服务配置”的对应架构，保存并重启服务。
          </p>
        </Card>
      </TabsContent>
    </Tabs>
    <Dialog v-model:open="helpOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>固件启动说明</DialogTitle>
          <DialogDescription
            >使用自带菜单，或按需添加启动脚本。</DialogDescription
          >
        </DialogHeader>
        <Tabs default-value="usage" class="min-h-0 gap-4">
          <TabsList class="w-full">
            <TabsTrigger value="usage">如何启动</TabsTrigger>
            <TabsTrigger value="scripts">可选脚本</TabsTrigger>
          </TabsList>
          <div class="max-h-[55dvh] overflow-y-auto">
            <TabsContent value="usage" class="space-y-4">
              <Card class="p-4">
                <ol class="list-decimal space-y-3 pl-4 text-sm">
                  <li>下载固件，文件自动保存到 TFTP目录。</li>
                  <li>将文件名填入服务配置的对应架构。</li>
                  <li>保存并重启服务，从客户端网络启动。</li>
                </ol>
              </Card>
              <p class="text-sm text-muted-foreground">
                项目固件进入内置菜单；netboot.xyz
                默认使用在线菜单，通常无需额外脚本。
              </p>
              <p class="text-sm text-muted-foreground">
                服务器不指定后续脚本。使用无内置菜单的通用 iPXE
                时，需自行提供启动入口。
              </p>
            </TabsContent>
            <TabsContent value="scripts" class="space-y-4">
              <Card class="divide-y">
                <div v-for="script in helpScripts" :key="script.name">
                  <div class="space-y-1 px-4 py-3">
                    <p class="break-all font-mono text-xs font-medium">
                      {{ script.name }}
                    </p>
                    <p class="text-sm text-muted-foreground">
                      {{ script.description }}
                    </p>
                  </div>
                </div>
              </Card>
              <p class="text-xs text-muted-foreground">
                自动加载取决于固件和启动方式。netboot.xyz 会在本地 TFTP
                启动时，自动探测并启动上述文件。
              </p>
              <p class="text-xs text-muted-foreground">
                可选文件未找到不一定是故障，也无需额外创建空文件。
              </p>
            </TabsContent>
          </div>
        </Tabs>
        <DialogFooter class="items-center sm:justify-between">
          <div class="flex gap-3">
            <Button variant="link" size="sm" class="h-auto p-0 text-xs" as-child
              ><a
                href="https://ipxe.org/howto/chainloading"
                target="_blank"
                rel="noreferrer"
                >iPXE 文档<ExternalLink /></a
            ></Button>
            <Button variant="link" size="sm" class="h-auto p-0 text-xs" as-child
              ><a
                href="https://github.com/netbootxyz/netboot.xyz/blob/development/roles/netbootxyz/templates/disks/netboot.xyz.j2"
                target="_blank"
                rel="noreferrer"
                >netboot.xyz 文档<ExternalLink /></a
            ></Button>
          </div>
          <Button size="sm" @click="helpOpen = false">知道了</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { onBeforeRouteLeave } from "vue-router";
import {
  ArrowRight,
  CircleHelp,
  Copy,
  Cpu,
  Download,
  ExternalLink,
  FolderOpen,
  Globe,
  LoaderCircle,
  RefreshCw,
} from "@lucide/vue";
import { toast } from "vue-sonner";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import Feedback from "@/components/Feedback.vue";
import { api } from "@/lib/api";
import { confirmAction } from "@/lib/confirm";
import { usePageRefresh } from "@/lib/pageRefresh";

type FirmwareFile = {
  name: string;
  architecture: string;
  exists: boolean;
  size: number;
  modified?: string;
};
type Source = {
  id: string;
  name: string;
  description: string;
  website: string;
  directory: string;
  files: FirmwareFile[];
};
type DownloadResult = {
  file: string;
  ok: boolean;
  error?: string;
};
const sources = ref<Source[]>([]);
const helpOpen = ref(false);
const helpScripts = [
  {
    name: "autoexec.ipxe",
    description: "iPXE 自动脚本，通常与固件放在同一目录。",
  },
  { name: "local-vars.ipxe", description: "netboot.xyz 本地变量配置。" },
  {
    name: "HOSTNAME-主机名.ipxe / MAC-地址.ipxe",
    description: "netboot.xyz 按设备定制的启动脚本。",
  },
  { name: "menu.ipxe", description: "netboot.xyz 本地默认菜单。" },
];
const activeSource = ref("project");
const loading = ref(false);
const busy = ref(false);
const downloading = ref(false);
const pending = ref<string[]>([]);
const results = ref<Record<string, DownloadResult>>({});
const message = ref("");
const error = ref(false);
let downloadController: AbortController | undefined;
let catalogController: AbortController | undefined;
let disposed = false;

function resultFor(source: string, name: string) {
  return results.value[`${source}/${name}`];
}
async function fetchCatalog() {
  catalogController?.abort();
  catalogController = new AbortController();
  const data = await api<{ sources: Source[] }>("/firmware", {
    signal: catalogController.signal,
  });
  sources.value = Array.isArray(data.sources) ? data.sources : [];
}
async function load() {
  if (busy.value || loading.value) return;
  loading.value = true;
  message.value = "";
  error.value = false;
  try {
    await fetchCatalog();
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "读取固件失败";
  } finally {
    loading.value = false;
  }
}
async function download(source: Source, files: FirmwareFile[]) {
  if (busy.value || loading.value) return;
  busy.value = true;
  message.value = "";
  error.value = false;
  try {
    if (
      files.some((file) => file.exists) &&
      !(await confirmAction("将替换同名固件，启动配置保持不变。继续下载？"))
    )
      return;
    downloading.value = true;
    pending.value = files.map((file) => file.name);
    for (const file of files) delete results.value[`${source.id}/${file.name}`];
    downloadController = new AbortController();
    const data = await api<{ downloads: DownloadResult[] }>(
      "/firmware/download",
      {
        method: "POST",
        signal: downloadController.signal,
        body: JSON.stringify({ source: source.id, files: pending.value }),
      },
    );
    const downloads = Array.isArray(data.downloads) ? data.downloads : [];
    for (const result of downloads)
      results.value[`${source.id}/${result.file}`] = result;
    const failed = downloads.filter((result) => !result.ok).length;
    if (failed) {
      error.value = true;
      message.value = `${failed} 个固件下载失败，请查看文件下方的说明。`;
    } else {
      message.value = `已下载 ${downloads.length} 个固件`;
    }
  } catch (e) {
    error.value = true;
    message.value = downloadController?.signal.aborted
      ? "下载已取消，已完成的文件保留。"
      : e instanceof Error
        ? e.message
        : "下载失败";
  } finally {
    if (downloading.value && !disposed) {
      try {
        await fetchCatalog();
      } catch {
        error.value = true;
        message.value = "读取本地状态失败，请刷新页面。";
      }
    }
    busy.value = false;
    downloading.value = false;
    pending.value = [];
    downloadController = undefined;
  }
}
async function copyPath(path: string) {
  try {
    await navigator.clipboard.writeText(path);
    toast.success("启动路径已复制");
  } catch {
    toast.error("复制失败，请手动复制路径");
  }
}
function formatSize(size: number) {
  return size < 1024 * 1024
    ? `${(size / 1024).toFixed(1)} KiB`
    : `${(size / 1024 / 1024).toFixed(1)} MiB`;
}
function formatTime(value?: string) {
  return value ? new Date(value).toLocaleDateString("zh-CN") : "";
}
onBeforeRouteLeave(
  async () =>
    !downloading.value ||
    (await confirmAction("离开页面将取消下载，是否继续？")),
);
onBeforeUnmount(() => {
  disposed = true;
  downloadController?.abort();
  catalogController?.abort();
});
usePageRefresh(load);
onMounted(load);
</script>
