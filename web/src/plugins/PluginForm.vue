<script setup lang="ts">
import {ref,watch,onUnmounted} from 'vue'
import {pluginUI,type PluginPage} from '../api/plugins'
import PluginJson from './PluginJson'
const props=defineProps<{instanceId:string;modelValue:string;running:boolean}>(),emit=defineEmits<{'update:modelValue':[string]}>()
const loading=ref(false),form=ref<PluginPage>(),error=ref(''),model=ref<Record<string,unknown>>({});let epoch=0
async function load(){const token=++epoch;form.value=undefined;error.value='';loading.value=props.running
 if(!props.running){error.value='实例停用：动态表单和状态页需 worker 运行；可直接保存 JSON，不会自动启用。';return}
 try{const value=await pluginUI(props.instanceId,'form');if(token===epoch)form.value=value}
 catch{if(token===epoch)error.value='表单暂不可用，仍可使用下方 JSON 配置。'}finally{if(token===epoch)loading.value=false}
}
watch(()=>[props.instanceId,props.running],load,{immediate:true})
watch(()=>[props.modelValue,form.value],()=>{try{const value=JSON.parse(props.modelValue);if(value&&typeof value==='object'&&!Array.isArray(value))model.value={...form.value?.defaults,...value}}catch{}},{immediate:true})
function update(value:Record<string,unknown>){model.value=value;emit('update:modelValue',JSON.stringify(value,null,2))}
onUnmounted(()=>epoch++)
</script>
<template><p v-if="loading" role="status">动态表单加载中…</p><el-button v-if="running&&error" :disabled="loading" @click="load">重试读取动态表单</el-button><p v-if="error" role="status">{{error}}</p><PluginJson v-if="form" :nodes="form.content" :model="model" @update:model="update"/></template>
