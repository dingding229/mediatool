<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import { subscribeJobEvents } from '../api/client';
import StatusBadge from '../components/StatusBadge.vue';
import TaskControl from '../workflows/TaskControl.vue';
import { pageResult } from './model';
import { onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { request } from '../api/client';
import { useResource } from '../workflows/useResource';
import { date, errorText } from '../workflows/readiness';
import { stateLabel, queueLabel, kindLabel, taskTarget, copyOriginal } from './model';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
const route = useRoute();
const filter = reactive({ state: '', kind: '', queue: '', q: typeof route.query.q === 'string' ? route.query.q : '', page: usePageNumber(), pageSize: 25 }), jobs = reactive(useResource<any>(() => request('/jobs/browse?' + new URLSearchParams(Object.fromEntries(Object.entries(filter).map(([k, v]) => [k, String(v)])))).then(pageResult))), error = ref<unknown>(null), detail = ref<any>(null), open = ref(false), copied = ref(''), copyText = ref('');
async function load() { await jobs.load(); }
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
onMounted(()=>{connect();globalThis.document?.addEventListener('visibilitychange',connect)});
watch(()=>route.query.tab,connect);
onUnmounted(()=>{stream?.close();globalThis.document?.removeEventListener('visibilitychange',connect)});
defineExpose({ load, filter, jobs });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface f3-queue-list">
      <h2>任务列表</h2>
      <form class="u5-filters" @submit.prevent="search">
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
      <div class="f4-queue-rows">
      <ResourceState :stale="jobs.loaded" :loading="jobs.loading" :error="jobs.error" :empty="!!jobs.data&&!jobs.data.items?.length" @reload="load"/>
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <article v-for="j in jobs.data?.items||[]" :key="j.id" class="organize-record f3-job-row">
        <div class="f4-job-summary"><h3>{{ kindLabel(j.kind) }}</h3><StatusBadge :state="j.state" :label="stateLabel(j.state)" :title="j.lastError||undefined"/><span class="f4-job-queue">{{ queueLabel(j.queue) }}</span></div>
        <p class="f3-job-meta">尝试 {{ j.attempts??'—' }}/{{ j.maxAttempts??'—' }} · {{ date(j.updatedAt) }} · 耗时 {{ j.startedAt ? Math.max(0,Math.round(((j.state==='running'?Date.now():Date.parse(j.updatedAt))-Date.parse(j.startedAt))/1000)) : '—' }} 秒 <span> · {{ j.state==='succeeded'?'已完成':j.state==='running'?'业务进度见详情':'等待状态核对' }}</span></p>
        <div class="media-actions">
          <el-button @click="inspect(j.id)">任务详情</el-button>
          <TaskControl kind="jobs" :id="j.id" :state="j.state" @changed="jobs.load"/>
          <RouterLink v-if="taskTarget(j)" :to="taskTarget(j)!">前往业务页核对</RouterLink>
        </div>
      </article>
      </div>
      <PagePagination v-model:current-page="filter.page" :total="jobs.data.total" :page-size="25" layout="prev,pager,next,total" @current-change="changePage"/>
    </section>
    <DetailDrawer v-model:visible="open" title="任务与运行技术详情" :technical-data="detail" class="media-detail">
      <el-button @click="copyOriginal(JSON.stringify(detail,null,2),s=>copyText=s).then(s=>copied=s)">复制原始记录</el-button>
      <p>{{ copied }}</p>
      <textarea v-if="copyText" class="u6-raw" :value="copyText" readonly aria-label="待复制任务记录"/>
    </DetailDrawer>
  </div>
</template>
