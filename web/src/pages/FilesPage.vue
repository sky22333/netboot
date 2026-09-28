<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-lg font-semibold">文件管理</h1>
        <p class="mt-1 text-sm text-muted-foreground">
          管理启动脚本、固件和镜像。
        </p>
      </div>
      <div class="flex gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger as-child
            ><Button variant="outline" :disabled="writeBusy"
              ><Plus class="size-4" />新建<ChevronDown class="size-4" /></Button
          ></DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem @select="openCreateDir"
              ><FolderPlus />新建目录</DropdownMenuItem
            >
            <DropdownMenuItem @select="openCreateFile"
              ><FilePlus2 />新建文件</DropdownMenuItem
            >
          </DropdownMenuContent>
        </DropdownMenu>
        <Button :disabled="writeBusy" @click="uploadInput?.click()"
          ><Upload />上传文件</Button
        >
        <input
          ref="uploadInput"
          class="hidden"
          type="file"
          aria-label="上传文件"
          :disabled="writeBusy"
          @change="onFile"
        />
      </div>
    </div>
    <Tabs
      :model-value="root"
      @update:model-value="(value) => switchRoot(value as RootKey)"
    >
      <TabsList
        ><TabsTrigger
          v-for="item in roots"
          :key="item.key"
          :value="item.key"
          :disabled="busy"
          ><component :is="item.icon" />{{ item.label }}</TabsTrigger
        ></TabsList
      >
    </Tabs>
    <p class="text-sm text-muted-foreground">{{ activeRoot.description }}</p>
    <Feedback
      v-if="!dialog && !editorOpen && !detailsOpen"
      :message="message"
      :error="error"
    />
    <Card v-if="uploadTask" class="gap-3 p-4" aria-live="polite">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="break-all text-sm font-medium">{{ uploadTask.name }}</p>
          <p class="mt-1 break-all text-xs text-muted-foreground">
            上传到 {{ uploadTask.destination }}
          </p>
        </div>
        <Button
          v-if="uploadController"
          variant="ghost"
          size="sm"
          @click="uploadController.abort()"
          >取消上传</Button
        >
        <Button v-else variant="ghost" size="sm" @click="uploadTask = null"
          >关闭</Button
        >
      </div>
      <Progress
        v-if="uploadController"
        :model-value="uploadProgress"
        :max="100"
      />
      <p
        class="text-xs"
        :class="
          uploadTask.failed ? 'text-destructive' : 'text-muted-foreground'
        "
      >
        {{
          uploadTask.status ||
          `${formatSize((uploadTask.size * uploadProgress) / 100)} / ${formatSize(uploadTask.size)} · ${uploadProgress === 100 ? "正在保存" : uploadProgress + "%"}`
        }}
      </p>
    </Card>
    <Card>
      <div
        class="flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between"
      >
        <div class="flex min-w-0 items-center gap-2">
          <Button
            variant="ghost"
            size="icon-sm"
            :disabled="busy || currentPath === '.'"
            aria-label="返回上一级"
            @click="goUp"
            ><ArrowUp
          /></Button>
          <Breadcrumb class="min-w-0"
            ><BreadcrumbList class="gap-1 break-all">
              <BreadcrumbItem
                ><BreadcrumbLink as-child
                  ><Button
                    variant="ghost"
                    size="sm"
                    :disabled="busy"
                    @click="goPath('.')"
                    >{{ activeRoot.label }}</Button
                  ></BreadcrumbLink
                ></BreadcrumbItem
              >
              <template v-for="(crumb, index) in crumbs" :key="crumb.path"
                ><BreadcrumbSeparator /><BreadcrumbItem>
                  <BreadcrumbPage v-if="index === crumbs.length - 1">{{
                    crumb.name
                  }}</BreadcrumbPage>
                  <BreadcrumbLink v-else as-child
                    ><Button
                      variant="ghost"
                      size="sm"
                      :disabled="busy"
                      @click="goPath(crumb.path)"
                      >{{ crumb.name }}</Button
                    ></BreadcrumbLink
                  >
                </BreadcrumbItem></template
              >
            </BreadcrumbList></Breadcrumb
          >
        </div>
        <div class="flex gap-2 sm:shrink-0">
          <Input
            v-model="search"
            aria-label="搜索当前目录"
            placeholder="搜索当前目录"
            class="min-w-0 sm:w-44"
          />
          <NativeSelect
            v-model="sortBy"
            aria-label="排序方式"
            class="w-28 shrink-0"
            ><NativeSelectOption value="name">名称排序</NativeSelectOption
            ><NativeSelectOption value="size">大小优先</NativeSelectOption
            ><NativeSelectOption value="time"
              >最近修改</NativeSelectOption
            ></NativeSelect
          >
        </div>
      </div>
      <Skeleton
        v-if="busy && !files.length"
        class="m-4 h-32"
        aria-label="正在加载"
      />
      <Table v-else class="table-fixed">
        <TableHeader
          ><TableRow
            ><TableHead class="px-4">名称</TableHead
            ><TableHead class="hidden w-24 sm:table-cell">大小</TableHead
            ><TableHead class="hidden w-44 lg:table-cell">修改时间</TableHead
            ><TableHead class="w-20 text-right"
              ><span class="sr-only">操作</span></TableHead
            ></TableRow
          ></TableHeader
        >
        <TableBody>
          <TableRow
            v-for="file in sortedFiles"
            :key="file.name"
            :class="selected?.name === file.name ? 'bg-muted/50' : ''"
          >
            <TableCell class="px-4 py-3">
              <Button
                variant="ghost"
                class="h-auto max-w-full justify-start gap-2 px-0 hover:bg-transparent"
                :disabled="busy"
                @click="selectFile(file)"
              >
                <Folder
                  v-if="file.dir"
                  class="shrink-0 text-amber-600"
                /><FileText
                  v-else
                  class="shrink-0 text-muted-foreground"
                /><span class="truncate">{{ file.name }}</span>
              </Button>
              <p class="mt-1 text-xs text-muted-foreground">
                {{ file.dir ? "目录" : filePurpose(file.name)
                }}<span v-if="!file.dir" class="sm:hidden">
                  · {{ formatSize(file.size) }}</span
                >
              </p>
            </TableCell>
            <TableCell class="hidden text-muted-foreground sm:table-cell">{{
              file.dir ? "—" : formatSize(file.size)
            }}</TableCell>
            <TableCell
              class="hidden text-xs text-muted-foreground lg:table-cell"
              >{{ formatTime(file.mod_time) }}</TableCell
            >
            <TableCell class="pr-3 text-right">
              <DropdownMenu
                ><DropdownMenuTrigger as-child
                  ><Button
                    variant="ghost"
                    size="icon-sm"
                    :disabled="busy"
                    :aria-label="`${file.name} 的操作`"
                    ><Ellipsis /></Button
                ></DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    v-if="file.editable"
                    :disabled="writeBusy"
                    @select="editFile(file)"
                    ><Pencil />编辑</DropdownMenuItem
                  >
                  <DropdownMenuItem
                    v-if="!file.dir"
                    :disabled="root === 'http' && !config"
                    @select="copyText(accessPath(fullPath(file.name)))"
                    ><Copy />{{
                      root === "http" ? "复制链接" : "复制启动路径"
                    }}</DropdownMenuItem
                  >
                  <DropdownMenuItem @select="showDetails(file)"
                    ><Info />查看详情</DropdownMenuItem
                  >
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    :disabled="writeBusy"
                    @select="startRename(file)"
                    ><MoveRight />重命名</DropdownMenuItem
                  >
                  <DropdownMenuItem
                    variant="destructive"
                    :disabled="writeBusy"
                    @select="remove(fullPath(file.name))"
                    ><Trash2 />删除</DropdownMenuItem
                  >
                </DropdownMenuContent>
              </DropdownMenu>
            </TableCell>
          </TableRow>
          <TableRow v-if="!sortedFiles.length"
            ><TableCell
              colspan="4"
              class="h-36 text-center text-muted-foreground"
              >{{
                search ? "没有匹配的文件" : "目录为空，上传文件或点击“新建”。"
              }}</TableCell
            ></TableRow
          >
        </TableBody>
      </Table>
      <div
        class="flex flex-wrap justify-between gap-2 border-t px-4 py-3 text-xs text-muted-foreground"
      >
        <span>{{ sortedFiles.length }} 项</span
        ><span v-if="maxUploadBytes"
          >单文件最多 {{ formatSize(maxUploadBytes) }}</span
        >
      </div>
    </Card>
    <Sheet v-model:open="detailsOpen">
      <SheetContent
        class="overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-md"
      >
        <SheetHeader
          ><SheetTitle>文件详情</SheetTitle
          ><SheetDescription class="break-all">{{
            selected?.name
          }}</SheetDescription></SheetHeader
        >
        <div v-if="selected" class="space-y-5 px-4 pb-4 text-sm">
          <dl class="space-y-4">
            <div>
              <dt class="text-xs text-muted-foreground">相对路径</dt>
              <dd class="mt-1 break-all">{{ selectedFullPath }}</dd>
            </div>
            <div>
              <dt class="text-xs text-muted-foreground">所在目录</dt>
              <dd class="mt-1 break-all">{{ basePath }}</dd>
            </div>
            <div>
              <dt class="text-xs text-muted-foreground">类型</dt>
              <dd class="mt-1">
                {{ selected.dir ? "目录" : filePurpose(selected.name) }}
              </dd>
            </div>
            <div v-if="!selected.dir">
              <dt class="text-xs text-muted-foreground">大小</dt>
              <dd class="mt-1">{{ formatSize(selected.size) }}</dd>
            </div>
            <div>
              <dt class="text-xs text-muted-foreground">修改时间</dt>
              <dd class="mt-1">{{ formatTime(selected.mod_time) }}</dd>
            </div>
            <div v-if="!selected.dir">
              <dt class="text-xs text-muted-foreground">
                {{ root === "http" ? "访问链接" : "启动路径" }}
              </dt>
              <dd class="mt-1 break-all font-mono text-xs">
                {{ selectedAccessPath || "读取配置后可复制链接" }}
              </dd>
            </div>
          </dl>
          <Button
            v-if="!selected.dir"
            variant="outline"
            :disabled="!selectedAccessPath"
            @click="copyText(selectedAccessPath)"
            ><Copy />{{ root === "http" ? "复制链接" : "复制启动路径" }}</Button
          >
          <Feedback :message="message" :error="error" />
        </div>
      </SheetContent>
    </Sheet>
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
        <div v-if="message" class="px-4 pt-3">
          <Feedback :message="message" :error="error" />
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
        <Feedback :message="message" :error="error" />
        <form class="space-y-4" @submit.prevent="confirmDialog">
          <Label for="file-name">{{
            dialog === "rename" ? "新名称" : "名称"
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

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
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
  ChevronDown,
  ArrowUp,
  Ellipsis,
  Plus,
  Copy,
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

type RootKey = "http" | "tftp";

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
    label: "HTTP目录",
    icon: Globe2,
    description: "存放启动脚本和系统镜像，通过 HTTP 访问。",
  },
  {
    key: "tftp" as RootKey,
    label: "TFTP目录",
    icon: HardDrive,
    description: "存放客户端网络启动所需的固件。",
  },
];

const locationStorageKey = "pxe.files.location";
const initialLocation = loadSavedLocation();

const root = ref<RootKey>(initialLocation.root);
const currentPath = ref(initialLocation.path);
const basePath = ref("");
const files = ref<FileEntry[]>([]);
const selected = ref<FileEntry | null>(null);
const detailsOpen = ref(false);
const search = ref("");
const sortBy = ref("name");
const uploadTask = ref<{
  name: string;
  destination: string;
  size: number;
  status: string;
  failed: boolean;
} | null>(null);
let disposed = false;
let pendingUploadRefresh: {
  root: RootKey;
  directory: string;
  path: string;
} | null = null;
const message = ref("");
const error = ref(false);
const busy = ref(false);
const uploadInput = ref<HTMLInputElement | null>(null);
const uploadProgress = ref(0);
const uploadController = ref<AbortController | null>(null);
const maxUploadBytes = ref(0);
const writeBusy = computed(() => busy.value || !!uploadController.value);
onBeforeUnmount(() => {
  disposed = true;
  uploadController.value?.abort();
});
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
  files.value
    .filter((file) =>
      file.name.toLocaleLowerCase().includes(search.value.toLocaleLowerCase()),
    )
    .sort((a, b) => {
      const directories = Number(b.dir) - Number(a.dir);
      if (directories) return directories;
      const order =
        sortBy.value === "size"
          ? b.size - a.size
          : sortBy.value === "time"
            ? Date.parse(b.mod_time) - Date.parse(a.mod_time)
            : 0;
      return order || a.name.localeCompare(b.name, "zh-Hans-CN");
    }),
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
const dialogTitle = computed(() => {
  if (dialog.value === "mkdir") return "新建目录";
  if (dialog.value === "file") return "新建文件";
  return "重命名";
});
const dialogHint = computed(() => {
  if (dialog.value === "mkdir") return "在当前目录下创建一个子目录。";
  if (dialog.value === "file") return "创建后打开文本编辑器。";
  return "修改当前文件或目录的名称。";
});
const dialogPlaceholder = computed(() => {
  if (dialog.value === "mkdir") return "目录名";
  if (dialog.value === "file") return "例如 install.ipxe";
  return "新名称";
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
    if (pendingUploadRefresh) {
      const pending = pendingUploadRefresh;
      pendingUploadRefresh = null;
      if (
        !disposed &&
        root.value === pending.root &&
        currentPath.value === pending.directory
      ) {
        await loadAfterUpload(pending.path);
      }
    }
  }
}

async function switchRoot(value: RootKey) {
  if (busy.value || root.value === value) return;
  if (!(await allowLeave())) return;
  search.value = "";
  files.value = [];
  selected.value = null;
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
  search.value = "";
  files.value = [];
  selected.value = null;
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
  if (file.editable && !writeBusy.value) {
    void editFile(file);
    return;
  }
  showDetails(file);
}

function showDetails(file: FileEntry) {
  selected.value = file;
  message.value = "";
  detailsOpen.value = true;
}

function openCreateDir() {
  message.value = "";
  error.value = false;
  dialog.value = "mkdir";
  dialogValue.value = "";
}

function openCreateFile() {
  message.value = "";
  error.value = false;
  dialog.value = "file";
  dialogValue.value = "";
}

function startRename(file: FileEntry) {
  selected.value = file;
  message.value = "";
  error.value = false;
  dialog.value = "rename";
  dialogValue.value = file.name;
}

async function confirmDialog() {
  const value = dialogValue.value.trim();
  if (!value) return;
  if ([".", ".."].includes(value) || /[/\\]/.test(value)) {
    error.value = true;
    message.value = "请填写名称，不要包含路径。";
    return;
  }
  if (
    files.value.some(
      (file) => file.name.toLocaleLowerCase() === value.toLocaleLowerCase(),
    )
  ) {
    error.value = true;
    message.value = "名称已存在，请换一个名称。";
    return;
  }
  if (dialog.value === "mkdir") {
    await mkdir(value);
  } else if (dialog.value === "file") {
    await createFile(value);
  } else if (dialog.value === "rename") {
    await rename(fullPath(value));
  }
}

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file || writeBusy.value) return;
  const targetRoot = root.value,
    targetDirectory = currentPath.value;
  const targetPath = fullPath(file.name);
  const task = {
    name: file.name,
    destination: `${activeRoot.value.label}/${targetPath}`,
    size: file.size,
    status: "",
    failed: false,
  };
  uploadTask.value = task;
  uploadProgress.value = 0;
  const controller = new AbortController();
  uploadController.value = controller;
  try {
    if (file.size > maxUploadBytes.value)
      throw new Error(`文件超过 ${formatSize(maxUploadBytes.value)} 上限`);
    await upload(
      `/files/upload?${new URLSearchParams({ root: targetRoot, path: targetPath })}`,
      file,
      controller.signal,
      (percent) => (uploadProgress.value = percent),
    );
    task.status = "上传完成";
    if (
      !disposed &&
      root.value === targetRoot &&
      currentPath.value === targetDirectory
    ) {
      search.value = "";
      if (busy.value)
        pendingUploadRefresh = {
          root: targetRoot,
          directory: targetDirectory,
          path: targetPath,
        };
      else await loadAfterUpload(targetPath);
    }
  } catch (e) {
    task.failed = !controller.signal.aborted;
    task.status = e instanceof Error ? e.message : "上传失败";
  } finally {
    uploadController.value = null;
    if (!disposed) uploadTask.value = { ...task };
    input.value = "";
  }
}
async function loadAfterUpload(path: string) {
  await run(async () => {
    await fetchFiles(path);
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
  const content = editorContent.value;
  await run(async () => {
    await api("/files/content", {
      method: "PUT",
      body: JSON.stringify({
        root: root.value,
        path: editingPath.value,
        content,
      }),
    });
    originalContent.value = content;
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
  if (selectPath) search.value = "";
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
  if (root.value === "http")
    return config.value
      ? `${httpBase()}/${clean.split("/").map(encodeURIComponent).join("/")}`
      : "";
  return clean;
}

function httpBase() {
  const ip = config.value?.server.advertise_ip || window.location.hostname;
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
