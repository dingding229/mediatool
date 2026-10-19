<script setup lang="ts">
import {ref,watch}from'vue'
import {useReadState} from '../plugins/read-state'
import ReadFeedback from '../plugins/ReadFeedback.vue'
import {useRoute}from'vue-router'
import {uiSurfaces}from'../api/plugins'
import PluginSurface from '../plugins/PluginSurface.vue'
const route=useRoute(),plugin=ref('')
const listRead=useReadState()
async function load(){plugin.value='';listRead.reset();await listRead.run(()=>uiSurfaces(),rows=>{plugin.value=rows.find(i=>i.instanceId===route.params.id)?.pluginId??''})}
watch(()=>route.params.id,load,{immediate:true})
</script>
<template><header class="page-header"><h1>插件应用</h1><RouterLink to="/plugins">返回插件库</RouterLink></header><ReadFeedback :state="listRead" caption="插件页面状态" @retry="load"/><el-alert v-if="listRead.loaded&&!plugin" title="插件未启用或不可用" type="warning" :closable="false"/><section v-if="plugin" class="panel"><PluginSurface :key="String(route.params.id)" :instance-id="String(route.params.id)" :plugin-id="plugin" :page-key="String(route.params.key)"/></section></template>
