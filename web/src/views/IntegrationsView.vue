<script setup lang="ts">
import {ref,onMounted,computed} from 'vue'
import {request} from '../api/client'
import {connectionOwner,connectionLabel} from '../integrations/owners'
import StatusBadge from '../components/StatusBadge.vue'
const rows=ref<Array<{id:string;kind:string;name:string;enabled:boolean;lastCheckState:string;lastCheckMessage:string}>>([]),loading=ref(false),error=ref('')
async function refresh(){loading.value=true;error.value='';try{rows.value=(await request<typeof rows.value>('/integrations')).filter(x=>x.kind!=='local')}catch(e){error.value=e instanceof Error?e.message:'读取失败'}finally{loading.value=false}}
const category=ref('all'),state=ref('all')
const categories=[{key:'cloud',name:'云盘',kinds:['open115','clouddrive2']},{key:'media',name:'媒体库',kinds:['emby']},{key:'pt',name:'PT站点',kinds:['ptsite']},{key:'download',name:'下载器',kinds:['qbittorrent']},{key:'notify',name:'通知',kinds:['webhook','telegram','smtp']},{key:'system',name:'系统服务',kinds:['tmdb']}]
const group=(kind:string)=>categories.find(x=>x.kinds.includes(kind))?.key || 'system'
const filtered=computed(()=>rows.value.filter(x=>(category.value==='all'||group(x.kind)===category.value)&&(state.value==='all'||(state.value==='enabled'?x.enabled:!x.enabled))))
onMounted(refresh)
</script>
<template><section class="page-heading"><div><span class="page-kicker">SYSTEM / 连接状态</span><h1>服务连接总览</h1><p>集中查看连接状态。新增和修改请进入对应业务模块，保存时自动验证。</p></div><el-button :loading="loading" @click="refresh">刷新状态</el-button></section><el-alert v-if="error" :title="error" type="error" :closable="false"/><section class="panel"><div class="connection-filters"><el-select v-model="category" aria-label="连接分类"><el-option label="全部服务" value="all"/><el-option v-for="c in categories" :key="c.key" :label="c.name+' ('+rows.filter(x=>group(x.kind)===c.key).length+')'" :value="c.key"/></el-select><el-select v-model="state" aria-label="连接启用状态"><el-option label="全部状态" value="all"/><el-option label="已启用" value="enabled"/><el-option label="未启用" value="disabled"/></el-select><span class="field-hint">{{filtered.length}} 个连接</span></div><el-table :data="filtered" empty-text="尚未配置外部服务"><el-table-column prop="name" label="连接名称"/><el-table-column label="所属模块"><template #default="{row}"><RouterLink :to="connectionOwner(row.kind)">{{connectionLabel(row.kind)}}</RouterLink></template></el-table-column><el-table-column label="状态"><template #default="{row}"><StatusBadge :state="row.lastCheckState"/><span v-if="!row.enabled" class="muted"> · 未启用</span></template></el-table-column><el-table-column prop="lastCheckMessage" label="最近验证结果" min-width="240"/></el-table></section></template>

<style scoped>.connection-filters{display:flex;gap:16px;align-items:center;margin-bottom:24px;flex-wrap:wrap}.connection-filters .el-select{width:200px}@media(max-width:600px){.connection-filters .el-select{width:100%}}</style>
