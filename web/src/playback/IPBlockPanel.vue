<script setup lang="ts">
import {reactive,ref} from 'vue';
import {listIPBlocks,checkIPBlock,deleteIPBlock,type IPBlockPage,type IPBlockRule,type IPBlockCheck} from '../api/ip-blocklist';
import {useRefresh} from '../composables/useRefresh';
import {useResource} from '../workflows/useResource';
import {usePageNumber,usePageSize} from '../ui/page-query';
import {errorText,date} from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import IPBlockEditor from './IPBlockEditor.vue';
const page=usePageNumber(),pageSize=usePageSize(25),q=ref(''),query=ref(''),data=reactive(useResource<IPBlockPage>(()=>listIPBlocks(page.value,query.value,pageSize.value),{items:[],total:0,revision:0,status:{available:false,retrySeconds:0,counterWarning:false},currentIp:'',maxRules:10000})),editor=ref<InstanceType<typeof IPBlockEditor>>();
const target=ref<IPBlockRule>(),targetRevision=ref(0),confirm=ref(false),busy=ref(false),error=ref(''),uncertain=ref(false),address=ref(''),result=ref<IPBlockCheck>(),checking=ref(false),checkError=ref('');
async function load(){await data.load();if(result.value&&result.value.revision!==data.data.revision)result.value=undefined;if(!data.error)uncertain.value=false}
const refresh=useRefresh(load,{interval:5000});
function search(){page.value=1;query.value=q.value.trim();void load()}
function askDelete(rule:IPBlockRule){target.value=rule;targetRevision.value=data.data.revision;confirm.value=true}
let checkGeneration=0;
function invalidateCheck(){checkGeneration++;result.value=undefined}
async function remove(){if(!target.value)return;busy.value=true;error.value='';try{await deleteIPBlock(target.value.id,targetRevision.value);confirm.value=false;await load()}catch(e){error.value=errorText(e);uncertain.value=true;confirm.value=false;await load()}finally{busy.value=false}}
async function check(){const ticket=++checkGeneration,requested=address.value.trim();checking.value=true;checkError.value='';result.value=undefined;try{const value=await checkIPBlock(requested);if(ticket===checkGeneration&&requested===address.value.trim()){if(value.revision!==data.data.revision)checkError.value='规则已变更，请刷新后重新预检';else result.value=value}}catch(e){if(ticket===checkGeneration)checkError.value=errorText(e)}finally{checking.value=false}}

defineExpose({load:refresh.refresh});
</script>
<template>
 <section class="media-surface ip-block-panel">
  <header class="media-section-heading"><div><h2>IP 黑名单</h2><p>拒绝指定 IP 或网段的媒体访问；管理后台始终可用于解封。</p></div><el-button type="primary" :disabled="!data.data.status.available||busy||!!data.error" @click="editor?.open()">添加 IP 规则</el-button></header>
  <el-alert v-if="data.loaded&&!data.data.status.available" :title="data.data.status.message||'IP 黑名单策略暂不可用，媒体请求返回 503；后台正在自动重试，成功后自动恢复。'" :description="'媒体入口返回 503；后台自动重试并在成功后恢复，管理后台不受影响。'" type="error" :closable="false"/>
  <el-alert v-if="data.data.status.counterWarning" title="命中计数暂未刷盘，正在自动重试；IP 黑名单阻断不受影响。" type="warning" :closable="false"/>
  <form class="media-actions ip-block-search" @submit.prevent="search"><el-input v-model="q" aria-label="搜索 IP 黑名单规则" placeholder="搜索 IP、网段或备注"/><el-button native-type="submit">搜索</el-button><el-button :loading="data.loading" @click="load">刷新规则</el-button></form>
  <p>最多 {{data.data.maxRules}} 条规则；重叠时计入最精确网段，命中计数约每 5 秒刷盘。</p>
  <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" :empty="data.data.status.available&&!data.data.items.length" empty-title="暂无 IP 黑名单规则" @reload="load"/>
  <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  <div v-for="rule in data.data.items" :key="rule.id" class="organize-record ip-block-row"><div><h3 :title="rule.network">{{rule.network}}</h3><p>{{rule.note||'暂无备注'}}</p><p>创建 {{date(rule.createdAt)}} · 命中 {{rule.hitCount}} 次 · 最近命中 {{date(rule.lastHitAt||undefined)}}</p></div><div class="media-actions"><el-button :disabled="busy||uncertain||!!data.error||!data.data.status.available" @click="editor?.open('',rule,data.data.revision)">编辑</el-button><el-button :disabled="busy||uncertain||!!data.error" @click="askDelete(rule)">删除</el-button></div></div>
  <PagePagination v-model:current-page="page" :total="data.data.total" v-model:page-size="pageSize" @current-change="load"/>
  <section class="ip-block-check"><h3>只读命中预检</h3><p>检查运行中规则缓存，不发送媒体请求，不增加命中次数。</p><form class="media-actions" @submit.prevent="check"><el-input v-model="address" aria-label="预检 IP 地址" placeholder="输入 IPv4 或 IPv6" maxlength="64" @input="invalidateCheck"/><el-button native-type="submit" :loading="checking" :disabled="!address.trim()||!data.data.status.available">检查是否命中</el-button></form><p v-if="result" role="status">{{result.matched?'已命中规则：'+result.network:'未命中 IP 黑名单规则'}} · 修订 {{result.revision}}</p><el-alert v-if="checkError" :title="checkError" type="error" :closable="false"/></section>
 </section>
 <IPBlockEditor ref="editor" @saved="load"/>
 <ConfirmAction v-model:visible="confirm" title="删除 IP 黑名单规则" :description="'将删除 '+(target?.network||'此规则')" impact-scope="移除该规则的阻断；重叠规则仍可能拒绝此地址。已排队计数不会重建规则。" confirm-text="确认删除规则" :loading="busy" @confirm="remove"/>
</template>
<style scoped>.ip-block-panel{min-width:0}.ip-block-search{margin:16px 0}.ip-block-search :deep(.el-input),.ip-block-check :deep(.el-input){width:min(100%,360px)}.ip-block-panel :deep(.el-input__wrapper){min-height:44px;box-sizing:border-box}.ip-block-panel :deep(.el-button){min-height:44px;margin:0}.ip-block-row{display:flex;gap:16px;justify-content:space-between;align-items:center}.ip-block-row>div:first-child{min-width:0;flex:1}.ip-block-row h3{overflow-wrap:anywhere}.ip-block-check{margin-top:24px;padding-top:16px;border-top:1px solid var(--mt-border-color)}@media(max-width:600px){.ip-block-row{align-items:flex-start;flex-direction:column}.ip-block-search :deep(.el-input),.ip-block-check :deep(.el-input){width:100%}}</style>
