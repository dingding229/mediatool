<script setup lang="ts">
import {ref,onMounted,onUnmounted}from'vue'
import {uiSurfaces,pluginUI,type DashboardMeta}from'../api/plugins'
import PluginSurface from './PluginSurface.vue'
import inventory from '../../../internal/modules/plugins/v2/library.json'
const cards=ref<Array<DashboardMeta&{instanceId:string;pluginId:string}>>([]);let alive=true
onMounted(async()=>{try{const rows=await uiSurfaces();const groups=await Promise.all(rows.filter(i=>inventory.manifests.find(m=>m.id===i.pluginId)?.extensions.dashboards?.length).slice(0,8).map(async i=>{try{return(await pluginUI<DashboardMeta[]>(i.instanceId,'meta')).map(m=>({...m,instanceId:i.instanceId,pluginId:i.pluginId}))}catch{return[]}}));if(alive)cards.value=groups.flat()}catch{}})
onUnmounted(()=>alive=false)
</script>
<template><section v-if="cards.length" class="plugin-dashboards" aria-label="插件仪表盘"><article v-for="card in cards" :key="card.instanceId+':'+card.key" :class="{'panel':card.border}" :style="{'--plugin-cols':String(Math.min(12,Math.max(1,card.cols)))}"><h2>{{card.title}}</h2><PluginSurface :instance-id="card.instanceId" :plugin-id="card.pluginId" kind="dashboard" :page-key="card.key" :refresh="card.refresh"/></article></section></template>
<style scoped>.plugin-dashboards{display:grid;grid-template-columns:repeat(12,minmax(0,1fr));gap:16px;margin-bottom:24px}.plugin-dashboards>article{grid-column:span var(--plugin-cols);min-width:0}@media(max-width:720px){.plugin-dashboards>article{grid-column:span 12}}</style>
