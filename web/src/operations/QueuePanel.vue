<script setup lang="ts">
import TaskControl from '../workflows/TaskControl.vue';
import {scheduleLabel} from '../presentation/labels';
import { pageResult } from './model';
import { formatBytes as bytes } from './model';
import { onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { savePrivate } from '../organize/model';
import { date, errorText } from '../workflows/readiness';
import { stateLabel, queueLabel, kindLabel, taskTarget, copyOriginal } from './model';
import ResourceState from '../workflows/ResourceState.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
const route = useRoute();
const filter = reactive({ state: '', kind: '', queue: '', q: typeof route.query.q === 'string' ? route.query.q : '', page: 1, pageSize: 25 }), jobs = reactive(useResource<any>(() => request('/jobs/browse?' + new URLSearchParams(Object.fromEntries(Object.entries(filter).map(([k, v]) => [k, String(v)])))).then(pageResult))), health = reactive(useResource<any>(() => request('/system/operations'))), op = reactive(useOperation('system-check')), detail = ref<any>(null), open = ref(false), confirm = ref(false), copied = ref(''), copyText = ref('');
async function load() {
  await Promise.all([jobs.load(), health.load()]);
  if (op.pending) {
    const p = JSON.parse(op.pending.key);
    if (p.id)
      try {
        const current: any = await request(`/jobs/${encodeURIComponent(p.id)}`);
        if (['succeeded', 'failed', 'cancelled'].includes(current.state))
          op.clear();
      }
      catch (e) {
        op.error = e;
      }
  }
}
async function create() {
  const out = await op.run('{}', () => post('/jobs', { kind: 'system.check' }));
  confirm.value = false;
  if (out?.id && op.pending) {
    op.pending.key = JSON.stringify({ id: out.id });
    savePrivate('media-tool:u5:system-check', op.pending);
  }
  await load();
}
async function inspect(id: string) {
  try {
    detail.value = await request(`/jobs/${encodeURIComponent(id)}`);
    open.value = true;
  }
  catch (e) {
    op.error = e;
  }
}
function search() {
  jobs.data = null;
  filter.page = 1;
  void load();
}
function changePage() {
  jobs.data = null;
  void load();
}
watch(() => route.query.q, value => {
  jobs.data = null;
  filter.q = typeof value === 'string' ? value : '';
  filter.page = 1;
  void load();
});
let timer: ReturnType<typeof setInterval>, stream: EventSource | undefined;
onMounted(() => {
  if (typeof EventSource !== 'undefined') {
    stream = new EventSource('/api/v1/jobs/events');
    stream.addEventListener('tick', () => {
      if (!document.hidden && !jobs.loading)
        void jobs.load();
    });
    stream.onerror = () => stream?.close();
  }
  void load();
  let ticks = 0;
  timer = setInterval(() => {
    if (document.hidden || op.busy)
      return;
    void jobs.load();
    if (++ticks % 3 === 0)
      void load();
  }, 5000);
});
onUnmounted(() => {
  clearInterval(timer);
  stream?.close();
});
defineExpose({ load, create, filter, jobs, health, op });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>运行与队列</h2>
        <el-button :disabled="!!op.pending" @click="confirm=true">运行系统检查</el-button>
      </header>
      <ResourceState :loading="health.loading" :error="health.error" @reload="load"/>
      <div class="u5-stats">
        <div>
          <span>数据库</span>
          <strong>{{ health.error?'—':health.data?.postgres?.state==='ready'?'可用':health.data?.postgres?.state?'不可用':'—' }}</strong>
          <small>{{ health.data?.postgres?.latencyMs??'—' }} ms</small>
        </div>
        <div>
          <span>缓存服务</span>
          <strong>{{ health.error?'—':health.data?.redis?.state==='ready'?'可用':health.data?.redis?.state?'不可用':'—' }}</strong>
        </div>
        <div>
          <span>数据库体积</span>
          <strong>{{ health.error?'—':bytes(health.data?.databaseBytes) }}</strong>
        </div>
      </div>
      <div class="u6-card-grid">
        <article v-for="w in health.error?[]:health.data?.workers||[]" :key="w.owner+':'+w.queue" class="u6-card">
          <h3>{{ w.queue==='default'?'后台任务服务':queueLabel(w.queue)+'服务' }}</h3>
          <p>{{ w.online?'在线':'离线' }} · {{ date(w.lastSeenAt) }}</p>
          <el-button @click="detail=w;open=true">执行器详情</el-button>
        </article>
      </div>
      <p v-if="!health.loading&&!health.error&&health.data&&!health.data.workers?.length">尚未收到后台任务服务的状态。</p>
      <article v-for="q in health.error?[]:health.data?.queues||[]" :key="q.name+q.state" class="organize-record">
        <p>{{ queueLabel(q.name) }} · {{ stateLabel(q.state) }} · {{ q.count??'—' }} 项 · 等待重新分配 {{ q.expiredLeases??'—' }}</p>
      </article>
      <details>
        <summary>查看运行计划与连接状态</summary>
        <article v-for="s in health.error?[]:health.data?.schedules||[]" :key="s.id" class="organize-record">
          <h3>{{ s.name }}</h3>
          <p>{{ s.enabled?'已启用':'已停用' }} · {{ scheduleLabel(s.cron) }} · 下次 {{ date(s.nextRunAt) }} · 上次 {{ date(s.lastRunAt) }}</p>
          <p v-if="s.error">{{ s.error }}</p>
          <RouterLink v-if="s.link?.startsWith('/')" :to="s.link">管理此计划</RouterLink>
        </article>
        <el-button @click="detail=health.data;open=true">技术详情</el-button>
      </details>
    </section>
    <section class="media-surface">
      <h2>任务列表</h2>
      <form class="u5-filters" @submit.prevent="search">
        <label>
          任务状态
          <el-select v-model="filter.state" aria-label="任务状态">
            <el-option value="" label="全部状态"/>
            <el-option v-for="v in ['pending','running','retry','succeeded','failed','paused','cancelled','unknown','checking']" :key="v" :value="v" :label="stateLabel(v)"/>
          </el-select>
        </label>
        <label>
          队列
          <el-select v-model="filter.queue" aria-label="任务队列">
            <el-option value="" label="全部队列"/>
            <el-option v-for="v in ['default','library','transfer','maintenance']" :key="v" :value="v" :label="queueLabel(v)"/>
          </el-select>
        </label>
        <label>
          任务类别
          <input v-model="filter.kind" aria-label="任务类别"/>
        </label>
        <label>
          搜索任务
          <input v-model="filter.q" aria-label="搜索任务"/>
        </label>
        <el-button native-type="submit">筛选任务</el-button>
      </form>
      <p>取消只停止本程序后续执行；重试先核对原业务状态，不能证明安全时保留未知状态。</p>
      <ResourceState :loading="jobs.loading" :error="jobs.error" :empty="!!jobs.data&&!jobs.data.items?.length" @reload="load"/>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" message="系统检查请求尚未核验完成，请先读取队列，不重复提交。" @refresh="load"/>
      <article v-for="j in jobs.error?[]:jobs.data?.items||[]" :key="j.id" class="organize-record">
        <h3>{{ kindLabel(j.kind) }}</h3>
        <p>{{ stateLabel(j.state) }} · {{ queueLabel(j.queue) }} · 尝试 {{ j.attempts??'—' }}/{{ j.maxAttempts??'—' }} · {{ date(j.updatedAt) }}</p>
        <p v-if="j.lastError">{{ j.lastError }}</p>
        <div class="media-actions">
          <el-button @click="inspect(j.id)">任务详情</el-button>
          <TaskControl kind="jobs" :id="j.id" :state="j.state" @changed="jobs.load"/>
          <RouterLink v-if="taskTarget(j)" :to="taskTarget(j)!">前往业务页核对</RouterLink>
        </div>
      </article>
      <PagePagination v-if="jobs.data?.total>25" v-model:current-page="filter.page" :total="jobs.data.total" :page-size="25" layout="prev,pager,next,total" @current-change="changePage"/>
    </section>
    <ConfirmAction v-model:visible="confirm" title="运行系统检查" description="创建一次新的系统检查任务？" impact-scope="只检查当前服务状态，不重放失败业务或修改媒体文件。" confirm-text="确认运行检查" :loading="op.busy" class="media-dialog" @confirm="create"/>
    <DetailDrawer v-model:visible="open" title="任务与运行技术详情" :technical-data="detail" class="media-detail">
      <el-button @click="copyOriginal(JSON.stringify(detail,null,2),s=>copyText=s).then(s=>copied=s)">复制原始记录</el-button>
      <p>{{ copied }}</p>
      <textarea v-if="copyText" class="u6-raw" :value="copyText" readonly aria-label="待复制任务记录"/>
    </DetailDrawer>
  </div>
</template>
