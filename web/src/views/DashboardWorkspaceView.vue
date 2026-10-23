<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { computed } from 'vue';
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import ServiceReadiness from '../workflows/ServiceReadiness.vue';
import { bytes, date, errorText, stateFor } from '../workflows/readiness';
import {recentSearches,hasResourceCount} from '../dashboard/summaries';
import {useReadinessTest} from '../dashboard/useReadinessTest';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { useDashboard } from '../dashboard/useDashboard';
const props=withDefaults(defineProps<{attention?:boolean}>(),{attention:false});
const { slots, groups, steps, complete, guideVisible, refresh, load, hideGuide } = useDashboard({overview:!props.attention});
const inboxLoading=computed(()=>['jobs','candidates','transfers','manual','batches','alerts','plugins'].some(k=>slots[k]?.loading));
const visibleGroups=computed(()=>groups.value.filter(g=>g.items.length||g.errors.length));
const busy = computed(() => Object.values(slots).some(s => s.loading)), runtime = computed(() => slots.runtime?.data);
const counters = [{ key: 'sites', label: '站点', path: '/sites?tab=accounts' }, { key: 'subscriptions', label: '订阅', path: '/subscriptions' }, { key: 'downloads', label: '下载', path: '/downloads' }, { key: 'media', label: '媒体文件', path: '/library' }, { key: 'todaySearches', label: '今日搜索', path: '/search' }];
const recent = computed(() => recentSearches(slots.recent?.data));
const probe=useReadinessTest(()=>slots.accounts?.error?null:slots.accounts?.data,()=>Promise.all([load('accounts'),load('readiness')]));
usePageRefresh(refresh,'*');
</script>
<template>
  <div class="dashboard-workspace">
    <PageHeader v-if="!attention" title="首页" description="常用业务概览与最近搜索。">
      <template #actions>

      </template>
    </PageHeader>
    <h2 v-if="attention">检测与待处理</h2>
    <ServiceReadiness v-if="attention" :value="slots.readiness?.data" :loading="!!slots.readiness?.loading" :error="slots.readiness?.error" :can-test="probe.available.value" :testing="probe.busy.value" :test-locked="!!probe.phase.value" @test="probe.start" @retry="load('readiness')" />
    <AsyncResultBanner v-if="attention&&probe.phase.value" :status="probe.phase.value" :message="probe.message.value" @refresh="probe.reconcile"/>
    <p v-if="attention&&probe.message.value&&!probe.phase.value" role="status">{{probe.message.value}}</p>
    <section v-if="attention" class="dashboard-inbox u2-surface" aria-label="待处理收件箱">
      <header v-if="visibleGroups.length"><h2>待处理</h2></header>
      <StateView v-if="!visibleGroups.length&&inboxLoading" state="loading" title="正在读取待处理事项…"/>
      <p v-else-if="!visibleGroups.length" class="dashboard-empty">暂无待处理事项</p>
      <div v-if="visibleGroups.length" class="inbox-grid">
        <article v-for="group in visibleGroups" :key="group.id" :aria-label="group.title">
          <h3>{{ group.title }}</h3>
          <p v-if="(group.id === 'uploads' ? ['transfers','manual'] : [group.id]).some(k=>slots[k]?.loading)" role="status">正在读取…</p>
          <template v-else>
            <StateView v-for="source in group.errors" :key="source" :state="stateFor(slots[source]?.error)" title="这一组部分数据暂不可用，数量未知。" :description="errorText(slots[source]?.error)" @retry="load(source)"/>
            <div v-for="item in group.items.slice(0,5)" :key="item.id" class="inbox-item">
              <strong>{{item.title}}</strong>
              <small v-if="item.note">{{item.note}}</small>
              <small v-if="item.count">近期记录 {{item.count}} 次 · 最近 {{date(item.latestAt)}}</small>
              <RouterLink :to="{path:item.path,query:item.query}">去处理<span class="sr-only">{{item.title}}</span></RouterLink>
              <small v-if="item.alert">请在运维页确认；确认告警不等于问题已解决。</small>
            </div>

          </template>
        </article>
      </div>
      <small v-if="slots.jobs?.data?.total>100">失败次数按最近 100 条任务记录统计，更多记录请在运维页查看。</small>
    </section>
    <section v-if="attention&&guideVisible" class="setup-guide u2-surface" aria-label="首次使用引导">
      <header>
        <div>
          <h2>完成配置，开始自动跟进</h2>
          <p>{{ steps.filter(s=>s.done).length }} / 4 项已完成。</p>
        </div>
        <el-button v-if="complete" @click="hideGuide">收起引导</el-button>
      </header>
      <ol>
        <li v-for="(step,index) in steps" :key="step.label">
          <span>{{ step.done?'✓':index+1 }}</span>
          <div>
            <strong>{{ step.label }}</strong>
            <small>{{ step.note||(step.done?'已完成':'待完成或尚未取得有效数据') }}</small>
            <RouterLink :to="{path:step.path,query:{tab:step.tab}}">{{ step.done?'查看配置':'去配置' }}</RouterLink>
          </div>
        </li>
      </ol>
      <p v-if="slots.accounts?.error||slots.classification?.error" role="alert">部分配置读取失败，完成度暂不确定；可刷新状态后再核对。</p>
    </section>
    <section v-if="!attention" class="dashboard-counters" aria-label="业务规模">
      <RouterLink v-for="counter in counters" :key="counter.key" :to="counter.path">
        <span>{{ counter.label }}</span>
        <strong>{{ runtime?.daily?.[counter.key] ?? '—' }}</strong>
      </RouterLink>
    </section>
    <section v-if="!attention" class="u2-surface dashboard-server" aria-label="服务器状态">
      <h2>服务器状态</h2>
      <StateView v-if="slots.runtime?.error" :state="stateFor(slots.runtime.error)" title="服务器采样暂不可用" :description="errorText(slots.runtime.error)" @retry="load('runtime')" />
      <dl>
        <div>
          <dt>服务器负载</dt>
          <dd>{{ runtime?.server?.loadAverage?.[0]?.toFixed(2) ?? '—' }}</dd>
          <small>1 / 5 / 15 分钟：{{ runtime?.server?.loadAverage?.map((n:number)=>n.toFixed(2)).join(' / ') || '—' }}</small>
        </div>
        <div>
          <dt>内存占用</dt>
          <dd>{{ bytes(runtime?.server?.memoryUsed) }}</dd>
          <small>总内存 {{ bytes(runtime?.server?.memoryTotal) }}</small>
        </div>
        <div>
          <dt>剩余存储空间</dt>
          <dd>{{ bytes(runtime?.server?.diskAvailable) }}</dd>
          <small>总容量 {{ bytes(runtime?.server?.diskTotal) }}</small>
        </div>
        <div>
          <dt>服务器运行时间</dt>
          <dd>{{ runtime?.server?.uptimeSeconds == null ? '—' : Math.floor(runtime.server.uptimeSeconds/86400)+' 天' }}</dd>
          <small>采样 {{ date(runtime?.server?.sampledAt) }}</small>
        </div>
      </dl>
    </section>
    <section v-if="!attention" class="u2-surface dashboard-searches" aria-label="最近搜索">
      <header>
        <h2>最近搜索</h2>
        <RouterLink to="/search?tab=history">查看搜索记录</RouterLink>
      </header>
      <StateView v-if="slots.recent?.loading" state="loading" title="正在读取最近搜索…" />
      <StateView v-else-if="slots.recent?.error" :state="stateFor(slots.recent.error)" title="搜索记录暂不可用" @retry="load('recent')" />
      <p v-else-if="!recent.length" class="dashboard-empty">暂无搜索记录，前往搜索发现作品。</p>
      <RouterLink v-for="item in recent" :key="item.id" :to="`/search/results/${item.id}`">
        <strong>{{ item.keyword }}</strong>
        <span v-if="hasResourceCount(item.total)">{{ item.total }} 个资源</span>
      </RouterLink>
    </section>
  </div>
</template>
<style scoped>
.dashboard-workspace {
  display:grid;
  gap:24px;
  min-width:0
}
.u2-surface {
  padding:24px;
  border:1px solid var(--mt-border-color);
  border-radius:12px;
  background:var(--mt-bg-surface)
}
header {
  display:flex;
  align-items:center;
  justify-content:space-between;
  gap:16px;
  flex-wrap:wrap;
  margin-bottom:20px
}
header>span,small,.setup-guide p,.dashboard-empty {
  font-size:12px;
  color:var(--mt-text-muted)
}
.inbox-grid {
  display:grid;
  grid-template-columns:repeat(3,minmax(0,1fr));
  gap:24px
}
.inbox-grid article {
  min-width:0
}
.inbox-grid h3 {
  font-size:14px;
  margin-bottom:12px
}
.inbox-item {
  padding:12px 0;
  border-bottom:1px solid var(--mt-border-color);
  font-size:13px;
  overflow-wrap:anywhere
}
.inbox-item a {
  display:inline-flex;
  min-height:44px;
  color:var(--mt-color-primary)
}
.inbox-item small {
  display:block;
  margin-top:6px
}
.inbox-source-error {
  padding:12px;
  background:var(--mt-bg-canvas);
  font-size:12px
}
.dashboard-workspace :deep(.el-button) {
  min-height:44px
}
.setup-guide ol {
  list-style:none;
  padding:0;
  margin:0;
  display:grid;
  grid-template-columns:repeat(4,minmax(0,1fr));
  gap:16px
}
.setup-guide li {
  display:flex;
  gap:12px
}
.setup-guide li>span {
  display:grid;
  place-items:center;
  width:28px;
  height:28px;
  flex-shrink:0;
  border-radius:50%;
  background:var(--mt-color-primary-light);
  color:var(--mt-color-primary)
}
.setup-guide small {
  display:block;
  margin-top:6px
}
.setup-guide a {
  display:inline-flex;
  align-items:center;
  min-height:44px;
  color:var(--mt-color-primary);
  font-size:13px
}
.dashboard-counters {
  display:grid;
  grid-template-columns:repeat(5,minmax(0,1fr));
  gap:16px
}
.dashboard-counters a {
  padding:20px;
  border:1px solid var(--mt-border-color);
  background:var(--mt-bg-surface);
  border-radius:12px
}
.dashboard-counters span,dt {
  color:var(--mt-text-muted);
  font-size:13px
}
.dashboard-counters strong,dd {
  display:block;
  font-size:26px;
  margin:8px 0 0
}
.dashboard-server dl {
  display:grid;
  grid-template-columns:repeat(4,minmax(0,1fr));
  gap:20px;
  margin:20px 0 0
}
.dashboard-server small {
  display:block;
  margin-top:8px;
  overflow-wrap:anywhere
}
.dashboard-searches>a {
  display:flex;
  align-items:center;
  justify-content:space-between;
  gap:16px;
  min-height:48px;
  padding:12px 0;
  border-top:1px solid var(--mt-border-color);
  font-size:13px
}
.dashboard-searches header a {
  min-height:44px;
  display:inline-flex;
  align-items:center;
  color:var(--mt-color-primary);
  font-size:13px
}
@media(max-width:1100px) {
  .inbox-grid,.setup-guide ol,.dashboard-server dl {
    grid-template-columns:1fr 1fr
  }
}
@media(max-width:600px) {
  .u2-surface {
    padding:16px
  }
  .inbox-grid,.setup-guide ol,.dashboard-counters,.dashboard-server dl {
    grid-template-columns:1fr
  }
  .dashboard-counters a {
    display:flex;
    align-items:center;
    justify-content:space-between
  }
  .dashboard-counters strong {
    margin:0
  }
  .dashboard-searches>a {
    flex-wrap:wrap
  }
  .dashboard-workspace {
    gap:16px
  }
}
.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
</style>
