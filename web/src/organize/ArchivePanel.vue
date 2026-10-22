<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';

import TaskControl from '../workflows/TaskControl.vue';
import {usePageRefresh} from '../ui/page-refresh';
import { ref, onMounted, onUnmounted } from 'vue';
import { APIError, post, request } from '../api/client';
import { requestUUID } from '../utils/request-uuid';
import { errorText, date } from '../workflows/readiness';
import ConfirmAction from '../ui/ConfirmAction.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { sourceState } from '../media/model';
import { eligible, label, rows, readPrivate, savePrivate } from './model';
const stored = readPrivate('media-tool:archive-pending:v1'), items = ref<any[]>([]), page = usePageNumber(), total = ref(0), policy = ref<any>(null), days = ref(30), selected = ref<any[]>([]), loading = ref(true), busy = ref(false), error = ref<unknown>(null), conflict = ref<any>(null), plan = ref<any>(stored?.plan || null), requestId = ref(stored?.requestId || ''), phase = ref<'queued' | 'reconciling' | null>(stored ? 'reconciling' : null), confirmOpen = ref(false), retryOpen = ref(false), retryTarget = ref<any>(null), readbackMissing = ref(false), results = ref<any[]>([]), detailsOpen = ref(false);
const policyPending = ref(false), retryPending = ref(readPrivate('media-tool:archive-retry:v1'));
let disposed = false, loadVersion = 0, previewVersion = 0;
async function load() {
  const currentVersion = ++loadVersion;
  loading.value = true;
  error.value = null;
  try {
    const [out, current]: any = await Promise.all([request(`/library/archives?page=${page.value}&pageSize=50`), request('/library/archive-policy')]);
    if (!disposed && currentVersion === loadVersion) {
      items.value = rows(out.items);
      total.value = out.total;
      if (!policy.value)
        days.value = current.retentionDays;
      if (policyPending.value)
        conflict.value = current;
      policy.value = current;
      policyPending.value = false;
      retryPending.value = null;
      savePrivate('media-tool:archive-retry:v1', null);
    }
  }
  catch (e) {
    error.value = e;
  }
  finally {
    if (!disposed)
      loading.value = false;
  }
}
async function savePolicy() {
  if (!policy.value || busy.value || phase.value || conflict.value)
    return;
  busy.value = true;
  try {
    policy.value = await post('/library/archive-policy', { retentionDays: days.value, revision: policy.value.revision, automaticCleanup: false });
    await load();
  }
  catch (e) {
    error.value = e;
    if (!(e instanceof APIError) || e.status >= 500 || e.status === 409) {
      policyPending.value = true;
      conflict.value = { retentionDays: '尚未读取', revision: null };
      try {
        conflict.value = await request('/library/archive-policy');
        policyPending.value = false;
        error.value = Error('配置已被修改，已重新读取。保留天数输入已保留，请核对后采用最新修订。');
      }
      catch {
        error.value = Error('配置冲突后重新读取失败，输入已保留。');
      }
    }
  }
  finally {
    busy.value = false;
  }
}
function toggle(row: any, checked: boolean) {
  if (phase.value || busy.value)
    return;
  selected.value = selected.value.filter(r => r.id !== row.id);
  if (checked && eligible(row) && selected.value.length < 20)
    selected.value.push(row);
  plan.value = null;
  requestId.value = '';
}
async function preview() {
  if (busy.value || phase.value || !selected.value.length)
    return;
  const current = ++previewVersion;
  busy.value = true;
  error.value = null;
  try {
    const out: any = await post('/library/archives/cleanup-preview', { archiveIds: selected.value.map(r => r.id) });
    if (disposed || current !== previewVersion)
      return;
    if (!out.sourceFilesUntouched || !out.physicalDeletion)
      throw Error('归档范围保障不完整，不能批准清理。');
    plan.value = out;
    requestId.value = requestUUID();
    results.value = [];
    readbackMissing.value = false;
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function apply() {
  if (!plan.value || busy.value || phase.value && !readbackMissing.value)
    return;
  const body = { requestId: requestId.value, archiveIds: plan.value.items.map((r: any) => r.id), planDigest: plan.value.planDigest };
  busy.value = true;
  phase.value = 'reconciling';
  readbackMissing.value = false;
  savePrivate('media-tool:archive-pending:v1', { requestId: body.requestId, plan: plan.value });
  try {
    const out: any = await post('/library/archives/cleanup-confirm', body);
    results.value = rows(out);
    confirmOpen.value = false;
    if (results.value.length !== body.archiveIds.length)
      throw Error('清理结果不完整，请核对已有请求结果；不换请求重复提交。');
    phase.value = 'queued';
    await load();
  }
  catch (e) {
    error.value = e;
    confirmOpen.value = false;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      try {
        const receipt: any = await request(`/library/archive-cleanups/${body.requestId}`);
        if (!Array.isArray(receipt))
          throw Error('清理记录格式不完整');
        results.value = receipt;
        if (!results.value.length) {
          phase.value = null;
          plan.value = null;
          requestId.value = '';
          savePrivate('media-tool:archive-pending:v1', null);
          error.value = Error('批准已被拒绝，核对未发现已批准记录。请刷新归档并重新预览后再确认。');
        }
      }
      catch {
      }
    }
  }
  finally {
    busy.value = false;
  }
}
async function reconcile() {
  if (!requestId.value || busy.value)
    return;
  busy.value = true;
  try {
    results.value = rows(await request(`/library/archive-cleanups/${requestId.value}`));
    readbackMissing.value = !results.value.length;
    if (readbackMissing.value)
      error.value = Error('尚未确认批准记录，只能保留同一请求和计划再次明确确认。');
    else {
      phase.value = 'queued';
      await load();
    }
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function retry() {
  if (!retryTarget.value?.cleanupId || busy.value || retryPending.value)
    return;
  busy.value = true;
  retryPending.value = retryTarget.value.cleanupId;
  savePrivate('media-tool:archive-retry:v1', retryPending.value);
  try {
    await post(`/library/archive-cleanups/${retryTarget.value.cleanupId}/retry`, {});
    retryOpen.value = false;
    await load();
  }
  catch (e) {
    error.value = e;
    retryOpen.value = false;
  }
  finally {
    busy.value = false;
  }
}
function finish() {
  if (results.value.length && results.value.every(r => ['removed', 'conflict'].includes(r.state))) {
    phase.value = null;
    plan.value = null;
    requestId.value = '';
    selected.value = [];
    savePrivate('media-tool:archive-pending:v1', null);
  }
}
onMounted(load);
onUnmounted(() => {
  disposed = true;
});
usePageRefresh(load, 'recovery');
</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>旧输出归档</h2>

    </header>
    <p>只管理完成迁移后生成的过期托管归档；不自动清理，不操作原媒体、做种源、当前 STRM 或共享 NFO。</p>
    <div class="media-actions">
      <label>
        归档保留天数
        <el-input-number v-model="days" aria-label="归档保留天数" :min="1" :max="3650" :disabled="!!phase"/>
      </label>

      <el-button :disabled="!selected.length||busy||!!phase" @click="preview">预览清理所选 {{ selected.length }}</el-button>
    </div>
    <div class="mt-save-actions"><el-button :disabled="!policy||busy||!!phase||!!conflict" @click="savePolicy" type="primary" class="mt-save-button">保存保留期</el-button></div>
    <p v-if="retryPending" role="status">上次继续清理的结果未知，请先刷新归档核对状态。</p>
    <section v-if="conflict">
      <p>服务端当前保留期 {{ conflict.retentionDays }} 天。你的输入 {{ days }} 天。</p>
      <el-button :disabled="conflict.revision==null" @click="policy=conflict;conflict=null">保留输入并采用最新修订</el-button>
    </section>
    <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
    <StateView v-if="loading" state="loading" title="正在读取归档与策略…"/>
    <StateView v-else-if="!items.length&&!error" state="empty" title="暂无旧输出归档"/>
    <article v-for="item in items" :key="item.id" class="organize-record">
      <label class="media-consent">
        <input type="checkbox" :disabled="!eligible(item)||!!phase||busy" :checked="selected.some(r=>r.id===item.id)" @change="toggle(item,($event.target as HTMLInputElement).checked)"/>
        {{ item.originalPath }}
      </label>
      <p>{{ item.archivePath }}</p>
      <p>{{ label(item.state) }} · 保留至 {{ date(item.eligibleAfter) }}</p>
      <p>{{ item.message||'—' }}</p>
      <TaskControl v-if="item.cleanupId&&item.state!=='removed'" kind="archives" :id="item.cleanupId" state="unknown" @changed="load"/>
    </article>
    <PagePagination v-model:current-page="page" :page-size="50" :total="total" layout="prev,pager,next,total" @current-change="load"/>
    <section v-if="plan" class="organize-plan" aria-label="归档清理计划">
      <h3>预览归档清理</h3>
      <p>将物理删除下面选中的过期归档，不操作原媒体或当前输出。</p>
      <p v-for="item in plan.items" :key="item.id">{{ item.originalPath }} → {{ item.archivePath }}</p>
      <el-button :disabled="busy||!!phase&&!readbackMissing" type="danger" @click="confirmOpen=true">{{ readbackMissing?'使用原请求继续确认':'确认物理清理' }}</el-button>
      <el-button @click="detailsOpen=true">技术详情</el-button>
    </section>
    <AsyncResultBanner v-if="phase" :status="phase" message="归档清理结果需核对，未确认前不更换请求或计划。" @refresh="reconcile"/>
    <div v-if="phase" class="media-actions">
      <el-button @click="reconcile">核对已有请求结果</el-button>
      <el-button :disabled="!results.length||!results.every(r=>['removed','conflict'].includes(r.state))" @click="finish">已核对结果，结束本次操作</el-button>
    </div>
    <p v-for="result in results" :key="result.id">{{ label(result.state) }} · {{ result.message||'—' }}</p>
  </section>
  <ConfirmAction v-model:visible="confirmOpen" title="确认物理清理归档" description="物理删除已预览的过期托管归档，不能自动撤销。" impact-scope="不删除原媒体、做种源、当前输出或共享NFO。确认时重新检查归属、内容和保留期。" confirm-phrase="清理归档" confirm-text="确认物理清理" cancel-text="返回检查" danger-level="danger" :loading="busy" class="media-dialog" @confirm="apply"/>
  <ConfirmAction v-model:visible="retryOpen" title="继续已批准的归档清理" description="只继续此项已有批准，仍检查文件、归属、保留期和恢复状态。" impact-scope="不会重建清理计划，不覆盖或删除手工修改内容。" confirm-text="明确继续" :loading="busy" class="media-dialog" @confirm="retry"/>
  <DetailDrawer v-model:visible="detailsOpen" title="归档清理技术详情" :technical-data="{requestId,plan,results}" class="media-detail">
    <p>请求与计划仅用于核对本次已批准的归档，不得公开分享。</p>
  </DetailDrawer>
</template>
