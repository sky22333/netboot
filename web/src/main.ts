import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import Dashboard from './pages/Dashboard.vue'
import ConfigPage from './pages/ConfigPage.vue'
import ClientsPage from './pages/ClientsPage.vue'
import FilesPage from './pages/FilesPage.vue'
import NetbootPage from './pages/NetbootPage.vue'
import LogsPage from './pages/LogsPage.vue'
import DiagnosticsPage from './pages/DiagnosticsPage.vue'
import UsersPage from './pages/UsersPage.vue'
import './styles/main.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Dashboard },
    { path: '/config', component: ConfigPage },
    { path: '/clients', component: ClientsPage },
    { path: '/files', component: FilesPage },
    { path: '/netboot', component: NetbootPage },
    { path: '/users', component: UsersPage },
    { path: '/logs', component: LogsPage },
    { path: '/diagnostics', component: DiagnosticsPage }
  ]
})

const app = createApp(App)
app.config.errorHandler = (err) => {
  console.error('pxe ui error', err)
}
app.use(router).mount('#app')
