<script setup lang="ts">
import { View, MoreFilled } from '@element-plus/icons-vue';
import {ElDropdown,ElDropdownMenu,ElDropdownItem} from 'element-plus';
import 'element-plus/es/components/dropdown/style/css';
import 'element-plus/es/components/dropdown-menu/style/css';
import 'element-plus/es/components/dropdown-item/style/css';
import {usePageNumber,usePageSize} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import { subscribeJobEvents } from '../api/client';
import StatusBadge from '../components/StatusBadge.vue';
import TaskControl from '../workflows/TaskControl.vue';
import { pageResult } from './model';
import {listConnections} from '../api/connections';
import {jobObject} from '../presentation/job-object';
import { onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { request } from '../api/client';
import { useResource } from '../workflows/useResource';
import { date, errorText } from '../workflows/readiness';
import { stateLabel, queueLabel, kindLabel, taskTarget } from './model';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
const route = useRoute();
const filter = reactive({ state: '', kind: '', queue: '', q: typeof route.query.q === 'string' ? route.query.q : '', page: usePageNumber(), pageSize: usePageSize(25) }), jobs = reactive(useResource<any>(() => request('/jobs/browse?' + new URLSearchParams(Object.fromEntries(Object.entries(filter).map(([k, v]) => [k, String(v)])))).then(pageResult))), error = ref<unknown>(null), detail = ref<any>(null), open = ref(false);
async function load() { await jobs.load(); }
const accounts=reactive(useResource(listConnections,[]));
function jobTitle(row:any){const name=jobObject(row,accounts.data).name;return kindLabel(row.kind)+(name?' · '+name:'')}
async function inspect(id: string) {
  try {
    detail.value = await request(`/jobs/${encodeURIComponent(id)}`);
    open.value = true;
  }
  catch (e) {
    error.value = e;
  }
}
function search() {
  filter.page = 1;
  void load();
}
function changePage() {
  void load();
}
watch(() => route.query.q, value => {
  filter.q = typeof value === 'string' ? value : '';
  filter.page = 1;
  void load();
});
let stream: {close:()=>void} | undefined, lastTick=0;
const connected=ref(false);
async function refreshQueue() {
  lastTick=Date.now();
  await jobs.load();
}
const events=useRefresh(refreshQueue,{interval:()=>connected.value?0:15000,active:()=>(typeof document==='undefined'||(route?.query?.tab||'jobs')==='jobs')});
watch(connected,value=>{if(!value&&(route?.query?.tab||'jobs')==='jobs')void events.refresh()});
// Server SSE currently emits a heartbeat every 3s, not a per-job change event.
// Coalesce those ticks; do not stack an independent 5s network poll on top.
function connect() {
  stream?.close(); stream=undefined;
  if (typeof document === 'undefined' || document.hidden || (route.query.tab||'jobs')!=='jobs') return;
  stream=subscribeJobEvents(()=>{if(Date.now()-lastTick>=15000){lastTick=Date.now();void events.refresh();}},state=>connected.value=state);
}
onMounted(()=>{void accounts.load();connect();globalThis.document?.addEventListener('visibilitychange',connect)});
watch(()=>route.query.tab,connect);
onUnmounted(()=>{stream?.close();globalThis.document?.removeEventListener('visibilitychange',connect)});
defineExpose({ load, filter, jobs });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface f3-queue-list">
      <h2>任务列表</h2>
      <form class="u5-filters ops-job-filters" @submit.prevent="search">
        <label>
          任务状态
          <el-select v-model="filter.state" aria-label="任务状态" placeholder="全部状态">
            <el-option value="" label="全部状态"/>
            <el-option v-for="v in ['pending','running','retry','succeeded','failed','paused','cancelled','unknown','checking']" :key="v" :value="v" :label="stateLabel(v)"/>
          </el-select>
        </label>
        <label>
          队列
          <el-select v-model="filter.queue" aria-label="任务队列" placeholder="全部队列">
            <el-option value="" label="全部队列"/>
            <el-option v-for="v in ['default','library','transfer','maintenance']" :key="v" :value="v" :label="queueLabel(v)"/>
          </el-select>
        </label>
        <label>
          任务类别
          <input v-model="filter.kind" aria-label="任务类别" placeholder="按任务类别筛选"/>
        </label>
        <label>
          搜索任务
          <input v-model="filter.q" aria-label="搜索任务" placeholder="搜索任务或错误内容"/>
        </label>
        <el-button native-type="submit">筛选任务</el-button>
      </form>
      <p>取消只停止本程序后续执行；重试先核对原业务状态，不能证明安全时保留未知状态。</p>
      <div class="mt-list" aria-label="后台任务列表">
      <div class="mt-list-columns ops-job-columns" aria-hidden="true"><span>任务</span><span>状态 / 队列</span><span>执行摘要</span><span>操作</span></div>
      <div class="f4-queue-rows ops-content-stage ops-job-list" :aria-busy="jobs.loading||jobs.refreshing">
      <div v-if="!jobs.loaded&&!jobs.error" class="ops-job-loading" role="status" aria-label="正在读取任务">
        <div v-for="n in 3" :key="n" class="ops-job-loading-row" aria-hidden="true"><el-skeleton :rows="1" animated/></div>
      </div>
      <ResourceState :stale="jobs.loaded" :error="jobs.error" :empty="jobs.loaded&&!jobs.error&&!jobs.data?.items?.length" @reload="load"/>
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <article v-for="j in jobs.data?.items||[]" :key="j.id" class="mt-list-row f3-job-row">
        <div class="ops-job-name"><h3 :title="jobTitle(j)">{{ jobTitle(j) }}</h3></div>
        <div class="ops-job-status"><StatusBadge :state="j.state" :label="stateLabel(j.state)" :title="j.lastError||undefined"/><span class="ops-job-queue">{{queueLabel(j.queue)}}</span></div>
        <div class="ops-job-meta">
          <p class="f3-job-meta"><span class="ops-job-meta-queue">{{queueLabel(j.queue)}} · </span>尝试 {{ j.attempts??'—' }}/{{ j.maxAttempts??'—' }} · {{ date(j.updatedAt) }}</p>
          <p class="ops-job-progress">耗时 {{ j.startedAt ? Math.max(0,Math.round(((j.state==='running'?Date.now():Date.parse(j.updatedAt))-Date.parse(j.startedAt))/1000)) : '—' }} 秒 · {{ j.state==='succeeded'?'已完成':j.state==='running'?'业务进度见详情':'等待状态核对' }}</p>
        </div>
        <div class="mt-list-actions ops-job-actions">
          <el-button size="small" aria-label="任务详情" @click="inspect(j.id)"><el-icon class="mt-list-action-icon"><View/></el-icon><span class="mt-list-action-text">任务详情</span></el-button>
          <TaskControl kind="jobs" :id="j.id" :state="j.state" @changed="jobs.load" v-slot="{ask,disabled,canCancel,canRetry}">
            <ElDropdown v-if="canCancel||canRetry||taskTarget(j)" trigger="click" popper-class="t8-ops-menu">
              <el-button size="small" aria-label="更多"><el-icon class="mt-list-action-icon"><MoreFilled/></el-icon><span class="mt-list-action-text">更多</span></el-button>
              <template #dropdown><ElDropdownMenu>
                <ElDropdownItem v-if="taskTarget(j)"><RouterLink :to="taskTarget(j)!">前往业务页核对</RouterLink></ElDropdownItem>
                <ElDropdownItem v-if="canRetry" :disabled="disabled" @click="ask('retry')">核对后重试</ElDropdownItem>
                <ElDropdownItem v-if="canCancel" :disabled="disabled" @click="ask('cancel')">取消后台任务</ElDropdownItem>
              </ElDropdownMenu></template>
            </ElDropdown>
          </TaskControl>
        </div>
      </article>
      </div>
      </div>
      <PagePagination :status="!jobs.loaded?(jobs.error?'error':'loading'):''" v-model:current-page="filter.page" :total="jobs.data?.total" v-model:page-size="filter.pageSize" layout="prev,pager,next,total" @current-change="changePage"/>
    </section>
    <DetailDrawer v-model:visible="open" title="任务详情" size="large" class="media-detail">
      <template v-if="detail">
        <h3>{{jobTitle(detail)}}</h3>
        <dl class="u5-detail-list">
          <div><dt>状态</dt><dd>{{stateLabel(detail.state)}}</dd></div>
          <div><dt>队列</dt><dd>{{queueLabel(detail.queue)}}</dd></div>
          <div><dt>执行次数</dt><dd>{{detail.attempts??'—'}} / {{detail.maxAttempts??'—'}}</dd></div>
          <div><dt>创建时间</dt><dd>{{date(detail.createdAt)}}</dd></div>
          <div><dt>最近更新</dt><dd>{{date(detail.updatedAt)}}</dd></div>
        </dl>
        <p v-if="detail.lastError">{{detail.lastError}}</p>
      </template>
    </DetailDrawer>
  </div>
</template>
