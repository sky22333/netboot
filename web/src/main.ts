import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import { toast } from "vue-sonner";
import "./styles/main.css";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: () => import("./pages/Dashboard.vue") },
    { path: "/config", component: () => import("./pages/ConfigPage.vue") },
    { path: "/clients", component: () => import("./pages/ClientsPage.vue") },
    { path: "/files", component: () => import("./pages/FilesPage.vue") },
    { path: "/netboot", component: () => import("./pages/FirmwarePage.vue") },
    { path: "/users", component: () => import("./pages/UsersPage.vue") },
    { path: "/logs", component: () => import("./pages/LogsPage.vue") },
    {
      path: "/diagnostics",
      component: () => import("./pages/DiagnosticsPage.vue"),
    },
  ],
});

const app = createApp(App);
app.config.errorHandler = (err) => {
  console.error("pxe ui error", err);
  toast.error(err instanceof Error ? err.message : "操作失败，请重试");
};
app.use(router).mount("#app");
