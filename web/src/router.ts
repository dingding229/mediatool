import {beginActivity} from './api/activity'
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { routes } from './router/routes'
import { installPageRedirects } from './router/redirects'
import { installChunkRecovery } from './router/chunk-recovery'
import {ElMessage} from 'element-plus'
import 'element-plus/es/components/message/style/css'

export { routes }

export const router = createRouter({
  history: createWebHistory(),
  routes
})

installPageRedirects(router)
const navigation=new Map<string,()=>void>()
router.beforeEach(to=>{navigation.get(to.fullPath)?.();navigation.set(to.fullPath,beginActivity())})
router.afterEach(to=>{navigation.get(to.fullPath)?.();navigation.delete(to.fullPath)})
router.onError(()=>{for(const finish of navigation.values())finish();navigation.clear()})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  try {
    await auth.ensure()
  } catch {
    return to.name === 'login' ? true : { name: 'login' }
  }
  if (auth.authenticated && auth.account?.mustChangePassword && to.name !== 'resetPassword') {
    return { name: 'resetPassword' }
  }
  if (to.name === 'resetPassword' && auth.authenticated && !auth.account?.mustChangePassword) {
    return { name: 'dashboard' }
  }
  if (!to.meta.public && !auth.authenticated) return { name: 'login' }
  if (to.name === 'login' && auth.authenticated) return { name: 'dashboard' }
  return true
})

if(typeof window!=='undefined')installChunkRecovery(router,()=>{ElMessage.error('页面资源加载失败，请刷新页面后重试。')})

export default router
