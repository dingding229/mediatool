<script setup lang="ts">
import {ref,onMounted} from 'vue';import {request}from'../api/client'
const templates=ref<{id:string;name:string}[]>([]),loading=ref(true),error=ref('')
onMounted(async()=>{try{templates.value=(await request<{templates:{id:string;name:string}[]}>('/pt/templates')).templates}catch(e){error.value=String(e)}finally{loading.value=false}})
</script>
<template><section class="page-heading"><div><h1>站点模板</h1><p>内置 Go 模板，全部自动加载。</p></div></section><el-alert v-if="error" :title="error" type="error" :closable="false"/><el-skeleton v-if="loading" :rows="3" animated/><section v-else class="template-grid"><article v-for="site in templates" :key="site.id" class="panel"><h2>{{site.name}}</h2></article></section></template>
<style scoped>.template-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:16px}.template-grid h2{margin:0}</style>
