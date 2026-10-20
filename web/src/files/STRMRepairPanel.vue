<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { APIError, post, request } from '../api/client';
import PreviewApprove from '../ui/PreviewApprove.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import { errorText, date } from '../workflows/readiness';
import { sourceState } from '../media/model';
import { label, rows, readPrivate, savePrivate } from '../organize/model';
const route = useRoute(), router = useRouter(), assets = ref<any[]>([]), page = ref(1), total = ref(0), query = ref(''), limit = ref(100), selected = ref<string[]>([]), pickerOpen = ref(false), loading = ref(false), loadError = ref<unknown>(null), batch = ref<any>(null), busy = ref(false), error = ref<unknown>(null), pending = ref<any>(readPrivate('media-tool:strm-repair:v1')), phase = ref<'queued' | 'reconciling' | null>(pending.value ? 'reconciling' : null), confirmOpen = ref(false), retry = ref(false), readVerified = ref(false), technical = ref(false);
let epoch = 0, loadEpoch = 0, disposed = false;
const repairLabel = (s: string) => ({ preview: '等待核对', running: '执行中', succeeded: '修复完成', partial: '部分完成，含跳过项', ready: '原文完整', missing: '待恢复缺失文件', staged: '已登记恢复意图', repaired: '已修复', needs_reconcile: '需核对后继续', blocked: '跳过，需核对', expired: '预览已过期' } as Record<string, string>)[s] || label(s);
const allSelected = computed(() => assets.value.length > 0 && assets.value.every(a => selected.value.includes(a.id)));
function toggle(id: string, checked: boolean) {
  selected.value = selected.value.filter(v => v !== id);
  if (checked && selected.value.length < limit.value)
    selected.value.push(id);
}
function selectPage() {
  if (allSelected.value)
    selected.value = selected.value.filter(id => !assets.value.some(a => a.id === id));
  else
    for (const a of assets.value)
      toggle(a.id, true);
}
watch(limit, () => {
  limit.value = Math.min(100, Math.max(1, Number(limit.value) || 1));
  selected.value = selected.value.slice(0, limit.value);
});
async function loadAssets() {
  const current = ++loadEpoch;
  loading.value = true;
  loadError.value = null;
  try {
    const out: any = await request(`/library/outputs?kind=strm&page=${page.value}&pageSize=100&q=${encodeURIComponent(query.value)}`);
    if (current === loadEpoch && !disposed) {
      assets.value = rows(out.items);
      total.value = out.total;
    }
  }
  catch (e) {
    if (current === loadEpoch)
      loadError.value = e;
  }
  finally {
    if (current === loadEpoch)
      loading.value = false;
  }
}
function choose() {
  pickerOpen.value = true;
  void loadAssets();
}
async function preview() {
  if (busy.value || pending.value || !selected.value.length)
    return;
  const current = ++epoch;
  busy.value = true;
  error.value = null;
  readVerified.value = false;
  try {
    const out: any = await post('/strm/repairs/preview', { assetIds: [...selected.value] });
    if (current !== epoch || disposed)
      return;
    batch.value = out;
    phase.value = null;
    await router.replace({ query: { ...route.query, repairId: out.id } });
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function readback(id = String(route.query.repairId || batch.value?.id || pending.value?.id || '')) {
  if (!id)
    return;
  const current = ++epoch;
  busy.value = true;
  error.value = null;
  readVerified.value = false;
  try {
    const out: any = await request(`/strm/repairs/${id}`);
    if (current !== epoch || disposed)
      return;
    batch.value = out;
    readVerified.value = true;
    if (pending.value?.id === id && (['succeeded', 'partial', 'expired'].includes(out.state) || pending.value.rejected && out.state === 'preview')) {
      pending.value = null;
      phase.value = null;
      savePrivate('media-tool:strm-repair:v1', null);
    }
    else
      phase.value = pending.value ? 'reconciling' : null;
  }
  catch (e) {
    if (current === epoch)
      error.value = e;
  }
  finally {
    if (current === epoch)
      busy.value = false;
  }
}
function ask(continueExisting = false) {
  retry.value = continueExisting;
  confirmOpen.value = true;
}
async function apply() {
  if (!batch.value || busy.value || !retry.value && pending.value || retry.value && (!readVerified.value || batch.value.state !== 'needs_reconcile'))
    return;
  const id = batch.value.id;
  busy.value = true;
  error.value = null;
  readVerified.value = false;
  pending.value = { id };
  phase.value = 'reconciling';
  savePrivate('media-tool:strm-repair:v1', { id });
  try {
    batch.value = retry.value ? await post(`/strm/repairs/${id}/retry`, { digest: batch.value.digest, confirm: true }) : await post(`/strm/repairs/${id}/approve`, { digest: batch.value.digest, confirm: true });
    phase.value = 'queued';
    confirmOpen.value = false;
  }
  catch (e) {
    error.value = e;
    confirmOpen.value = false;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      pending.value = { id, rejected: true };
      savePrivate('media-tool:strm-repair:v1', pending.value);
    }
    if (e instanceof APIError && e.status === 409)
      error.value = Error('修复计划或文件已变化，请先读取现有批次再核对，不重复批准。');
  }
  finally {
    busy.value = false;
  }
}
watch(() => route.query.repairId, id => {
  if (id && id !== batch.value?.id)
    void readback(String(id));
});
onMounted(() => {
  if (pending.value?.id)
    void readback(pending.value.id);
  else if (route.query.repairId)
    void readback(String(route.query.repairId));
});
onUnmounted(() => {
  disposed = true;
  epoch++;
  loadEpoch++;
});
defineExpose({ readback });
</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <div>
        <h2>STRM 批量修复</h2>
        <p>只恢复可证明原文的缺失 STRM，不覆盖手工修改，不旋转播放凭据，不处理 NFO。</p>
      </div>
      <el-button :disabled="!!pending||busy" @click="choose">选择受管资产</el-button>
    </header>
    <PreviewApprove :current-step="!batch?1:readVerified&&['succeeded','partial'].includes(batch.state)?4:phase?3:2">
      <StateView v-if="error" :state="sourceState(error)" title="修复核对未完成" :description="errorText(error)" :retry-text="batch?.id||pending?.id?'核对现有批次':'重新读取资产'" @retry="batch?.id||pending?.id?readback(pending?.id||batch?.id):loadAssets()"/>
      <p v-if="!batch&&!busy">先选资产并生成检查结果；预览阶段不会修复任何文件。</p>
      <section v-if="pickerOpen" class="files-repair-picker">
        <h3>选择需要检查的资产</h3>
        <form class="files-browser-toolbar" @submit.prevent="page=1;loadAssets()">
          <label>
            按标题或路径筛选
            <el-input v-model="query" aria-label="修复资产筛选"/>
          </label>
          <label>
            本批选择上限
            <el-input-number v-model="limit" aria-label="修复选择上限" :min="1" :max="100"/>
          </label>
          <el-button native-type="submit">筛选资产</el-button>
        </form>
        <div class="media-actions">
          <el-button :disabled="loading||!!pending" @click="selectPage">{{ allSelected?'清除当前页选择':'按上限选择当前页' }}</el-button>
          <el-button @click="selected=[]">清空全部选择</el-button>
          <span>已选 {{ selected.length }} / {{ limit }}</span>
        </div>
        <StateView v-if="loading" state="loading" title="正在读取可选资产…"/>
        <StateView v-else-if="loadError" :state="sourceState(loadError)" title="资产读取失败" :description="errorText(loadError)" @retry="loadAssets"/>
        <StateView v-else-if="!assets.length" state="empty" title="暂无匹配的受管资产"/>
        <label v-for="asset in assets" :key="asset.id" class="files-repair-asset">
          <input type="checkbox" :disabled="loading||!!pending||!selected.includes(asset.id)&&selected.length>=limit" :checked="selected.includes(asset.id)" @change="toggle(asset.id,($event.target as HTMLInputElement).checked)"/>
          <span>
            <strong>{{ asset.title||'—' }} {{ asset.year||'' }}</strong>
            <small>{{ asset.source||'—' }}</small>
            <small v-for="out in asset.outputs?.filter((o:any)=>o.kind==='strm')||[]" :key="out.path">{{ out.path }}</small>
          </span>
        </label>
        <el-pagination v-if="total>100" v-model:current-page="page" :page-size="100" :total="total" layout="prev,pager,next,total" @current-change="loadAssets"/>
        <el-button :disabled="!selected.length||!!pending||!!loadError" :loading="busy" @click="preview">检查已选 {{ selected.length }} 个资产</el-button>
      </section>
      <section v-if="batch" aria-label="修复批次">
        <h3>{{ phase&&!readVerified?(phase==='queued'?'已排队，待核验':'正在核对上次结果'):repairLabel(batch.state) }}</h3>
        <p>预览有效至 {{ date(batch.expiresAt) }} · {{ batch.items?.length??'—' }} 项</p>
        <article v-for="row in batch.items" :key="row.ordinal" class="organize-record">
          <h3>{{ row.path||'所选资产的受管输出' }}</h3>
          <p>{{ phase&&!readVerified?'等待核验':repairLabel(row.state) }} · {{ !phase||readVerified?row.message||'—':'以重新读取的记录为准' }}</p>
        </article>
        <AsyncResultBanner v-if="phase" :status="phase" :message="phase==='queued'?'修复已排队，尚未核验完成。':'正在核对上次修复结果，不重复批准。'" @refresh="readback(pending?.id)"/>
        <div class="media-actions">
          <el-button v-if="batch.state==='preview'&&!pending" :disabled="busy||!batch.items.some((r:any)=>r.state==='missing')||Date.parse(batch.expiresAt)<=Date.now()" @click="ask()">批准批量修复</el-button>
          <el-button :loading="busy" @click="readback()">刷新批次状态</el-button>
          <el-button v-if="batch.state==='needs_reconcile'" :disabled="!readVerified||busy||pending&&pending.id!==batch.id" @click="ask(true)">核对后恢复此批次</el-button>
          <el-button @click="technical=true">技术详情</el-button>
        </div>
      </section>
      <el-button v-if="pending&&!batch" @click="readback(pending.id)">核对已有修复请求</el-button>
    </PreviewApprove>
  </section>
  <ConfirmAction v-model:visible="confirmOpen" :title="retry?'恢复 STRM 修复任务':'批准 STRM 修复'" :description="retry?'沿用已有批准与原始修复意图，需已读取当前批次状态。':'仅恢复本次预览中可证明原文的缺失文件。'" impact-scope="不覆盖手工文件、不重新上传源媒体、不生成新播放凭据、不处理 NFO。" :confirm-text="retry?'明确恢复任务':'明确批准修复'" :loading="busy" class="media-dialog" @confirm="apply"/>
  <DetailDrawer v-model:visible="technical" title="修复技术详情" :technical-data="batch" class="media-detail">
    <p>用于核对当前批次与原始修复意图。</p>
  </DetailDrawer>
</template>
