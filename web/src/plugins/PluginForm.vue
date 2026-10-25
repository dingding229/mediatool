<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {staticPluginForm} from '../api/plugins'
import PluginJson from './PluginJson'
import {validateForm} from './form-validation'
const props=defineProps<{pluginId:string;modelValue:string;readonly?:boolean}>(),emit=defineEmits<{'update:modelValue':[string];valid:[boolean]}>()
const form=computed(()=>staticPluginForm(props.pluginId)),model=ref<Record<string,unknown>>({}),parseError=ref('')
watch(()=>[props.modelValue,form.value],()=>{try{const value=JSON.parse(props.modelValue);if(!value||typeof value!=='object'||Array.isArray(value))throw Error();model.value={...form.value?.defaults,...value};parseError.value=''}catch{parseError.value='已保存配置无法读取，请刷新后重试。'}},{immediate:true})
const cronValid=ref(false);
const hasCron=(nodes:any[]):boolean=>nodes.some(n=>n.component==='VCronInput'||hasCron(n.children||[]));
const error=computed(()=>parseError.value||(!form.value?'此插件的设置表单暂不可用，已有配置仍保留。':validateForm(form.value.content,model.value)||(hasCron(form.value.content)&&!cronValid.value?'请先通过插件计划校验':'')))
watch(error,value=>emit('valid',!value),{immediate:true})
function update(value:Record<string,unknown>){model.value=value;emit('update:modelValue',JSON.stringify(value))}
defineExpose({form,model,error})
</script>
<template><p v-if="error" role="alert">{{error}}</p><PluginJson v-if="form" :nodes="form.content" :model="model" :readonly="readonly" @update:model="update" @cron-valid="cronValid=$event"/></template>
