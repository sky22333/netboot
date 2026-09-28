<template>
  <div class="space-y-6">
    <Skeleton
      v-if="busy && files.length === 0"
      class="h-16 w-full"
      aria-label="正在加载"
    />
    <Card class="p-6">
      <div
        class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between"
      >
        <div>
          <h1 class="text-lg font-semibold">文件管理</h1>
          <p class="mt-1 text-sm text-muted-foreground">
            管理启动脚本、固件和镜像。
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <Button
            :variant="root === item.key ? 'default' : 'outline'"
            v-for="item in roots"
            :key="item.key"
            class="gap-2"
            @click="switchRoot(item.key)"
          >
            <component :is="item.icon" class="h-4 w-4" />
            {{ item.label }}
          </Button>
        </div>
      </div>

      <div class="mt-4 rounded-md border border-border bg-muted/40 p-3">
        <div
          class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between"
        >
          <div class="min-w-0">
            <div class="text-sm font-medium">{{ activeRoot.label }}</div>
            <p class="mt-1 break-all text-xs text-muted-foreground">
              {{ activeRoot.description }}
            </p>
            <p class="mt-1 break-all text-xs text-muted-foreground">
              本地目录：{{ basePath || activeRoot.localPath }}
            </p>
          </div>
          <Button
            variant="outline"
            class="shrink-0 gap-2"
            :disabled="!accessExample"
            @click="copyText(accessExample)"
          >
            <Copy class="h-4 w-4" />
            复制访问示例
          </Button>
        </div>
      </div>
    </Card>

    <Card v-if="uploadController" class="p-4 flex-row items-center gap-3">
      <Progress class="flex-1" :model-value="uploadProgress" :max="100" />
      <span
        >{{ uploadProgress }}%{{
          uploadProgress === 100 ? " · 正在保存" : ""
        }}</span
      >
      <Button variant="outline" @click="uploadController?.abort()"
        >取消上传</Button
      >
    </Card>
    <p v-if="maxUploadBytes" class="text-xs text-muted-foreground">
      单文件上传上限：{{ formatSize(maxUploadBytes) }}
    </p>
    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
      <Card class="overflow-hidden">
        <div
          class="flex flex-col gap-3 border-b border-border p-4 lg:flex-row lg:items-center lg:justify-between"
        >
          <div class="min-w-0">
            <div class="text-xs text-muted-foreground">当前位置</div>
            <div class="mt-1 flex flex-wrap items-center gap-1 text-sm">
              <Button
                variant="ghost"
                class="rounded px-2 py-1 font-medium hover:bg-neutral-100 h-auto whitespace-normal"
                @click="goPath('.')"
                >{{ activeRoot.label }}</Button
              >
              <template v-for="crumb in crumbs" :key="crumb.path">
                <ChevronRight class="h-3.5 w-3.5 text-neutral-400" />
                <Button
                  variant="ghost"
                  class="rounded px-2 py-1 hover:bg-neutral-100 h-auto whitespace-normal"
                  @click="goPath(crumb.path)"
                  >{{ crumb.name }}</Button
                >
              </template>
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button
              variant="outline"
              class="gap-2"
              :disabled="busy"
              @click="openCreateDir"
            >
              <FolderPlus class="h-4 w-4" />
              新建目录
            </Button>
            <Button
              variant="outline"
              class="gap-2"
              :disabled="busy"
              @click="openCreateFile"
            >
              <FilePlus2 class="h-4 w-4" />
              新建文件
            </Button>
            <Button
              variant="outline"
              :disabled="busy"
              @click="uploadInput?.click()"
              ><Upload class="size-4" />上传文件</Button
            >
            <input
              ref="uploadInput"
              class="hidden"
              type="file"
              aria-label="上传文件"
              :disabled="busy"
              @change="onFile"
            />
          </div>
        </div>

        <div class="hidden overflow-x-auto md:block">
          <Table class="w-full min-w-[720px] table-fixed text-sm">
            <TableHeader
              class="border-b border-border bg-muted/40 text-left text-xs font-medium text-muted-foreground"
            >
              <TableRow>
                <TableHead class="w-[34%] px-4 py-3">名称</TableHead>
                <TableHead class="w-[14%] px-4 py-3">类型</TableHead>
                <TableHead class="w-[12%] px-4 py-3">大小</TableHead>
                <TableHead class="w-[24%] px-4 py-3">修改时间</TableHead>
                <TableHead class="w-[16%] px-4 py-3 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody class="divide-y divide-neutral-100">
              <TableRow v-if="currentPath !== '.'" class="hover:bg-muted/40">
                <TableCell class="px-4 py-3" colspan="5">
                  <Button
                    variant="ghost"
                    class="flex items-center gap-2 text-sm font-medium h-auto whitespace-normal"
                    @click="goUp"
                  >
                    <CornerUpLeft class="h-4 w-4" />
                    返回上一级
                  </Button>
                </TableCell>
              </TableRow>
              <TableRow
                v-for="file in sortedFiles"
                :key="file.name"
                class="hover:bg-muted/40"
                :class="selected?.name === file.name ? 'bg-muted/40' : ''"
              >
                <TableCell class="min-w-0 px-4 py-3">
                  <Button
                    variant="ghost"
                    class="flex max-w-full items-center gap-2 h-auto whitespace-normal"
                    @click="selectFile(file)"
                  >
                    <Folder
                      v-if="file.dir"
                      class="h-4 w-4 shrink-0 text-amber-600"
                    />
                    <FileText
                      v-else
                      class="h-4 w-4 shrink-0 text-muted-foreground"
                    />
                    <span class="truncate font-medium">{{ file.name }}</span>
                  </Button>
                </TableCell>
                <TableCell class="px-4 py-3 text-muted-foreground">{{
                  file.dir ? "目录" : filePurpose(file.name)
                }}</TableCell>
                <TableCell class="px-4 py-3 text-muted-foreground">{{
                  file.dir ? "-" : formatSize(file.size)
                }}</TableCell>
                <TableCell class="px-4 py-3 text-muted-foreground">{{
                  formatTime(file.mod_time)
                }}</TableCell>
                <TableCell class="px-4 py-3">
                  <div class="flex justify-end gap-1">
                    <Button
                      variant="outline"
                      class="h-8 px-2"
                      :disabled="file.dir || !file.editable"
                      aria-label="编辑"
                      title="编辑"
                      @click.stop="editFile(file)"
                    >
                      <Pencil class="h-4 w-4" />
                    </Button>
                    <Button
                      variant="outline"
                      class="h-8 px-2"
                      aria-label="文件详情"
                      title="文件详情"
                      @click.stop="selectFile(file)"
                    >
                      <Info class="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <div class="divide-y divide-neutral-100 md:hidden">
          <Button
            variant="ghost"
            v-if="currentPath !== '.'"
            class="flex w-full items-center gap-2 p-4 text-left text-sm font-medium h-auto whitespace-normal"
            @click="goUp"
          >
            <CornerUpLeft class="h-4 w-4" />
            返回上一级
          </Button>
          <Button
            variant="ghost"
            v-for="file in sortedFiles"
            :key="file.name"
            class="flex w-full items-start justify-between gap-3 p-4 text-left text-sm hover:bg-muted/40 h-auto whitespace-normal"
            @click="selectFile(file)"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <Folder
                  v-if="file.dir"
                  class="h-4 w-4 shrink-0 text-amber-600"
                />
                <FileText
                  v-else
                  class="h-4 w-4 shrink-0 text-muted-foreground"
                />
                <span class="truncate font-medium">{{ file.name }}</span>
              </div>
              <div class="mt-1 text-xs text-muted-foreground">
                {{
                  file.dir
                    ? "目录"
                    : `${filePurpose(file.name)} · ${formatSize(file.size)}`
                }}
              </div>
            </div>
            <ChevronRight class="mt-0.5 h-4 w-4 shrink-0 text-neutral-400" />
          </Button>
        </div>

        <div
          v-if="sortedFiles.length === 0"
          class="p-10 text-center text-sm text-muted-foreground"
        >
          当前目录为空，可以上传文件或新建目录。
        </div>
      </Card>

      <aside class="space-y-6">
        <Card class="p-4">
          <div class="flex items-center justify-between gap-3">
            <h2 class="font-medium">文件详情</h2>
            <Badge
              variant="outline"
              class="rounded bg-neutral-100 px-2 py-1 text-xs text-muted-foreground"
              >{{ selected ? "已选择" : "未选择" }}</Badge
            >
          </div>
          <div v-if="selected" class="mt-4 space-y-4 text-sm">
            <div>
              <div class="text-xs text-muted-foreground">相对路径</div>
              <div class="mt-1 break-all font-medium">
                {{ selectedFullPath }}
              </div>
            </div>
            <div>
              <div class="text-xs text-muted-foreground">访问路径</div>
              <div class="mt-1 break-all rounded-md bg-muted/40 p-2 text-xs">
                {{ selectedAccessPath }}
              </div>
            </div>
            <div class="grid grid-cols-2 gap-2">
              <Button
                variant="outline"
                class="gap-2"
                :disabled="!selectedAccessPath"
                @click="copyText(selectedAccessPath)"
              >
                <Copy class="h-4 w-4" />
                复制路径
              </Button>
              <Button
                variant="outline"
                class="gap-2"
                :disabled="selected.dir || !selected.editable"
                @click="editFile(selected)"
              >
                <Pencil class="h-4 w-4" />
                编辑
              </Button>
              <Button
                variant="outline"
                class="gap-2"
                :disabled="selected.dir"
                @click="startRename"
              >
                <MoveRight class="h-4 w-4" />
                重命名
              </Button>
              <Button
                variant="destructive"
                class="gap-2"
                @click="remove(selectedFullPath)"
              >
                <Trash2 class="h-4 w-4" />
                删除
              </Button>
            </div>
          </div>
          <p v-else class="mt-4 text-sm text-muted-foreground">
            选择文件查看详情。
          </p>
        </Card>

        <Feedback :message="message" :error="error" />
      </aside>
    </div>

    <Dialog
      :open="editorOpen && editing"
      @update:open="
        (open) => {
          if (!open) closeEditorWindow();
        }
      "
    >
      <DialogContent
        :show-close-button="false"
        class="flex h-[85svh] w-[calc(100%-2rem)] max-w-6xl flex-col gap-0 overflow-hidden p-0 sm:max-w-6xl"
        @interact-outside.prevent
        @escape-key-down.prevent="closeEditorWindow"
      >
        <DialogHeader class="border-b p-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <DialogTitle
                >编辑文件
                <Badge v-if="dirty" variant="outline"
                  >未保存</Badge
                ></DialogTitle
              ><DialogDescription class="mt-2 break-all">{{
                editingPath
              }}</DialogDescription>
            </div>
            <div class="flex gap-2">
              <Button :disabled="busy || !dirty" @click="saveContent"
                ><Save />保存</Button
              ><Button
                variant="outline"
                :disabled="busy"
                @click="closeEditorWindow"
                >关闭</Button
              >
            </div>
          </div>
        </DialogHeader>
        <div
          class="flex flex-wrap justify-between gap-2 border-b bg-muted/40 px-4 py-2 text-xs text-muted-foreground"
        >
          <span>{{ editorStats }}</span
          ><span>UTF-8 · 最大 1 MiB · Ctrl/⌘ + S 保存</span>
        </div>
        <Textarea
          ref="editorRef"
          v-model="editorContent"
          aria-label="文件内容"
          class="min-h-0 flex-1 resize-none rounded-none border-0 p-4 font-mono text-sm leading-6 focus-visible:ring-inset"
          spellcheck="false"
          @keydown.ctrl.s.prevent="saveContent"
          @keydown.meta.s.prevent="saveContent"
        />
      </DialogContent>
    </Dialog>
    <Dialog
      :open="!!dialog"
      @update:open="
        (open) => {
          if (!open && !busy) dialog = '';
        }
      "
    >
      <DialogContent
        @interact-outside="
          (event) => {
            if (busy) event.preventDefault();
          }
        "
      >
        <DialogHeader
          ><DialogTitle>{{ dialogTitle }}</DialogTitle
          ><DialogDescription>{{ dialogHint }}</DialogDescription></DialogHeader
        >
        <form class="space-y-4" @submit.prevent="confirmDialog">
          <Label for="file-name">{{
            dialog === "rename" ? "新路径" : "名称"
          }}</Label>
          <Input
            id="file-name"
            v-model="dialogValue"
            :placeholder="dialogPlaceholder"
            :disabled="busy"
            autocomplete="off"
          />
          <DialogFooter
            ><Button
              type="button"
              variant="outline"
              :disabled="busy"
              @click="dialog = ''"
              >取消</Button
            ><Button type="submit" :disabled="busy || !dialogValue.trim()">{{
              busy ? "保存中" : "确定"
            }}</Button></DialogFooter
          >
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { usePageRefresh } from "@/lib/pageRefresh";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { onBeforeRouteLeave } from "vue-router";

import Feedback from "@/components/Feedback.vue";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import {
  Table,
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { confirmAction } from "@/lib/confirm";
import { computed, nextTick, onMounted, onBeforeUnmount, ref } from "vue";
import {
  ChevronRight,
  Copy,
  CornerUpLeft,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  Globe2,
  HardDrive,
  Info,
  MoveRight,
  Pencil,
  Save,
  Trash2,
  Upload,
} from "@lucide/vue";
import { api, upload } from "../lib/api";
import type { ServiceConfig } from "../lib/types";

type RootKey = "http" | "tftp" | "netboot";

type FileEntry = {
  name: string;
  dir: boolean;
  size: number;
  mod_time: string;
  editable?: boolean;
};

type FileListResponse = {
  root: RootKey;
  path: string;
  max_upload_bytes: number;
  base_path: string;
  files: FileEntry[];
};

const roots = [
  {
    key: "http" as RootKey,
    label: "HTTP Boot",
    localPath: "data/boot/http",
    icon: Globe2,
    description: "存放启动脚本和系统镜像，通过 HTTP 访问。",
  },
  {
    key: "tftp" as RootKey,
    label: "TFTP 启动",
    localPath: "data/boot/tftp",
    icon: HardDrive,
    description:
      "放 undionly.kpxe、ipxe-x86_64.efi、ipxe-arm64.efi 等第一阶段或 TFTP 引导文件。",
  },
  {
    key: "netboot" as RootKey,
    label: "netboot.xyz",
    localPath: "data/boot/netboot",
    icon: FileText,
    description:
      "存放 netboot.xyz 官方固件；到服务配置中填写 netboot/文件名后使用。",
  },
];

const locationStorageKey = "pxe.files.location";
const initialLocation = loadSavedLocation();

const root = ref<RootKey>(initialLocation.root);
const currentPath = ref(initialLocation.path);
const basePath = ref("");
const files = ref<FileEntry[]>([]);
const selected = ref<FileEntry | null>(null);
const message = ref("");
const error = ref(false);
const busy = ref(false);
const uploadInput = ref<HTMLInputElement | null>(null);
const uploadProgress = ref(0);
const uploadController = ref<AbortController | null>(null);
const maxUploadBytes = ref(0);
onBeforeUnmount(() => uploadController.value?.abort());
const dialog = ref<"mkdir" | "file" | "rename" | "">("");
const dialogValue = ref("");
const config = ref<ServiceConfig | null>(null);
const editing = ref(false);
const editorOpen = ref(false);
const editingPath = ref("");
const editorContent = ref("");
const originalContent = ref("");
const editorRef = ref<InstanceType<typeof Textarea> | null>(null);

const activeRoot = computed(
  () => roots.find((item) => item.key === root.value) ?? roots[0],
);
const sortedFiles = computed(() =>
  [...files.value].sort(
    (a, b) =>
      Number(b.dir) - Number(a.dir) ||
      a.name.localeCompare(b.name, "zh-Hans-CN"),
  ),
);
const dirty = computed(() => editorContent.value !== originalContent.value);
const editorStats = computed(() => {
  const bytes = new Blob([editorContent.value]).size;
  const lines =
    editorContent.value === "" ? 1 : editorContent.value.split("\n").length;
  return `${lines} 行 · ${formatSize(bytes)}`;
});
const crumbs = computed(() => {
  if (currentPath.value === ".") return [];
  const parts = currentPath.value.split("/").filter(Boolean);
  return parts.map((name, index) => ({
    name,
    path: parts.slice(0, index + 1).join("/"),
  }));
});
const selectedFullPath = computed(() =>
  selected.value ? fullPath(selected.value.name) : "",
);
const selectedAccessPath = computed(() =>
  selected.value ? accessPath(selectedFullPath.value) : "",
);
const accessExample = computed(() => {
  if (root.value === "http") return `${httpBase()}/boot.ipxe`;
  if (root.value === "tftp") return "undionly.kpxe";
  return `${httpBase()}/netboot/netboot.xyz.kpxe`;
});
const dialogTitle = computed(() => {
  if (dialog.value === "mkdir") return "新建目录";
  if (dialog.value === "file") return "新建文件";
  return "重命名或移动";
});
const dialogHint = computed(() => {
  if (dialog.value === "mkdir") return "在当前目录下创建一个子目录。";
  if (dialog.value === "file") return "创建后打开文本编辑器。";
  return "填写新名称或相对路径。";
});
const dialogPlaceholder = computed(() => {
  if (dialog.value === "mkdir") return "目录名";
  if (dialog.value === "file") return "例如 boot.ipxe";
  return "新名称或目标路径";
});

async function load() {
  await run(async () => {
    await fetchFiles(selectedFullPath.value);
  });
}

async function fetchFiles(selectPath = "") {
  const res = await api<FileListResponse>(
    `/files?root=${root.value}&path=${encodeURIComponent(currentPath.value)}`,
  );
  maxUploadBytes.value = res.max_upload_bytes;
  files.value = Array.isArray(res.files) ? res.files : [];
  basePath.value = res.base_path || "";
  selected.value = selectPath
    ? (files.value.find((file) => fullPath(file.name) === selectPath) ?? null)
    : null;
  persistLocation();
}

async function loadConfig() {
  try {
    config.value = await api<ServiceConfig>("/config");
  } catch {
    config.value = null;
  }
}

async function run(task: () => Promise<void>) {
  if (busy.value) return;
  busy.value = true;
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

async function switchRoot(value: RootKey) {
  if (busy.value || root.value === value) return;
  if (!(await allowLeave())) return;
  root.value = value;
  currentPath.value = ".";
  persistLocation();
  closeEditor();
  load();
}

function fullPath(name: string) {
  return currentPath.value === "." ? name : `${currentPath.value}/${name}`;
}

async function goPath(path: string) {
  if (busy.value) return;
  if (!(await allowLeave())) return;
  currentPath.value = normalizePath(path);
  persistLocation();
  closeEditor();
  load();
}

function goUp() {
  const parts = currentPath.value.split("/").filter(Boolean);
  parts.pop();
  goPath(parts.join("/") || ".");
}

function selectFile(file: FileEntry) {
  if (file.dir) {
    goPath(fullPath(file.name));
    return;
  }
  selected.value = file;
}

function openCreateDir() {
  dialog.value = "mkdir";
  dialogValue.value = "";
}

function openCreateFile() {
  dialog.value = "file";
  dialogValue.value = "";
}

function startRename() {
  if (!selected.value) return;
  dialog.value = "rename";
  dialogValue.value = selectedFullPath.value;
}

async function confirmDialog() {
  const value = dialogValue.value.trim();
  if (!value) return;
  if (dialog.value === "mkdir") {
    await mkdir(value);
  } else if (dialog.value === "file") {
    await createFile(value);
  } else if (dialog.value === "rename") {
    await rename(value);
  }
}

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement;
  if (!input.files?.[0]) return;
  const file = input.files[0];
  const target = new URLSearchParams({
    root: root.value,
    path: fullPath(file.name),
  });
  await run(async () => {
    if (file.size > maxUploadBytes.value)
      throw new Error(`文件超过上传上限 ${formatSize(maxUploadBytes.value)}`);
    const controller = new AbortController();
    uploadController.value = controller;
    uploadProgress.value = 0;
    try {
      await upload(
        `/files/upload?${target}`,
        file,
        controller.signal,
        (percent) => {
          uploadProgress.value = percent;
        },
      );
      await refreshCurrentDirectory("", "文件已上传");
    } finally {
      uploadController.value = null;
      input.value = "";
    }
  });
}

async function mkdir(name: string) {
  await run(async () => {
    await api("/files/mkdir", {
      method: "POST",
      body: JSON.stringify({ root: root.value, path: fullPath(name) }),
    });
    dialog.value = "";
    await refreshCurrentDirectory("", "目录已创建");
  });
}

async function createFile(name: string) {
  const path = fullPath(name);
  await run(async () => {
    await api("/files/content", {
      method: "PUT",
      body: JSON.stringify({ root: root.value, path, content: "" }),
    });
    dialog.value = "";
    await refreshCurrentDirectory(path, "文件已创建");
    const created = files.value.find(
      (item) => !item.dir && fullPath(item.name) === path,
    );
    if (created?.editable) {
      selected.value = created;
      await loadEditorContent(path);
    }
  });
}

async function rename(to: string) {
  if (!selected.value) return;
  await run(async () => {
    await api("/files/rename", {
      method: "POST",
      body: JSON.stringify({
        root: root.value,
        from: selectedFullPath.value,
        to,
      }),
    });
    dialog.value = "";
    await refreshCurrentDirectory(to, "已重命名");
  });
}

async function remove(path: string) {
  if (!(await confirmAction(`确认删除 ${path}？此操作不可恢复。`))) return;
  await run(async () => {
    await api(`/files?root=${root.value}&path=${encodeURIComponent(path)}`, {
      method: "DELETE",
    });
    closeEditor();
    await refreshCurrentDirectory("", "已删除");
  });
}

async function editFile(file: FileEntry) {
  if (file.dir || !file.editable) return;
  selected.value = file;
  const path = fullPath(file.name);
  await run(async () => {
    await loadEditorContent(path);
  });
}

async function loadEditorContent(path: string) {
  const res = await api<{ content: string }>(
    `/files/content?root=${root.value}&path=${encodeURIComponent(path)}`,
  );
  editing.value = true;
  editorOpen.value = true;
  editingPath.value = path;
  editorContent.value = res.content;
  originalContent.value = res.content;

  await focusEditor();
}

async function saveContent() {
  if (!editing.value || !dirty.value) return;
  await run(async () => {
    await api("/files/content", {
      method: "PUT",
      body: JSON.stringify({
        root: root.value,
        path: editingPath.value,
        content: editorContent.value,
      }),
    });
    originalContent.value = editorContent.value;
    await refreshCurrentDirectory(editingPath.value, "文件已保存");
  });
}

function closeEditor() {
  editing.value = false;
  editorOpen.value = false;
  editingPath.value = "";
  editorContent.value = "";
  originalContent.value = "";
}

async function closeEditorWindow() {
  if (dirty.value && !(await confirmAction("修改未保存，是否放弃？"))) return;
  closeEditor();
}

async function focusEditor() {
  await nextTick();
  editorRef.value?.focus();
}

async function refreshCurrentDirectory(selectPath = "", nextMessage = "") {
  await fetchFiles(selectPath);
  message.value = nextMessage;
}

async function copyText(text: string) {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    error.value = false;
    message.value = "已复制到剪贴板";
  } catch {
    error.value = true;
    message.value = "复制失败，请手动复制路径";
  }
}

function accessPath(path: string) {
  const clean = path.replace(/^\.?\//, "");
  if (root.value === "http") return `${httpBase()}/${clean}`;
  if (root.value === "netboot") return `${httpBase()}/netboot/${clean}`;
  return clean;
}

function httpBase() {
  const ip = config.value?.server.advertise_ip || "通告IP";
  const addr = config.value?.httpboot.addr || ":80";
  const port = httpPort(addr);
  return port && port !== "80" ? `http://${ip}:${port}` : `http://${ip}`;
}

function httpPort(addr: string) {
  if (!addr) return "80";
  if (addr.startsWith(":")) return addr.slice(1) || "80";
  const match = addr.match(/:(\d+)$/);
  return match?.[1] || "80";
}

function filePurpose(name: string) {
  const ext = extName(name);
  if (ext === "efi") return "UEFI 固件";
  if (["kpxe", "pxe", "bios"].includes(ext)) return "PXE 固件";
  if (ext === "ipxe") return "iPXE 脚本";
  if (ext === "wim") return "WIM 镜像";
  if (ext === "iso") return "ISO 镜像";
  if (["vhd", "vhdx", "img"].includes(ext)) return "磁盘镜像";
  if (["gz", "xz", "zip"].includes(ext)) return "压缩文件";
  return "文件";
}

function extName(name: string) {
  const index = name.lastIndexOf(".");
  return index >= 0 ? name.slice(index + 1).toLowerCase() : "";
}

function loadSavedLocation() {
  try {
    const raw = window.localStorage.getItem(locationStorageKey);
    const saved = raw
      ? (JSON.parse(raw) as { root?: string; path?: string })
      : {};
    const savedRoot = roots.some((item) => item.key === saved.root)
      ? (saved.root as RootKey)
      : "http";
    return { root: savedRoot, path: normalizePath(saved.path || ".") };
  } catch {
    return { root: "http" as RootKey, path: "." };
  }
}

function persistLocation() {
  try {
    window.localStorage.setItem(
      locationStorageKey,
      JSON.stringify({ root: root.value, path: currentPath.value }),
    );
  } catch {
    // localStorage may be unavailable in hardened browser profiles; the file manager still works in memory.
  }
}

function normalizePath(path: string) {
  const clean = path.replaceAll("\\", "/").split("/").filter(Boolean).join("/");
  return clean || ".";
}

function formatSize(size: number) {
  if (!Number.isFinite(size)) return "-";
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  if (size < 1024 * 1024 * 1024)
    return `${(size / 1024 / 1024).toFixed(1)} MiB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GiB`;
}

function formatTime(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("zh-CN", { hour12: false });
}

async function allowLeave() {
  return !dirty.value || (await confirmAction("修改未保存，是否放弃？"));
}
onBeforeRouteLeave(async () => {
  if (!(await allowLeave())) return false;
  if (
    uploadController.value &&
    !(await confirmAction("文件正在上传，离开将取消上传。是否继续？"))
  )
    return false;
  return true;
});
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value || uploadController.value) {
    event.preventDefault();
    event.returnValue = "";
  }
}
onBeforeUnmount(() => window.removeEventListener("beforeunload", beforeUnload));
usePageRefresh(load);
onMounted(async () => {
  window.addEventListener("beforeunload", beforeUnload);
  await loadConfig();
  await load();
});
</script>
