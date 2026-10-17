<script setup lang="ts">
import {ref,watch,onUnmounted} from 'vue'
import {pluginUI,type PluginPage} from '../api/plugins'
import PluginJson from './PluginJson'
const props=defineProps<{instanceId:string;modelValue:string}>(),emit=defineEmits<{'update:modelValue':[string]}>()
const form=ref<PluginPage>(),error=ref(''),model=ref<Record<string,unknown>>({});let epoch=0
watch(()=>props.instanceId,async()=>{const token=++epoch;form.value=undefined;error.value='';try{const value=await pluginUI(props.instanceId,'form');if(token===epoch)form.value=value}catch{if(token===epoch)error.value='表单暂不可用，仍可使用下方 JSON 配置。'}},{immediate:true})
watch(()=>[props.modelValue,form.value],()=>{try{const value=JSON.parse(props.modelValue);if(value&&typeof value==='object'&&!Array.isArray(value))model.value={...form.value?.defaults,...value}}catch{}},{immediate:true})
function update(value:Record<string,unknown>){model.value=value;emit('update:modelValue',JSON.stringify(value,null,2))}
onUnmounted(()=>epoch++)
</script>
<template><p v-if="error" role="status">{{error}}</p><PluginJson v-if="form" :nodes="form.content" :model="model" @update:model="update"/></template>
