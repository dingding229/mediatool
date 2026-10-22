<script setup lang="ts">
import { useRefresh } from '../composables/useRefresh';
import { markRaw,ref,shallowRef,watch,onUnmounted,type Component } from 'vue'
import {APIError} from '../api/client'
import { pluginUI,type PluginPage } from '../api/plugins'
import { apiClient } from './ui-runtime'
import { loadPluginUI } from './esm'
import inventory from '../../../internal/modules/plugins/v2/library.json'
import PluginJson from './PluginJson'
const props=defineProps<{instanceId:string;pluginId:string;kind?:'page'|'dashboard';pageKey:string;refresh?:number}>()
const data=ref<PluginPage>(),error=ref(''),loading=ref(false),esm=shallowRef<Component>(),mode=ref<'json'|'vue'>('json')
let revision=0,dead=false
async function load(){const token=++revision;loading.value=!data.value;error.value='';try{
 const value=await pluginUI(props.instanceId,props.kind??'page',props.pageKey)
 if(token!==revision||dead)return;data.value=value
 const manifest=inventory.manifests.find(m=>m.id===props.pluginId)
 if(manifest?.ui.mode==='vue'&&(props.kind??'page')==='page'){esm.value=markRaw(await loadPluginUI(props.pluginId));if(token!==revision||dead){esm.value=undefined;return}mode.value='vue'}
 }catch(e){if(token===revision&&!dead){if(e instanceof APIError&&[401,403].includes(e.status)){esm.value=undefined;mode.value='json'}error.value=e instanceof Error?e.message:'插件不可用'}}finally{if(token===revision&&!dead){loading.value=false}}}
function retry(){void load()}
watch(()=>[props.instanceId,props.pageKey,props.kind,props.pluginId],()=>{esm.value=undefined;data.value=undefined;mode.value='json';void load()},{immediate:true})
useRefresh(load, {interval:()=>props.refresh&&props.refresh>=15?Math.min(props.refresh,86400)*1000:0, immediate:false})
onUnmounted(()=>{dead=true;revision++;esm.value=undefined})
</script>
<template><div class="f3-plugin-surface" :aria-busy="loading"><p v-if="loading&&!data" role="status">正在加载插件界面…</p><el-alert v-if="error" :class="{'f3-plugin-stale':!!data}" :title="error" type="warning" :closable="false"/><el-button v-if="error" :class="{'f3-plugin-retry':!!data}" :disabled="loading" @click="retry">重试读取插件界面</el-button><template v-if="data"><div v-if="esm" class="plugin-surface-mode"><p class="field-hint">可信插件界面与管理端同页运行，不是沙箱。</p><el-button @click="mode=mode==='vue'?'json':'vue'">{{mode==='vue'?'查看 JSON 页面':'查看插件界面'}}</el-button></div><component :is="esm" v-if="mode==='vue'&&esm" :instance-id="instanceId" :api="apiClient" :page-key="pageKey"/><PluginJson v-else :nodes="data.content" readonly/></template></div></template>

<style scoped>.f3-plugin-surface{position:relative;min-height:160px}.f3-plugin-stale{position:absolute;top:0;right:0;z-index:2;max-width:360px}.f3-plugin-retry{position:absolute;top:48px;right:0;z-index:2}</style>
