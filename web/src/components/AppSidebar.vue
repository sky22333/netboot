<script setup lang="ts">
import { useRoute } from "vue-router";
import {
  Activity,
  Files,
  Gauge,
  Code2,
  HardDrive,
  Network,
  ScrollText,
  Settings,
  Users,
} from "@lucide/vue";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
const route = useRoute();
const { setOpenMobile } = useSidebar();
const nav = [
  { path: "/", name: "运行概览", icon: Gauge },
  { path: "/config", name: "服务配置", icon: Settings },
  { path: "/clients", name: "设备管理", icon: Network },
  { path: "/files", name: "文件管理", icon: Files },
  { path: "/netboot", name: "固件下载", icon: HardDrive },
  { path: "/users", name: "账号管理", icon: Users },
  { path: "/logs", name: "运行日志", icon: ScrollText },
  { path: "/diagnostics", name: "系统诊断", icon: Activity },
];
</script>
<template>
  <Sidebar>
    <SidebarHeader class="h-16 justify-center border-b px-6"
      ><span class="text-lg font-semibold tracking-tight"
        >PXE
        <span class="text-sm font-normal text-muted-foreground"
          >控制台</span
        ></span
      ></SidebarHeader
    >
    <SidebarContent class="p-3">
      <nav aria-label="主导航">
        <SidebarMenu>
          <SidebarMenuItem v-for="item in nav" :key="item.path">
            <SidebarMenuButton
              as-child
              :is-active="route.path === item.path"
              class="h-10 px-3"
            >
              <RouterLink :to="item.path" @click="setOpenMobile(false)"
                ><component :is="item.icon" /><span>{{
                  item.name
                }}</span></RouterLink
              >
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </nav>
    </SidebarContent>
    <SidebarFooter class="border-t p-3"
      ><SidebarMenu
        ><SidebarMenuItem
          ><SidebarMenuButton as-child
            ><a
              href="https://github.com/sky22333/netboot"
              target="_blank"
              rel="noreferrer"
              ><Code2 /><span>GitHub</span></a
            ></SidebarMenuButton
          ></SidebarMenuItem
        ></SidebarMenu
      ></SidebarFooter
    >
  </Sidebar>
</template>
