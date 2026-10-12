<script setup lang="ts">
import {ref,onMounted} from 'vue'
import {request} from '../api/client'
import {connectionOwner,connectionLabel} from '../integrations/owners'
import StatusBadge from '../components/StatusBadge.vue'
const rows=ref<Array<{id:string;kind:string;name:string;enabled:boolean;lastCheckState:string;lastCheckMessage:string}>>([]),loading=ref(false),error=ref('')
async function refresh(){loading.value=true;error.value='';try{rows.value=(await request<typeof rows.value>('/integrations')).filter(x=>x.kind!=='local')}catch(e){error.value=e instanceof Error?e.message:'读取失败'}finally{loading.value=false}}
onMounted(refresh)
</script>
<template><section class="page-heading"><div><span class="page-kicker">SYSTEM / 连接状态</span><h1>服务连接总览</h1><p>集中查看连接状态。新增和修改请进入对应业务模块，保存时自动验证。</p></div><el-button :loading="loading" @click="refresh">刷新状态</el-button></section><el-alert v-if="error" :title="error" type="error" :closable="false"/><section class="panel"><el-table :data="rows" empty-text="尚未配置外部服务"><el-table-column prop="name" label="连接名称"/><el-table-column label="所属模块"><template #default="{row}"><RouterLink :to="connectionOwner(row.kind)">{{connectionLabel(row.kind)}}</RouterLink></template></el-table-column><el-table-column label="状态"><template #default="{row}"><StatusBadge :state="row.lastCheckState"/><span v-if="!row.enabled" class="muted"> · 未启用</span></template></el-table-column><el-table-column prop="lastCheckMessage" label="最近验证结果" min-width="240"/></el-table></section></template>
