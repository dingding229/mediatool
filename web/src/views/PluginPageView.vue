<script setup lang="ts">
import {ref,watch,onUnmounted}from'vue'
import {useRoute}from'vue-router'
import {uiSurfaces}from'../api/plugins'
import PluginSurface from '../plugins/PluginSurface.vue'
const route=useRoute(),plugin=ref(''),error=ref('');let revision=0
watch(()=>route.params.id,async()=>{const token=++revision;plugin.value='';try{const rows=await uiSurfaces();const found=rows.find(i=>i.instanceId===route.params.id);if(token===revision){plugin.value=found?.pluginId??'';error.value=found?'':'插件未启用或不可用'}}catch(e){if(token===revision)error.value=e instanceof Error?e.message:'插件不可用'}},{immediate:true})
onUnmounted(()=>revision++)
</script>
<template><header class="page-header"><h1>插件应用</h1><RouterLink to="/plugins">返回插件库</RouterLink></header><el-alert v-if="error" :title="error" type="warning" :closable="false"/><section v-if="plugin" class="panel"><PluginSurface :key="String(route.params.id)" :instance-id="String(route.params.id)" :plugin-id="plugin" :page-key="String(route.params.key)"/></section></template>
