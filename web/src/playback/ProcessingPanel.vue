<script setup lang="ts">
import {usePageNumber,usePageSize} from '../ui/page-query';
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { getProcessing, saveProcessing, listProcessing, retryProcessing, collectProcessing } from '../api/mediaServer';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText, bytes } from '../workflows/readiness';
const props = withDefaults(defineProps<{
  initialKind?: string;
  initialState?: string;
  jobId?: string;
}>(), { initialKind: '', initialState: '', jobId: '' });
const filter = reactive<any>({ kind: props.initialKind === 'still' ? 'still' : 'probe', state: props.jobId ? '' : 'failed', page: usePageNumber(), pageSize: usePageSize(50) }), summary = reactive(useResource(getProcessing)), tasks = reactive(useResource(() => listProcessing(filter.kind, filter.state, filter.page, filter.pageSize))), form = reactive({ enabled: false, probeEnabled: true, stillsEnabled: true, revision: 0 }), latest = ref<any>(null), op = reactive(useOperation('processing')), confirm = ref(false), action = ref<any>(null), notice = ref('');
const states: Record<string, string> = { pending: '等待中', probing: '探测中', grabbing: '截图中', ready: '已就绪', failed: '失败', needs_action: '需人工处理', skipped: '已跳过' }, reasons: Record<string, string> = { not_exposable: '媒体暂不可用', file_missing: '文件不存在', strm_mismatch: '播放文件与记录不一致', authorization: '115 授权失效，请重新授权', rate_limited: '请求限流', timeout: '执行超时', unavailable: '处理工具不可用', unsafe_target: '播放地址不安全', ua_mismatch: '播放器标识不一致', ticket_unavailable: '播放授权暂不可用', lease_expired: '任务超时，需要核对', process_failed: '媒体处理失败', invalid_result: '结果无效', invalid_image: '截图无效', tmdb_still: '已有单集图片', source_unavailable: '没有可用截图来源', storage_unavailable: '截图缓存暂不可用', storage_full: '截图缓存空间不足', cache_evicted: '截图已被回收，可按条件重新获取', schema_outdated: '记录需要更新', link_timeout: '解析超时', probe_failed: '探测失败' };
const validStates = computed(() => filter.kind === 'probe' ? ['pending', 'probing', 'ready', 'failed', 'needs_action'] : ['pending', 'grabbing', 'ready', 'failed', 'needs_action', 'skipped']);
const currentStates = computed(() => filter.kind === 'probe' ? summary.data?.probe : summary.data?.still), capacityPercent = computed(() => summary.data?.capacity?.limitBytes ? Math.min(100, Math.round(100 * (summary.data.capacity.usedBytes + summary.data.capacity.reservedBytes) / summary.data.capacity.limitBytes)) : 0);
async function load() {
  const pristine = summary.data && ['enabled', 'probeEnabled', 'stillsEnabled', 'revision'].every(k => (form as any)[k] === (summary.data!.settings as any)[k]);
  await Promise.all([summary.load(), tasks.load()]);
  if (summary.data && (!form.revision || pristine && !op.pending))
    Object.assign(form, summary.data.settings);
  if (summary.data && form.revision && summary.data.settings.revision !== form.revision)
    latest.value = summary.data.settings;
  if (op.pending && !summary.error && !tasks.error) {
    let p: any;
    try {
      p = JSON.parse(op.pending.key);
    }
    catch {
      return;
    }
    if (p.action === 'settings') {
      if (summary.data.settings.revision !== p.revision) {
        latest.value = summary.data.settings;
        op.clear();
      }
    }
    else if (p.action === 'gc') {
      op.clear();
      notice.value = notice.value ? notice.value + ' 已重新读取当前缓存空间。' : '已重新读取当前缓存空间；继续清理需要再次确认。';
    }
    else if (p.id) {
      let proof;
      try {
        proof = await listProcessing(p.kind, '', 1, 200);
      }
      catch (e) {
        op.error = e;
        return;
      }
      const row = proof.items.find(r => r.id === p.id);
      if (row && (row.state !== p.state || row.attempts !== p.attempts || row.nextAttemptAt !== p.nextAttemptAt))
        op.clear();
    }
    else {
      const count = (p.kind === 'probe' ? summary.data.probe : summary.data.still).find(s => s.state === p.state)?.count;
      if (count === 0 || typeof count === 'number' && count < p.baseline)
        op.clear();
    }
  }
}
async function save() {
  const old = form.revision;
  const out = await op.run(JSON.stringify({ action: 'settings', revision: old }), () => saveProcessing({ ...form }));
  if (out !== undefined) {
    await load();
    if (summary.data?.settings.revision !== old) {
      form.revision = summary.data.settings.revision;
      latest.value = null;
      op.clear();
    }
  }
  else if (op.error) {
    const fresh = await summary.load();
    latest.value = fresh?.settings || null;
    notice.value = '保存未完成，输入已保留。请重新读取并核对修订。';
  }
}
function ask(task: any = null, gc = false) {
  action.value = gc ? { action: 'gc' } : { action: 'retry', kind: filter.kind, state: filter.state, id: task?.id || '', attempts: task?.attempts, nextAttemptAt: task?.nextAttemptAt, baseline: currentStates.value?.find(s => s.state === filter.state)?.count };
  confirm.value = true;
}
async function apply() {
  if (op.pending || op.busy)
    return;
  notice.value = '';
  const a = { ...action.value };
  const out = await op.run(JSON.stringify(a), () => a.action === 'gc' ? collectProcessing() : retryProcessing({ kind: a.kind, state: a.state, id: a.id, bulk: !a.id, confirmed: !a.id }));
  confirm.value = false;
  if (out !== undefined) {
    notice.value = a.action === 'gc' ? `服务回执：扫描 ${out.scanned} 项，回收 ${out.reclaimed} 个文件（${bytes(out.reclaimedBytes)}）。${out.partial ? '本轮部分完成。' : out.done ? '本轮扫描结束。' : '可核对空间后继续下一批。'}` : `服务回执：选中 ${out.selected} 条，已排队 ${out.queued} 条，跳过 ${out.skipped} 条，失败 ${out.failed} 条。${out.limitReached ? '已达 500 条上限。' : ''}`;
    await load();
    if (a.action === 'retry' && out.queued === 0 && !summary.error && !tasks.error)
      op.clear();
  }
}
function changeFilter() {
  if (filter.state && !validStates.value.includes(filter.state))
    filter.state = 'failed';
  filter.page = 1;
  void load();
}
function reason(code: string) {
  return code ? reasons[code] || '其他原因' : '—';
}
watch(() => [props.initialKind, props.initialState, props.jobId], () => {
  filter.kind = props.initialKind === 'still' ? 'still' : 'probe';
  filter.state = props.initialState && validStates.value.includes(props.initialState) ? props.initialState : props.jobId ? '' : 'failed';
  filter.page = 1;
  void load();
});
onMounted(load);
defineExpose({ load, save, ask, apply, form, filter, summary, tasks, op, action, reason, changeFilter });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <h2>预处理设置</h2>
      <p>提前读取媒体信息和生成截图，方便浏览；不会改变影片的播放方式。</p>
      <ResourceState overlay :stale="summary.loaded" :loading="summary.loading" :error="summary.error" @reload="load"/>
      <p>总开关与服务设置同步；关闭后，该服务的播放、探测和截图都会暂停。</p>
      <div class="u5-form">
        <p>播放服务：{{form.enabled?'已启用':'已停用'}} · <RouterLink to="/playback?tab=settings">管理播放服务开关</RouterLink></p>
        <label class="u5-check">
          <input v-model="form.probeEnabled" type="checkbox" :disabled="op.busy||!summary.loaded" aria-label="允许媒体探测"/>
          允许媒体探测
        </label>
        <label class="u5-check">
          <input v-model="form.stillsEnabled" type="checkbox" :disabled="op.busy||!summary.loaded" aria-label="允许单集截图"/>
          允许单集截图
        </label>

        <section v-if="latest">
          <p>当前设置已重新读取，输入保留供对照。</p>
          <el-button @click="form.revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
        </section>
      </div>
      <div class="mt-save-actions"><el-button :disabled="!summary.data||!!summary.error||!!op.pending||!!latest" :loading="op.busy" @click="save" type="primary" class="mt-save-button">保存预处理设置</el-button></div>
      <p>已用 {{ bytes(summary.data?.capacity?.usedBytes) }} · 预留 {{ bytes(summary.data?.capacity?.reservedBytes) }} · 上限 {{ bytes(summary.data?.capacity?.limitBytes) }} · 大小待核对 {{ summary.data?.capacity?.unknownSizes??'—' }} 项</p>
      <el-progress v-if="summary.data?.capacity?.limitBytes" :percentage="capacityPercent"/>
      <el-button :disabled="!!op.pending||op.busy||!!summary.error" @click="ask(null,true)">清理截图缓存</el-button>
    </section>
    <section class="media-surface">
      <h2>探测与截图任务</h2>
      <RouterLink v-if="jobId" :to="{path:'/ops',query:{tab:'jobs',q:jobId}}">查看对应后台任务记录</RouterLink>
      <form class="u5-filters" @submit.prevent="changeFilter">
        <label>
          任务类型
          <select v-model="filter.kind" aria-label="预处理任务类型" @change="changeFilter">
            <option value="probe">媒体探测</option>
            <option value="still">单集截图</option>
          </select>
        </label>
        <label>
          状态
          <select v-model="filter.state" aria-label="预处理状态筛选" @change="changeFilter">
            <option value="">全部状态</option>
            <option v-for="value in validStates" :key="value" :value="value">{{ states[value] }}</option>
          </select>
        </label>
        <el-button :disabled="!!op.pending||op.busy||summary.loading||tasks.loading||!!tasks.error" @click="ask()">批量重试当前筛选</el-button>
      </form>
      <div class="media-actions">
        <span v-for="state in currentStates||[]" :key="state.state">{{ states[state.state]||'其他' }} {{ state.count }}</span>
      </div>
      <ResourceState :stale="tasks.loaded" :loading="tasks.loading" :error="tasks.error" :empty="!tasks.data?.items.length" @reload="load"/>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <p v-if="notice" role="status">{{ notice }}</p>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'任务已排队，尚未完成。':'正在核对上次结果，不重复发起任务。'" @refresh="load"/>
      <article v-for="task in tasks.data?.items||[]" :key="task.id" class="organize-record">
        <h3>{{ task.name||'—' }}</h3>
        <p>{{ states[task.state]||'状态待核对' }} · {{ reason(task.errorReason) }}</p>
        <p>已尝试 {{ task.attempts??'—' }} 次 · 下次 {{ date(task.nextAttemptAt||undefined) }}</p>
        <el-button :disabled="!!op.pending||op.busy||tasks.loading||!!tasks.error" @click="ask(task)">重试此任务</el-button>
      </article>
      <PagePagination v-model:current-page="filter.page" v-model:page-size="filter.pageSize" :total="tasks.data.total" layout="prev,pager,next,total" @current-change="load"/>
    </section>
  </div>
  <ConfirmAction v-model:visible="confirm" :title="action?.action==='gc'?'清理截图缓存':action?.id?'重试预处理任务':'确认批量重试'" :description="action?.action==='gc'?'按服务端限制分批检查并清理截图缓存。':`将处理 ${action?.kind==='probe'?'媒体探测':'单集截图'} / ${states[action?.state]||'全部状态'}，${action?.id?'仅所选任务':'本轮最多 500 条'}。`" impact-scope="只影响预处理任务或截图缓存，不修改媒体文件，不改变播放链路。" confirm-text="确认执行" :loading="op.busy" class="media-dialog" @confirm="apply"/>
</template>
