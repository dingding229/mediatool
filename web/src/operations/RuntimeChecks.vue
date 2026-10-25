<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import { request, post } from '../api/client';
import { useRefresh } from '../composables/useRefresh';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { savePrivate } from '../organize/model';
import { date, errorText } from '../workflows/readiness';
import { formatBytes, queueLabel, stateLabel, copyOriginal } from './model';
import { scheduleLabel } from '../presentation/labels';
import ResourceState from '../workflows/ResourceState.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
const route=useRoute(), health=reactive(useResource<any>(()=>request('/system/operations'))), op=reactive(useOperation('system-check'));
const confirm=ref(false), open=ref(false), detail=ref<any>(null), copied=ref(''), copyText=ref('');
const workers=computed<any[]>(()=>health.data?.workers||[]), online=computed(()=>workers.value.filter(w=>w.online).length);
async function load(){
 await health.load();
 if(op.pending){
  const {id}=JSON.parse(op.pending.key);
  if(id)try{const j:any=await request(`/jobs/${encodeURIComponent(id)}`);if(['succeeded','failed','cancelled'].includes(j.state))op.clear();}catch(e){op.error=e;}
 }
}
async function create(){
 const result=await op.run('{}',()=>post('/jobs',{kind:'system.check'}));confirm.value=false;
 if(result?.id&&op.pending){op.pending.key=JSON.stringify({id:result.id});savePrivate('media-tool:u5:system-check',op.pending);}
 await load();
}
useRefresh(load,{interval:30000,active:()=>route.query.tab==='attention'&&!op.busy});
defineExpose({load,create,health,op});
</script>
<template>
 <section class="media-surface f5-runtime-checks" aria-label="系统运行检查">
  <header class="media-section-heading"><h2>运行检查</h2><el-button :disabled="!!op.pending||op.busy" @click="confirm=true">运行系统检查</el-button></header>
  <ResourceState overlay :stale="health.loaded" :loading="!health.loaded&&!health.error" :error="health.error" @reload="load"/>
  <p class="f5-runtime-summary">数据库 {{health.data?.postgres?.state==='ready'?'可用':health.data?.postgres?.state?'不可用':'—'}} · 缓存 {{health.data?.redis?.state==='ready'?'可用':health.data?.redis?.state?'不可用':'—'}} · 数据库体积 {{formatBytes(health.data?.databaseBytes)}} · 执行器在线 {{health.data?.workers?online+' / '+workers.length:'—'}}</p>
  <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
  <AsyncResultBanner v-if="op.phase" :status="op.phase" message="检查请求尚未核验完成，先读取原任务，不重复提交。" @refresh="load"/>
  <details><summary>运行计划与执行器详情</summary>
   <article v-for="worker in workers" :key="worker.owner+':'+worker.queue" class="organize-record"><strong>{{queueLabel(worker.queue)}} · {{worker.online?'在线':'离线'}}</strong><small> · {{date(worker.lastSeenAt)}}</small><el-button @click="detail=worker;open=true">执行器详情</el-button></article>
   <p v-for="q in health.data?.queues||[]" :key="q.name+q.state">{{queueLabel(q.name)}} · {{stateLabel(q.state)}} · {{q.count??'—'}} 项 · 等待重新分配 {{q.expiredLeases??'—'}}</p>
   <article v-for="s in health.data?.schedules||[]" :key="s.id" class="organize-record"><strong>{{s.name}}</strong><p>{{s.enabled?'已启用':'已停用'}} · {{scheduleLabel(s.cron)}} · 下次 {{s.nextRunAt===null?'计划已结束':date(s.nextRunAt)}} · 上次 {{date(s.lastRunAt)}}</p><p v-if="s.error">{{s.error}}</p><RouterLink v-if="s.link?.startsWith('/')" :to="s.link">管理此计划</RouterLink></article>
   <el-button @click="detail=health.data;open=true">技术详情</el-button>
  </details>
  <ConfirmAction v-model:visible="confirm" title="运行系统检查" description="创建一次新的系统检查任务？" impact-scope="只检查当前服务状态，不重放失败业务或修改媒体文件。" confirm-text="确认运行检查" :loading="op.busy" @confirm="create"/>
  <DetailDrawer v-model:visible="open" title="运行检查详情" :technical-data="detail"><el-button @click="copyOriginal(JSON.stringify(detail,null,2),s=>copyText=s).then(s=>copied=s)">复制原始记录</el-button><p>{{copied}}</p><textarea v-if="copyText" class="u6-raw" :value="copyText" readonly aria-label="待复制运行记录"/></DetailDrawer>
 </section>
</template>
<style scoped>
.f5-runtime-checks{min-height:180px;margin-bottom:24px}.f5-runtime-summary{min-height:44px;font-size:13px;line-height:1.8;color:var(--mt-text-muted)}summary{cursor:pointer;min-height:44px;display:flex;align-items:center}.organize-record{overflow-wrap:anywhere}
@media(max-width:600px){.f5-runtime-summary{min-height:88px}}
</style>
