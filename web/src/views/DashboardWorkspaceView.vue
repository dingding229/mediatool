<script setup lang="ts">
import { computed } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import PluginDashboards from '../plugins/PluginDashboards.vue';
import ServiceReadiness from '../workflows/ServiceReadiness.vue';
import { bytes, date, errorText, stateFor } from '../workflows/readiness';
import { list } from '../dashboard/model';
import { useDashboard } from '../dashboard/useDashboard';
const { slots, groups, steps, complete, guideVisible, refresh, load, hideGuide } = useDashboard();
const busy = computed(() => Object.values(slots).some(s => s.loading)), runtime = computed(() => slots.runtime?.data);
const counters = [{ key: 'sites', label: '站点', path: '/sites?tab=accounts' }, { key: 'subscriptions', label: '订阅', path: '/subscriptions' }, { key: 'downloads', label: '下载', path: '/downloads' }, { key: 'media', label: '媒体资产', path: '/library' }, { key: 'todaySearches', label: '今日搜索', path: '/search' }];
const recent = computed(() => list(slots.recent?.data).slice(0, 5));
</script>
<template>
  <div class="dashboard-workspace">
    <PageHeader title="运行总览" description="先看服务是否可用，再处理需要介入的任务。">
      <template #actions>
        <el-button type="primary" :icon="Refresh" :loading="busy" @click="refresh">刷新状态</el-button>
      </template>
    </PageHeader>
    <ServiceReadiness :value="slots.readiness?.data" :loading="!!slots.readiness?.loading" :error="slots.readiness?.error" @retry="load('readiness')" />
    <section class="dashboard-inbox u2-surface" aria-label="待处理收件箱">
      <header>
        <h2>待处理</h2>
        <span>每组最多展示当前读取到的近期事项</span>
      </header>
      <div class="inbox-grid">
        <article v-for="group in groups" :key="group.id" :aria-label="group.title">
          <h3>{{ group.title }}</h3>
          <p v-if="(group.id === 'uploads' ? ['transfers','manual'] : [group.id]).some(k=>slots[k]?.loading)" role="status">正在读取…</p>
          <template v-else>
            <div v-for="source in group.errors" :key="source" class="inbox-source-error" role="alert">
              <span>这一组部分数据暂不可用，数量未知。</span>
              <el-button @click="load(source)">重新读取</el-button>
            </div>
            <div v-for="item in group.items.slice(0,5)" :key="item.id" class="inbox-item">
              <RouterLink :to="{path:item.path,query:item.query}">
                {{ item.title }}
                <small v-if="item.note">{{ item.note }}</small>
              </RouterLink>
              <small v-if="item.alert">请在运维页确认；确认告警不等于问题已解决。</small>
            </div>
            <p v-if="!group.items.length&&!group.errors.length" class="dashboard-empty">暂无待处理事项</p>
          </template>
        </article>
      </div>
    </section>
    <section v-if="guideVisible" class="setup-guide u2-surface" aria-label="首次使用引导">
      <header>
        <div>
          <h2>完成配置，开始自动跟进</h2>
          <p>{{ steps.filter(s=>s.done).length }} / 4 项已完成；状态由服务检测和已保存配置推导。</p>
        </div>
        <el-button v-if="complete" @click="hideGuide">收起引导</el-button>
      </header>
      <ol>
        <li v-for="(step,index) in steps" :key="step.label">
          <span>{{ step.done?'✓':index+1 }}</span>
          <div>
            <strong>{{ step.label }}</strong>
            <small>{{ step.done?'已完成':'待完成或尚未取得有效数据' }}</small>
            <RouterLink :to="{path:step.path,query:{tab:step.tab}}">{{ step.done?'查看配置':'去配置' }}</RouterLink>
          </div>
        </li>
      </ol>
      <p v-if="slots.accounts?.error||slots.classification?.error" role="alert">部分配置读取失败，完成度暂不确定；可刷新状态后再核对。</p>
    </section>
    <section class="dashboard-counters" aria-label="业务规模">
      <RouterLink v-for="counter in counters" :key="counter.key" :to="counter.path">
        <span>{{ counter.label }}</span>
        <strong>{{ runtime?.daily?.[counter.key] ?? '—' }}</strong>
      </RouterLink>
    </section>
    <section class="u2-surface dashboard-server" aria-label="服务器状态">
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
    <section class="u2-surface dashboard-searches">
      <header>
        <h2>最近搜索</h2>
        <RouterLink to="/search?tab=history">查看搜索记录</RouterLink>
      </header>
      <StateView v-if="slots.recent?.loading" state="loading" title="正在读取最近搜索…" />
      <StateView v-else-if="slots.recent?.error" :state="stateFor(slots.recent.error)" title="搜索记录暂不可用" @retry="load('recent')" />
      <p v-else-if="!recent.length" class="dashboard-empty">暂无搜索记录，前往搜索发现作品。</p>
      <RouterLink v-for="item in recent" :key="item.id" :to="`/search/results/${item.id}`">
        <strong>{{ item.keyword }}</strong>
        <span>{{ item.total ?? '—' }} 个资源</span>
      </RouterLink>
    </section>
    <div class="dashboard-plugin-container">
      <PluginDashboards />
    </div>
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
  display:block;
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
.dashboard-plugin-container {
  min-width:0
}
.dashboard-plugin-container :deep(.plugin-dashboards>article) {
  border:1px solid var(--mt-border-color);
  background:var(--mt-bg-surface);
  border-radius:12px;
  padding:20px
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
</style>
