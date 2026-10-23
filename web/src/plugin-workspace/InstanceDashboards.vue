<script setup lang="ts">
import {computed,ref,watch,onUnmounted} from 'vue';
import {pluginUI,type DashboardMeta} from '../api/plugins';
import PluginSurface from '../plugins/PluginSurface.vue';
import inventory from '../../../internal/modules/plugins/v2/library.json';
const props=defineProps<{instanceId:string;pluginId:string;running:boolean}>();
const declared=computed(()=>inventory.manifests.find(m=>m.id===props.pluginId)?.extensions.dashboards||[]);
const cards=ref<DashboardMeta[]>([]),error=ref(''),loading=ref(false);
let generation=0,dead=false;
async function load(){
 const current=++generation;error.value='';
 if(!props.running||!declared.value.length){cards.value=[];loading.value=false;return;}
 loading.value=true;
 try{const all=await pluginUI<DashboardMeta[]>(props.instanceId,'meta');if(!dead&&current===generation)cards.value=all.filter(card=>declared.value.includes(card.key));}
 catch{if(!dead&&current===generation)error.value='插件概览暂不可用，可重试；已有配置不受影响。';}
 finally{if(!dead&&current===generation)loading.value=false;}
}
watch([()=>props.instanceId,()=>props.pluginId,()=>props.running],()=>{cards.value=[];void load();},{immediate:true});
onUnmounted(()=>{dead=true;generation++;});
</script>
<template>
 <section v-if="declared.length" class="media-surface f6-plugin-overview" aria-label="插件概览" :aria-busy="loading">
  <h2>概览</h2><p v-if="!running">插件停用时不读取概览，不会为显示此区块启动插件。</p>
  <p v-else-if="loading&&!cards.length" role="status">正在读取插件概览…</p>
  <p v-if="error" role="status">{{error}} <el-button @click="load">重试概览</el-button></p>
  <article v-for="card in cards" :key="card.key"><h3>{{card.title}}</h3><PluginSurface :instance-id="instanceId" :plugin-id="pluginId" kind="dashboard" :page-key="card.key" :refresh="card.refresh"/></article>
 </section>
</template>
<style scoped>.f6-plugin-overview{min-height:160px}.f6-plugin-overview article{min-width:0}.f6-plugin-overview article+article{margin-top:16px}</style>
