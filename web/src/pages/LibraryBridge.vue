<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import MediaLibraryView from '../views/MediaLibraryView.vue'
import EmbyView from '../views/EmbyView.vue'
const route = useRoute(), gatewayOpen = ref(route.query.connectionKind === 'emby')
watch(() => route.query.connectionKind, kind => { if (kind === 'emby') gatewayOpen.value = true })
function toggle(event: Event) { gatewayOpen.value = (event.target as HTMLDetailsElement).open }
</script>
<template><MediaLibraryView /><details class="library-gateway" :open="gatewayOpen" @toggle="toggle"><summary>Emby 网关状态与诊断</summary><EmbyView v-if="gatewayOpen" /></details></template>
<style scoped>.library-gateway { margin-top: 24px; border-top: 1px solid var(--mt-border-color); padding-top: 16px; }.library-gateway summary { min-height: 44px; cursor: pointer; color: var(--mt-color-primary); font-size: 14px; }</style>
