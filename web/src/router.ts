import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { routes } from './router/routes'

export { routes }

export const router = createRouter({
  history: createWebHistory(),
  routes
})

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

export default router
