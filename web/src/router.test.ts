import { describe, it, expect } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import { routes } from './router/routes'

describe('Router U1 Routes & Destinations', () => {
  function getRouter() {
    return createRouter({
      history: createMemoryHistory(),
      routes
    })
  }

  it('keeps 13 new primary routes reachable', async () => {
    const primaryRoutes = [
      '/dashboard',
      '/search',
      '/subscriptions',
      '/downloads',
      '/library',
      '/organize',
      '/files',
      '/playback',
      '/connections',
      '/sites',
      '/plugins',
      '/ops',
      '/settings'
    ]
    const router = getRouter()
    for (const p of primaryRoutes) {
      await router.push(p)
      expect(router.currentRoute.value.path).toBe(p)
    }
  })

  it('keeps legacy addresses and integrations redirect reachable', async () => {
    const router = getRouter()
    const legacyRoutes = [
      '/emby',
      '/mediaLibrary',
      '/media-server',
      '/media-users',
      '/player-access',
      '/cloudStorage',
      '/organize-history',
      '/classification',
      '/cloud-connections',
      '/strm',
      '/strm-tools',
      '/maintenance',
      '/jobs',
      '/logs',
      '/data',
      '/notifications',
      '/integrations',
      '/external-api',
      '/tmdb-cache',
      '/search-cache',
      '/site-templates'
    ]
    for (const p of legacyRoutes) {
      await router.push(p)
      expect(router.currentRoute.value.path).toBe(p)
    }

    // test integrations/:kind redirect
    await router.push('/integrations/emby')
    expect(router.currentRoute.value.path).toBe('/mediaLibrary')
    expect(router.currentRoute.value.query.connectionKind).toBe('emby')
  })
})
