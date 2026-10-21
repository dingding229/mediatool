import {pendingRestore} from './workflows/restore-session'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import {elementComponents,ElLoading} from './ui/element-components'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './style.css'
import './ui/consistency.css'
import PagePagination from './ui/PagePagination.vue'
import { initTheme } from './ui/theme'
import { initializePWA } from './pwa/install'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'

initTheme()

const app = createApp(App).use(createPinia()).use(router)
for(const [name,component]of Object.entries({PagePagination,...elementComponents}))app.component(name,component)

window.addEventListener('media-tool:session-expired', () => {
  const auth = useAuthStore()
  auth.authenticated = false
  auth.checked = true
  const restore=pendingRestore();router.replace(restore?{path:'/login',query:{restore}}:'/login')
})

app.directive('loading', ElLoading.directive)
app.mount('#app')
initializePWA()
