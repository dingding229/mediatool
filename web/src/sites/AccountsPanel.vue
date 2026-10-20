<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { ElDropdown, ElDropdownMenu, ElDropdownItem } from 'element-plus';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AccountEditor from './AccountEditor.vue';
import SchedulePanel from './SchedulePanel.vue';
import { bytes, date, errorText } from '../workflows/readiness';
import { publicAccount, numberText, addSamples, chartPoints } from './accountModel';
const accounts = reactive(useResource<any>(() => request<any[]>('/integrations').then(rows => rows.filter(r => r.kind === 'ptsite').map(publicAccount)), [])), connections = accounts, metrics = reactive(useResource<any>(() => request('/sites/metrics'), [])), op = reactive(useOperation('site-actions')), query = ref(''), editor = ref<InstanceType<typeof AccountEditor>>(), detailId = ref(''), detailOpen = ref(false), schedule = ref<InstanceType<typeof SchedulePanel>>(), confirm = ref(false), action = ref<any>(null), notice = ref(''), samples = reactive<Record<string, any[]>>({});
const rows = computed(() => accounts.data.map((a: any) => ({ ...a, connection: connections.data.find((r: any) => r.id === a.id), metric: metrics.data.find((m: any) => m.accountId === a.id) }))), shown = computed(() => rows.value.filter((r: any) => r.name.toLowerCase().includes(query.value.toLowerCase()))), detail = computed(() => rows.value.find((r: any) => r.id === detailId.value)), series = computed(() => samples[detailId.value] || []);
const siteState = (s: string) => ({ ready: '检测通过', partial: '部分数据', unchecked: '尚未检测', failed: '失败', running: '采集中', succeeded: '已采集', not_synced: '尚未采集', skipped: '已跳过', unavailable: '暂不可用' } as Record<string, string>)[s] || '状态待核对';
function total(key: string) {
  const valid = metrics.data.filter((r: any) => r.enabled && typeof r.metrics?.[key] === 'number' && Number.isFinite(r.metrics[key]));
  return { value: valid.length ? valid.reduce((n: number, r: any) => n + r.metrics[key], 0) : null, count: valid.length };
}
async function load() {
  const [, out] = await Promise.all([accounts.load(), metrics.load()]);
  void schedule.value?.load();
  if (out)
    addSamples(samples, out);
  if (op.pending) {
    const p = JSON.parse(op.pending.key), row = connections.data.find((r: any) => r.id === p.id);
    if (p.action === 'delete' && !accounts.error && !connections.error && !accounts.data.some((r: any) => r.id === p.id) && !row)
      op.clear();
    else if (p.action === 'toggle' && !connections.error && row?.revision !== p.revision && row?.enabled === p.enabled)
      op.clear();
    else if (p.action === 'test' && !connections.error && row?.lastCheckedAt && row.lastCheckedAt !== p.checkedAt)
      op.clear();
    else if (p.action === 'sync' && !metrics.error) {
      if (op.pending.status === 200) {
        op.clear();
        return;
      }
      const relevant = metrics.data.filter((m: any) => !p.id || m.accountId === p.id);
      if (relevant.some((m: any) => m.attemptedAt && m.attemptedAt !== p.attempts[m.accountId]))
        op.clear();
    }
  }
}
function saved(message: string) {
  notice.value = message;
  void load();
}
function create() {
  editor.value?.open();
}
function ask(kind: string, row: any) {
  action.value = { kind, row: JSON.parse(JSON.stringify(row)) };
  confirm.value = true;
}
async function apply() {
  const a = action.value, r = a.row.connection;
  if (!r)
    return;
  await op.run(JSON.stringify({ action: a.kind, id: r.id, revision: r.revision, enabled: !r.enabled }), () => a.kind === 'delete' ? request(`/integrations/${r.id}`, { method: 'DELETE' }) : request(`/integrations/${r.id}`, { method: 'PUT', body: JSON.stringify({ kind: 'ptsite', name: r.name, enabled: !r.enabled, settings: r.settings, revision: r.revision }) }));
  confirm.value = false;
  await load();
}
async function test(row: any) {
  const r = row.connection;
  if (!r)
    return;
  await op.run(JSON.stringify({ action: 'test', id: r.id, revision: r.revision, checkedAt: r.lastCheckedAt }), () => post(`/integrations/${r.id}/test`, {}));
  await load();
}
async function sync(id = '') {
  const attempts = Object.fromEntries(metrics.data.map((m: any) => [m.accountId, m.attemptedAt]));
  const out = await op.run(JSON.stringify({ action: 'sync', id, attempts }), () => post('/sites/metrics/sync', { accountId: id }));
  await load();
  if (out !== undefined && op.phase !== 'queued') {
    notice.value = `服务回执：尝试 ${out.attempted} 个账号，失败 ${out.failed ?? 0} 个。${out.remainingMayExist ? '本轮最多16站，其余请逐站采集。' : ''}本轮可能跳过正在处理或配置变化的站点，请逐站核对当前结果。`;
  }
}
let timer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  void load();
  timer = setInterval(() => {
    if (!document.hidden && !op.busy)
      void load();
  }, 30000);
});
onUnmounted(() => clearInterval(timer));
defineExpose({ load, create, rows, metrics, accounts, connections, sync, test, op, ask, apply });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>站点账号</h2>
        <div class="media-actions">
          <el-button :loading="metrics.loading" @click="load">读取当前记录</el-button>
          <el-button :disabled="!!op.pending||op.busy||!!metrics.error" @click="sync()">采集启用站点</el-button>
        </div>
      </header>
      <p>按站点管理登录信息和订阅地址，查看最近采集的数据。</p>
      <div class="u5-stats">
        <div>
          <span>已添加站点</span>
          <strong>{{ accounts.loading||accounts.error?'—':accounts.data.length }}</strong>
        </div>
        <div>
          <span>最近上传汇总</span>
          <strong>{{ metrics.loading||metrics.error?'—':bytes(total('uploaded').value) }}</strong>
          <small>已读取 {{ total('uploaded').count }} 个启用站点的快照</small>
        </div>
        <div>
          <span>最近下载汇总</span>
          <strong>{{ metrics.loading||metrics.error?'—':bytes(total('downloaded').value) }}</strong>
          <small>已读取 {{ total('downloaded').count }} 个启用站点的快照</small>
        </div>
      </div>
      <label class="u5-field">
        按名称筛选
        <input v-model="query" aria-label="筛选站点账号"/>
      </label>
      <ResourceState :loading="accounts.loading" :error="accounts.error" :empty="!shown.length" empty-title="暂无匹配的站点账号" @reload="load"/>
      <ResourceState :error="metrics.error" @reload="load"/>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <p v-if="notice" role="status">{{ notice }}</p>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'站点操作已排队，尚未核验完成。':'正在核对上次站点操作，不重复提交。'" @refresh="load"/>
      <article v-for="row in shown" :key="row.id" class="organize-record u5-metric-card">
        <header class="media-section-heading">
          <div class="u5-heading">
            <img v-if="row.metric?.iconUrl?.startsWith('/api/v1/pt/templates/')" :src="row.metric.iconUrl" :alt="row.name+'图标'"/>
            <h3>{{ row.name }}</h3>
          </div>
          <span>{{ row.connection?row.connection.enabled?'已启用':'已停用':'开关状态 —' }}</span>
        </header>
        <p>登录：{{ siteState(row.connection?.lastCheckState||row.state) }} · {{ date(row.connection?.lastCheckedAt) }} · 订阅地址：{{ row.settings.rssState==='ready'?'已获取':row.settings.rssState==='unavailable'?'尚未获取':'—' }}</p>
        <div class="u5-stats">
          <div>
            <span>上传</span>
            <strong>{{ bytes(row.metric?.metrics?.uploaded) }}</strong>
          </div>
          <div>
            <span>下载</span>
            <strong>{{ bytes(row.metric?.metrics?.downloaded) }}</strong>
          </div>
          <div>
            <span>分享率</span>
            <strong>{{ numberText(row.metric?.metrics?.ratio) }}</strong>
          </div>
        </div>
        <p>{{ row.metric?siteState(row.metric.state):'尚无采集记录' }} · {{ date(row.metric?.collectedAt) }}{{ row.metric?.stale?' · 快照已过期，请重新采集':'' }}</p>
        <p v-if="row.metric?.message">{{ row.metric.message }}</p>
        <div class="media-actions">
          <el-button @click="detailId=row.id;detailOpen=true">数据与趋势</el-button>
          <el-button @click="editor?.open(row.id)">编辑账号</el-button>
          <el-dropdown trigger="click" popper-class="media-dropdown">
            <el-button aria-label="更多站点操作">更多</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item :disabled="!row.connection||!!connections.error||!!op.pending" @click="test(row)">测试已保存账号</el-dropdown-item>
                <el-dropdown-item :disabled="!!op.pending||!!metrics.error" @click="sync(row.id)">采集此站点</el-dropdown-item>
                <el-dropdown-item :disabled="!row.connection||!!connections.error||!!op.pending" @click="ask('toggle',row)">{{ row.connection?.enabled?'停用账号':'启用账号' }}</el-dropdown-item>
                <el-dropdown-item divided :disabled="!row.connection||!!connections.error||!!op.pending" @click="ask('delete',row)">删除账号</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </article>
    </section>
    <SchedulePanel ref="schedule"/>
  </div>
  <AccountEditor ref="editor" @saved="saved"/>
  <ConfirmAction v-model:visible="confirm" :title="action?.kind==='delete'?'删除站点账号':action?.row.connection?.enabled?'停用站点账号':'启用站点账号'" :description="`将处理 ${action?.row.name||'此站点'}。`" :impact-scope="action?.kind==='delete'?'移除登录信息及关联站点指标，不删除媒体文件；相关订阅需要重新核对站点。':'保留凭据与记录，修改是否参与搜索、订阅及采集。现有服务在停用前也会验证登录；验证失败时不改开关，不清空凭据绕过。'" :confirm-text="action?.kind==='delete'?'确认删除账号':'确认修改开关'" :loading="op.busy" :danger-level="action?.kind==='delete'?'danger':'warning'" class="media-dialog" @confirm="apply"/>
  <DetailDrawer v-model:visible="detailOpen" title="站点数据与趋势" :technical-data="detail?.metric" class="media-detail">
    <h3>{{ detail?.name||'—' }}</h3>
    <dl class="u5-detail-list">
      <div v-for="(text,key) in {username:'站内用户名',level:'等级',bonus:'积分',seeding:'做种数'}" :key="key">
        <dt>{{ text }}</dt>
        <dd>{{ numberText(detail?.metric?.metrics?.[key]) }}</dd>
      </div>
      <div>
        <dt>做种体积</dt>
        <dd>{{ bytes(detail?.metric?.metrics?.seedingBytes) }}</dd>
      </div>
      <div>
        <dt>最近尝试</dt>
        <dd>{{ date(detail?.metric?.attemptedAt) }}</dd>
      </div>
    </dl>
    <p v-for="(note,index) in detail?.metric?.metrics?.notes||[]" :key="index">{{ note }}</p>
    <h3>本页采样趋势</h3>
    <p>服务暂不提供历史趋势；这里只记录本页打开后读到的真实采集快照，最多60次，不跨页面保存。</p>
    <svg v-if="chartPoints(series,'uploaded')||chartPoints(series,'downloaded')" viewBox="0 0 500 150" class="u5-chart" role="img" aria-label="上传与下载采样趋势">
      <polyline :points="chartPoints(series,'uploaded')"/>
      <polyline class="u5-download-line" :points="chartPoints(series,'downloaded')"/>
    </svg>
    <p>实线为上传、虚线为下载，分别按各自范围显示走势；具体数值见下方记录。</p>
    <p v-if="!chartPoints(series,'uploaded')&&!chartPoints(series,'downloaded')">暂无足够的有效采样，至少需要两次不同时间的采集结果。</p>
    <article v-for="sample in series" :key="sample.at" class="organize-record">
      <p>{{ date(sample.at) }} · 上传 {{ bytes(sample.uploaded) }} · 下载 {{ bytes(sample.downloaded) }}</p>
    </article>
  </DetailDrawer>
</template>
