<script setup lang="ts">
import{computed}from'vue'
const props=defineProps<{value:any}>();const emit=defineEmits<{command:[string,Record<string,unknown>]}>()
const columns=computed(()=>props.value?.rows?.length?Object.keys(props.value.rows[0]):[])
</script>
<template><div class="plugin-renderer"><h2 v-if="value?.title">{{value.title}}</h2><p v-if="value?.description">{{value.description}}</p><div class="plugin-metrics"><article v-for="(metric,i) in value?.metrics??[]" :key="i" class="note-panel"><span>{{metric.label}}</span><strong>{{metric.value??'—'}}</strong></article></div><el-table v-if="value?.rows" :data="value.rows" empty-text="插件没有返回记录"><el-table-column v-for="column in columns" :key="column" :prop="column" :label="column" min-width="120"/></el-table><div class="plugin-actions"><el-button v-for="action in value?.actions??[]" :key="action.id" @click="emit('command',action.id,action.data??{})">{{action.label||action.id}}</el-button></div><pre v-if="!value?.rows&&!value?.metrics">{{JSON.stringify(value,null,2)}}</pre></div></template>
<style scoped>.plugin-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin:16px 0}.plugin-metrics strong{display:block;font-size:24px}.plugin-actions{display:flex;gap:12px;flex-wrap:wrap;margin:16px 0}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:400px;overflow:auto}</style>
