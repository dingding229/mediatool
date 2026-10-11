import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { ElAlert, ElButton, ElInput, ElSkeleton, ElTable, ElTableColumn,ElDialog,ElSelect,ElOption,ElCheckbox,ElCheckboxGroup,ElPagination,ElProgress,ElInputNumber,ElForm,ElFormItem,ElSwitch,ElRadio,ElRadioGroup,ElRadioButton,ElTag } from 'element-plus'
import 'element-plus/dist/index.css'
import './style.css'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
const app=createApp(App).use(createPinia()).use(router)
for (const [name,component] of Object.entries({ElAlert,ElButton,ElInput,ElSkeleton,ElTable,ElTableColumn,ElDialog,ElSelect,ElOption,ElCheckbox,ElCheckboxGroup,ElPagination,ElProgress,ElInputNumber,ElForm,ElFormItem,ElSwitch,ElRadio,ElRadioGroup,ElRadioButton,ElTag})) app.component(name,component)
window.addEventListener('media-tool:session-expired',()=>{const auth=useAuthStore();auth.authenticated=false;auth.checked=true;router.replace('/login')})
app.mount('#app')
