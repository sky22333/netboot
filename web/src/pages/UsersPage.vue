<template>
  <div class="space-y-6">
    <Skeleton
      v-if="loading && users.length === 0"
      class="h-16 w-full"
      aria-label="正在加载"
    />
    <div>
      <div>
        <h1 class="text-lg font-semibold">账号管理</h1>
        <p class="mt-1 text-sm text-muted-foreground">
          管理后台登录账号，首个管理员不可删除。
        </p>
      </div>
      <div
        class="mt-4 grid items-end gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_8rem]"
      >
        <div class="min-w-0 space-y-2">
          <Label for="userspage-1">用户名</Label
          ><Input
            id="userspage-1"
            v-model.trim="username"
            autocomplete="off"
            placeholder="用户名"
          />
        </div>
        <div class="min-w-0 space-y-2">
          <Label for="userspage-2">密码</Label
          ><Input
            id="userspage-2"
            v-model="password"
            type="password"
            autocomplete="new-password"
            placeholder="密码至少 8 位"
          />
        </div>

        <Button
          variant="default"
          :disabled="creating || !canCreate"
          @click="create"
        >
          {{ creating ? "创建中..." : "添加账号" }}
        </Button>
      </div>
      <p class="mt-2 text-xs text-muted-foreground">
        用户名需为 3-32 位，仅支持字母、数字、点、下划线、短横线和 @。
      </p>
      <Feedback :message="message" :error="error" />
    </div>

    <Card class="p-6">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="font-semibold">账号列表</h2>
          <p class="mt-1 text-xs text-muted-foreground">
            删除账号后，其登录会话立即失效。
          </p>
        </div>
      </div>
      <div class="mt-4 overflow-hidden">
        <div
          v-for="u in users"
          :key="u.id"
          class="grid gap-3 border-b border-border/60 p-3 text-sm last:border-b-0 sm:grid-cols-[minmax(0,1fr)_8rem_13rem] sm:items-center"
        >
          <div class="min-w-0">
            <div class="truncate font-medium" :title="u.username">
              {{ u.username }}
            </div>
            <div class="mt-1 text-xs text-muted-foreground">
              创建时间：{{ shortDate(u.created_at) }}
            </div>
          </div>
          <div class="text-muted-foreground">
            {{ u.role === "admin" ? "管理员" : u.role }}
          </div>
          <div class="flex gap-2 justify-start sm:justify-end">
            <Button variant="outline" @click="openPassword(u)">{{
              u.current ? "修改密码" : "重置密码"
            }}</Button>
            <Button
              variant="destructive"
              :disabled="isDefaultAdmin(u) || deletingId === u.id"
              @click="remove(u)"
            >
              {{
                isDefaultAdmin(u)
                  ? "默认管理员"
                  : deletingId === u.id
                    ? "删除中..."
                    : "删除"
              }}
            </Button>
          </div>
        </div>
        <div
          v-if="users.length === 0 && !loading"
          class="p-4 text-sm text-muted-foreground"
        >
          暂无账号
        </div>
      </div>
    </Card>
    <Dialog
      :open="!!passwordUser"
      @update:open="
        (open) => {
          if (!open && !changingPassword) closePassword();
        }
      "
    >
      <DialogContent
        :show-close-button="!changingPassword"
        @interact-outside="
          (event) => {
            if (changingPassword) event.preventDefault();
          }
        "
        @escape-key-down="
          (event) => {
            if (changingPassword) event.preventDefault();
          }
        "
      >
        <DialogHeader
          ><DialogTitle>{{
            passwordUser?.current ? "修改密码" : "重置密码"
          }}</DialogTitle
          ><DialogDescription
            >{{ passwordUser?.username }} ·
            修改后该账号需重新登录。</DialogDescription
          ></DialogHeader
        >
        <form class="space-y-4" @submit.prevent="changePassword">
          <input
            class="sr-only"
            aria-label="目标账号"
            autocomplete="username"
            :value="passwordUser?.username"
            readonly
            tabindex="-1"
          />
          <div v-if="passwordUser?.current" class="space-y-2">
            <Label for="current-password">当前密码</Label>
            <Input
              id="current-password"
              v-model="currentPassword"
              :type="showPasswords ? 'text' : 'password'"
              autocomplete="current-password"
              :disabled="changingPassword"
              required
            />
          </div>
          <div class="space-y-2">
            <Label for="new-password">新密码</Label
            ><Input
              id="new-password"
              v-model="nextPassword"
              :type="showPasswords ? 'text' : 'password'"
              autocomplete="new-password"
              :disabled="changingPassword"
              minlength="8"
              required
            />
          </div>
          <div class="space-y-2">
            <Label for="confirm-password">确认新密码</Label>
            <Input
              id="confirm-password"
              v-model="confirmPassword"
              :type="showPasswords ? 'text' : 'password'"
              autocomplete="new-password"
              :disabled="changingPassword"
              required
              :aria-invalid="
                !!confirmPassword && confirmPassword !== nextPassword
              "
              aria-describedby="password-match"
            />
            <p
              v-if="confirmPassword && confirmPassword !== nextPassword"
              id="password-match"
              class="text-sm text-destructive"
            >
              两次输入的密码不一致
            </p>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            :aria-pressed="showPasswords"
            @click="showPasswords = !showPasswords"
            >{{ showPasswords ? "隐藏密码" : "显示密码" }}</Button
          >
          <Feedback :message="passwordError" :error="true" />
          <DialogFooter
            ><Button
              type="submit"
              :disabled="changingPassword || !canChangePassword"
              >{{ changingPassword ? "保存中..." : "保存密码" }}</Button
            ></DialogFooter
          >
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { usePageRefresh } from "@/lib/pageRefresh";
import { Label } from "@/components/ui/label";
import Feedback from "@/components/Feedback.vue";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { confirmAction } from "@/lib/confirm";
import { computed, onMounted, ref } from "vue";
import { api } from "../lib/api";

type User = {
  current: boolean;
  id: number;
  username: string;
  role: string;
  enabled: boolean;
  created_at: string;
};

const users = ref<User[]>([]);
const passwordUser = ref<User | null>(null);
const nextPassword = ref("");
const currentPassword = ref("");
const confirmPassword = ref("");
const showPasswords = ref(false);
const canChangePassword = computed(
  () =>
    !!passwordUser.value &&
    (!passwordUser.value.current || currentPassword.value.length > 0) &&
    nextPassword.value.length >= 8 &&
    new TextEncoder().encode(nextPassword.value).length <= 1024 &&
    nextPassword.value === confirmPassword.value,
);

function closePassword() {
  passwordUser.value = null;
  currentPassword.value = "";
  nextPassword.value = "";
  confirmPassword.value = "";
  passwordError.value = "";
  showPasswords.value = false;
}
function openPassword(user: User) {
  closePassword();
  passwordUser.value = user;
}

const changingPassword = ref(false);
const passwordError = ref("");

async function changePassword() {
  if (!passwordUser.value || changingPassword.value || !canChangePassword.value)
    return;
  changingPassword.value = true;
  passwordError.value = "";
  try {
    const result = await api<{ reauthenticate: boolean }>(
      `/users/${passwordUser.value.id}/password`,
      {
        method: "POST",
        body: JSON.stringify({
          password: nextPassword.value,
          current_password: passwordUser.value.current
            ? currentPassword.value
            : undefined,
        }),
      },
    );
    message.value = passwordUser.value.current ? "密码已修改" : "密码已重置";
    closePassword();
    error.value = false;
    if (result.reauthenticate)
      window.dispatchEvent(new Event("pxe-auth-expired"));
  } catch (e) {
    passwordError.value = e instanceof Error ? e.message : "修改失败";
  } finally {
    changingPassword.value = false;
  }
}

const username = ref("");
const password = ref("");
const loading = ref(false);
const creating = ref(false);
const deletingId = ref<number | null>(null);
const message = ref("");
const error = ref(false);
const usernamePattern = /^[A-Za-z0-9._@-]{3,32}$/;

const firstUserId = computed(() =>
  users.value.length ? Math.min(...users.value.map((u) => u.id)) : 0,
);
const canCreate = computed(
  () =>
    usernamePattern.test(username.value.trim()) && password.value.length >= 8,
);

async function load() {
  loading.value = true;
  try {
    const rows = await api<User[]>("/users");
    users.value = Array.isArray(rows) ? rows : [];
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "账号读取失败";
  } finally {
    loading.value = false;
  }
}

async function create() {
  if (creating.value || !canCreate.value) return;
  creating.value = true;
  error.value = false;
  message.value = "";
  try {
    await api("/users", {
      method: "POST",
      body: JSON.stringify({
        username: username.value.trim(),
        password: password.value,
        role: "admin",
      }),
    });
    username.value = "";
    password.value = "";
    message.value = "账号已创建";
    await load();
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "创建失败";
  } finally {
    creating.value = false;
  }
}

async function remove(user: User) {
  if (deletingId.value !== null || isDefaultAdmin(user)) return;
  if (!(await confirmAction(`确认删除用户 ${user.username}？`))) return;
  deletingId.value = user.id;
  error.value = false;
  message.value = "";
  try {
    await api(`/users/${user.id}`, { method: "DELETE" });
    message.value = "账号已删除";
    await load();
  } catch (e) {
    error.value = true;
    message.value = e instanceof Error ? e.message : "删除失败";
  } finally {
    deletingId.value = null;
  }
}

function isDefaultAdmin(user: User) {
  return user.id === firstUserId.value;
}

function shortDate(value: string) {
  return value ? value.slice(0, 10) : "-";
}

usePageRefresh(async () => {
  error.value = false;
  message.value = "";
  await load();
});
onMounted(load);
</script>
