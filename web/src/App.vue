<template>
  <Toaster position="top-right" :duration="3000" rich-colors />
  <ConfirmDialog v-if="confirmation" />
  <div
    v-if="authMode === 'loading'"
    class="grid min-h-svh place-items-center p-6"
  >
    <div class="w-64 space-y-3" role="status" aria-label="正在连接">
      <Skeleton class="h-6 w-32" /><Skeleton class="h-10 w-full" />
    </div>
  </div>
  <main
    v-else-if="authMode !== 'ready'"
    class="grid min-h-svh place-items-center bg-muted/40 p-4"
  >
    <Card class="w-full max-w-sm gap-6 p-6">
      <div>
        <h1 class="text-xl font-semibold">PXE 控制台</h1>
        <p class="mt-2 text-sm text-muted-foreground">
          {{
            authMode === "setup"
              ? "创建管理员账号，开始使用。"
              : "登录管理账号。"
          }}
        </p>
      </div>
      <form class="space-y-4" @submit.prevent="submitAuth">
        <div class="space-y-2">
          <Label for="username">用户名</Label
          ><Input
            id="username"
            v-model.trim="username"
            autocomplete="username"
            required
            :disabled="authBusy"
          />
        </div>
        <div class="space-y-2">
          <Label for="password">密码</Label
          ><Input
            id="password"
            v-model="password"
            type="password"
            :autocomplete="
              authMode === 'setup' ? 'new-password' : 'current-password'
            "
            required
            :disabled="authBusy"
          />
        </div>
        <p v-if="authMode === 'setup'" class="text-xs text-muted-foreground">
          用户名 3–32 位，支持字母、数字和 . _ @ -；密码至少 8 位。
        </p>
        <Alert v-if="authError" variant="destructive"
          ><AlertDescription>{{ authError }}</AlertDescription></Alert
        >
        <Button type="submit" class="w-full" :disabled="authBusy"
          ><LoaderCircle v-if="authBusy" class="animate-spin" />{{
            authBusy ? "请稍候" : authMode === "setup" ? "创建账号" : "登录"
          }}</Button
        >
      </form>
    </Card>
  </main>
  <SidebarProvider v-else>
    <AppSidebar />
    <SidebarInset class="min-w-0 bg-muted/30">
      <header
        class="sticky top-0 z-20 flex h-16 shrink-0 items-center justify-between gap-3 border-b bg-background px-4 lg:px-6"
      >
        <div class="flex items-center gap-3">
          <SidebarTrigger /><span class="text-sm font-medium"
            >网络启动控制台</span
          >
        </div>
        <div class="flex items-center gap-2">
          <Button
            v-if="pageRefresh"
            variant="ghost"
            :disabled="refreshing"
            @click="refresh"
            ><RefreshCw :class="{ 'animate-spin': refreshing }" />{{
              refreshing ? "刷新中" : "刷新"
            }}</Button
          >
          <Button variant="ghost" :disabled="loggingOut" @click="logout"
            ><LogOut />退出登录</Button
          >
        </div>
      </header>
      <div class="min-w-0 flex-1 p-4 lg:p-6"><RouterView /></div>
    </SidebarInset>
  </SidebarProvider>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { LoaderCircle, LogOut, RefreshCw } from "@lucide/vue";
import { toast } from "vue-sonner";
import AppSidebar from "@/components/AppSidebar.vue";
import ConfirmDialog from "@/components/ConfirmDialog.vue";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { Toaster } from "@/components/ui/sonner";
import {
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { api } from "@/lib/api";
import { useEventLog } from "@/lib/eventLog";
import { pageRefresh } from "@/lib/pageRefresh";
import { confirmation, resolveConfirmation } from "@/lib/confirm";

const authMode = ref<"loading" | "setup" | "login" | "ready">("loading");
const username = ref("admin");
const password = ref("");
const authError = ref("");
const authBusy = ref(false);
const refreshing = ref(false);
const loggingOut = ref(false);

async function refresh() {
  if (refreshing.value || !pageRefresh.value) return;
  refreshing.value = true;
  try {
    await pageRefresh.value();
  } catch (e) {
    toast.error(e instanceof Error ? e.message : "刷新失败");
  } finally {
    refreshing.value = false;
  }
}
async function checkAuth() {
  try {
    const setup = await api<{ has_user: boolean }>("/setup/status");
    if (!setup.has_user) {
      authMode.value = "setup";
      return;
    }
    await api("/status");
    authMode.value = "ready";
  } catch (e) {
    if (authMode.value !== "login")
      authError.value = e instanceof Error ? e.message : "连接失败，请重试";
    authMode.value = "login";
  }
}
async function submitAuth() {
  if (authBusy.value) return;
  authBusy.value = true;
  authError.value = "";
  try {
    if (authMode.value === "setup")
      await api("/setup", {
        method: "POST",
        body: JSON.stringify({
          username: username.value,
          password: password.value,
        }),
      });
    await api("/auth/login", {
      method: "POST",
      body: JSON.stringify({
        username: username.value,
        password: password.value,
      }),
    });
    password.value = "";
    authMode.value = "ready";
  } catch (e) {
    authError.value = e instanceof Error ? e.message : "登录失败";
  } finally {
    authBusy.value = false;
  }
}
function expireAuth() {
  useEventLog().disconnect();
  resolveConfirmation(false);
  password.value = "";
  authMode.value = "login";
}
async function logout() {
  loggingOut.value = true;
  try {
    await api("/auth/logout", { method: "POST" });
    expireAuth();
  } catch (e) {
    toast.error(e instanceof Error ? e.message : "退出失败");
  } finally {
    loggingOut.value = false;
  }
}
onMounted(() => {
  window.addEventListener("pxe-auth-expired", expireAuth);
  void checkAuth();
});
onUnmounted(() => window.removeEventListener("pxe-auth-expired", expireAuth));
</script>
