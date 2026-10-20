<script setup lang="ts">
import { computed, provide, shallowReactive } from 'vue'
import { useRoute, useRouter, routeLocationKey, routerKey, type RouteLocationNormalizedLoaded, type RouteLocationRaw } from 'vue-router'
import { settingsSections } from '../navigation/pages'
const props = defineProps<{ settings: boolean }>()
const route = useRoute(), router = useRouter()
const query = computed(() => props.settings ? { ...route.query, tab: route.query.section ?? (route.query.connectionKind === 'tmdb' ? 'tmdb' : 'access') } : route.query)
const projected = {} as RouteLocationNormalizedLoaded
for (const key in route) Object.defineProperty(projected, key, { enumerable: true, get: () => key === 'query' ? query.value : route[key as keyof typeof route] })
provide(routeLocationKey, shallowReactive(projected))
function target(value: RouteLocationRaw): RouteLocationRaw {
  if (!props.settings) return value
  const resolved = router.resolve(value)
  if (resolved.path !== '/settings' || !settingsSections.some(s => s.id === resolved.query.tab)) return value
  return { path: '/settings', query: { ...route.query, ...resolved.query, tab: 'general', section: resolved.query.tab }, hash: resolved.hash }
}
provide(routerKey, new Proxy(router, { get(object, key) { if (key === 'push' || key === 'replace') return (value: RouteLocationRaw) => object[key](target(value)); return Reflect.get(object, key) } }))
</script>
<template><slot /></template>
