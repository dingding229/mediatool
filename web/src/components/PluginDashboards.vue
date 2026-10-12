<script setup lang="ts">
import{ref,onMounted}from'vue';import{request}from'../api/client';import PluginRenderer from'./PluginRenderer.vue'
const cards=ref<any[]>([])
onMounted(async()=>{try{const list=await request<any[]>('/plugins');for(const plugin of list.filter(x=>x.enabled&&x.manifest.dashboard).slice(0,8)){try{cards.value.push({id:plugin.manifest.id,name:plugin.manifest.name,value:await request(`/plugins/${plugin.manifest.id}/view/dashboard/main`)})}catch{cards.value.push({id:plugin.manifest.id,name:plugin.manifest.name,error:'插件仪表盘暂不可用'})}}}catch{}})
</script>
<template><section v-if="cards.length" class="plugin-dashboard-grid"><article v-for="card in cards" :key="card.id" class="panel"><h2>{{card.name}}</h2><el-alert v-if="card.error" :title="card.error" type="warning" :closable="false"/><PluginRenderer v-else :value="card.value"/></article></section></template>
<style scoped>.plugin-dashboard-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px;margin-top:24px}</style>
