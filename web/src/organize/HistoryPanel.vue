<script setup lang="ts">
import PathText from '../ui/PathText.vue';
import StatusBadge from '../components/StatusBadge.vue';
import {beijingISO} from '../presentation/time';
import {cachedTMDBImage} from '../api/images';
import {usePageNumber,usePageSize} from '../ui/page-query';

import TaskControl from '../workflows/TaskControl.vue';
import {usePageRefresh} from '../ui/page-refresh';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { post, request } from '../api/client';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import { kindLabels, stepState } from '../files/workbench';
import { date, errorText } from '../workflows/readiness';
import { sourceState } from '../media/model';
import { label, removable, rows } from './model';
const brokenPosters=ref(new Set<string>());
const route = useRoute(), items = ref<any[]>([]), total = ref(0), page = usePageNumber(), pageSize = usePageSize(25), kind = ref(String(route.query.kind || '')), state = ref(''), query = ref(String(route.query.q || '')), batchId = ref(String(route.query.batchId || '')), origin = ref(''), method = ref(''), from = ref(''), to = ref(''), loading = ref(true), error = ref<unknown>(null), detail = ref<any>(null), open = ref(false), selected = ref<any[]>([]), removeOpen = ref(false), busy = ref(false), uncertain = ref(false);
let epoch = 0, disposed = false;
const summary = computed(() => ({ active: items.value.filter(r => ['pending', 'running', 'paused'].includes(r.state)).length, attention: items.value.filter(r => ['needs_reconcile', 'conflict', 'failed', 'partial'].includes(r.state)).length, done: items.value.filter(r => ['succeeded', 'imported', 'visible', 'applied', 'verified'].includes(r.state)).length }));
let hasLoaded=false;
  async function load() {
  const version = ++epoch;
  loading.value = !hasLoaded;
  error.value = null;
  try {
    const p = new URLSearchParams({ page: String(page.value), pageSize: String(pageSize.value), kind: kind.value, state: state.value, q: query.value, batchId: batchId.value, origin: origin.value, method: method.value });
    if (from.value)
      p.set('from', beijingISO(from.value));
    if (to.value)
      p.set('to', beijingISO(to.value));
    const out: any = await request(`/library/activity?${p}`);
    if (version === epoch && !disposed) {
      items.value = rows(out.items);
      total.value = out.total;
      selected.value = [];
      uncertain.value = false;
    }
  hasLoaded=true;
 }
  catch (e) {
    if (version === epoch && !disposed)
      error.value = e;
  }
  finally {
    if (version === epoch && !disposed)
      loading.value = false;
  }
}
function apply() {
  page.value = 1;
  void load();
}
function toggle(row: any, checked: boolean) {
  selected.value = selected.value.filter(r => r.id !== row.id || r.kind !== row.kind);
  if (checked)
    selected.value.push(row);
}
async function remove() {
  if (busy.value || uncertain.value || !selected.value.length)
    return;
  busy.value = true;
  try {
    await post('/library/activity/remove', { confirm: true, items: selected.value.map(r => ({ id: r.id, kind: r.kind, updatedAt: r.updatedAt })) });
    removeOpen.value = false;
    await load();
  }
  catch (e) {
    error.value = e;
    uncertain.value = true;
    removeOpen.value = false;
  }
  finally {
    busy.value = false;
  }
}
async function reconcile() {
  if (!detail.value || busy.value)
    return;
  busy.value = true;
  try {
    if (detail.value.kind === 'batch')
      await post(`/files/batches/${detail.value.id}/reconcile`, {});
    else if (detail.value.kind === 'file')
      await post(`/files/operations/${detail.value.id}/reconcile`, {});
    open.value = false;
    await load();
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
watch(() => [route.query.kind, route.query.batchId, route.query.q], () => {
  kind.value = String(route.query.kind || '');
  batchId.value = String(route.query.batchId || '');
  query.value = String(route.query.q || '');
  apply();
});
onMounted(load);
onUnmounted(() => {
  disposed = true;
  epoch++;
});
usePageRefresh(load, 'history');
</script>
<template>
  <section class="media-surface organize-history-panel">
    <header class="media-section-heading">
      <div>
        <h2>文件与入库历史</h2>
        <p>全部 {{ loading||error?'—':total }} 项 · 本页进行中 {{ loading||error?'—':summary.active }} · 需处理 {{ loading||error?'—':summary.attention }} · 已完成阶段 {{ loading||error?'—':summary.done }}</p>
      </div>

    </header>
    <nav class="media-actions mt-segments" aria-label="历史阶段快捷筛选">
      <el-button v-for="(text,value) in Object.fromEntries(Object.entries(kindLabels).filter(([key])=>key!=='cleanup'))" :key="value" :aria-pressed="kind===value" @click="kind=value;apply()">{{ text }}</el-button>
    </nav>
    <form class="media-form organize-history-filters" @submit.prevent="apply">
      <label>
        来源
        <el-select v-model="origin" aria-label="历史来源" @change="apply">
          <el-option value="" label="全部来源"/>
          <el-option value="download" label="下载源"/>
          <el-option value="file" label="文件管理"/>
          <el-option value="system" label="系统任务"/>
        </el-select>
      </label>
      <label>
        方式
        <el-select v-model="method" aria-label="历史方式" @change="apply">
          <el-option value="" label="全部方式"/>
          <el-option v-for="(text,value) in kindLabels" :key="value" :value="value" :label="text"/>
        </el-select>
      </label>
      <label>
        阶段
        <el-select v-model="kind" aria-label="历史阶段" @change="apply">
          <el-option value="" label="全部阶段"/>
          <el-option v-for="(text,value) in Object.fromEntries(Object.entries(kindLabels).filter(([key])=>key!=='cleanup'))" :key="value" :value="value" :label="text"/>
        </el-select>
      </label>
      <label>
        状态
        <el-select v-model="state" aria-label="历史状态" @change="apply">
          <el-option value="" label="全部状态"/>
          <el-option v-for="v in ['pending','running','paused','needs_reconcile','failed','partial','succeeded','imported','applied','verified','visible','cancelled']" :key="v" :value="v" :label="label(v)"/>
        </el-select>
      </label>
      <label>
        关键词
        <el-input v-model="query" aria-label="历史关键词"/>
      </label>
      <label>
        批次
        <el-input v-model="batchId" aria-label="历史批次ID"/>
      </label>
      <label>
        开始时间
        <input v-model="from" type="datetime-local" aria-label="历史开始时间"/>
      </label>
      <label>
        结束时间
        <input v-model="to" type="datetime-local" aria-label="历史结束时间"/>
      </label>
      <el-button native-type="submit">筛选</el-button>
    </form>
    <p v-if="error&&hasLoaded" class="f3-stale-warning" role="status">刷新失败，保留上次内容。<button @click="load">重试</button></p>
    <StateView v-if="loading" state="loading" title="正在读取历史…"/>
    <StateView v-else-if="error&&!hasLoaded" :state="sourceState(error)" title="历史读取或核对失败" :description="errorText(error)" @retry="load"/>
    <p v-if="uncertain">移除回执未知，请先重新读取，不重复提交。</p>
    <ol class="organize-history-timeline" aria-label="整理处理时间线">
    <li v-for="item in items" :key="item.kind+item.id" class="organize-record f5-history-row">
      <label v-if="removable(item)" class="media-consent f5-history-check"><input type="checkbox" aria-label="选择此历史" :checked="selected.some(r=>r.id===item.id&&r.kind===item.kind)" :disabled="uncertain" @change="toggle(item,($event.target as HTMLInputElement).checked)"/></label>
      <span v-else class="f5-history-check"/>
      <span class="f5-history-cover"><img v-if="cachedTMDBImage(item.poster||item.media?.posterPath)&&!brokenPosters.has(item.id)" :src="cachedTMDBImage(item.poster||item.media?.posterPath)" alt="作品海报" width="44" height="66" loading="lazy" @error="brokenPosters.add(item.id)"/><span v-else aria-label="暂无海报">暂无海报</span></span>
      <div class="f5-row-summary">
        <h3>{{kindLabels[item.kind]||'业务记录'}}</h3>
        <div class="history-paths"><div><small>源文件</small><PathText :path="item.source"/></div><span aria-hidden="true">→</span><div><small>目标路径</small><PathText :path="item.destination"/></div></div>
        <p>识别结果：{{item.media?.title?item.media.title+(item.media.year?' ('+item.media.year+')':''):'暂无识别结果'}}<span v-if="item.category"> · {{item.category}}</span></p>
        <div class="history-record-meta"><StatusBadge :state="item.state" :label="label(item.state)"/><time>{{date(item.updatedAt||item.createdAt)}}</time></div>
      </div>
      <div class="media-actions"><el-button @click="detail=item;open=true">查看步骤详情</el-button><TaskControl v-if="item.kind==='batch'" kind="organize" :id="item.batchId||item.id" :state="item.state" @changed="load"/></div>
    </li>
    </ol>
    <StateView v-if="!loading&&!error&&!items.length" state="empty" title="暂无符合条件的历史"/>
    <PagePagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" layout="prev,pager,next,total" @current-change="load"/>
    <div class="media-actions history-danger-actions"><el-button :disabled="!selected.length||busy||uncertain" @click="removeOpen=true">移除所选历史</el-button></div>
  </section>
  <DetailDrawer :key="detail?.id" v-model:visible="open" title="整理步骤详情" :technical-data="detail" class="media-detail">
    <h3>{{ detail?.media?.title||kindLabels[detail?.kind]||'业务记录' }}</h3>
    <p>{{ label(detail?.state) }} · {{ detail?.message||'—' }}</p>
    <p>原位置：{{detail?.source||'—'}}</p><p>目标位置：{{detail?.destination||'—'}}</p><p>目录分类：{{detail?.category||'—'}}</p>
    <article v-for="(step,index) in detail?.details?.plan?.steps||[]" :key="index">
      <strong>{{ step.action==='mkdir'?'创建目录':'移动或改名' }} · {{ label(stepState(detail.state,detail.details.completedSteps||0,Number(index))) }}</strong>
      <p>{{ step.source?.path||'—' }} → {{ step.target||'—' }}</p>
    </article>
    <article v-for="part in detail?.details?.items||[]" :key="part.ordinal">
      <p>{{ part.source }} · {{ label(part.state) }} · {{ part.message||'—' }}</p>
    </article>
    <section v-if="detail?.kind==='transfer'">
      <p>{{ detail.details.bytesTransferred??'—' }} / {{ detail.details.size??'—' }} 字节已传输；上传完成不等于云端校验或输出完成。</p>
      <RouterLink :to="{path:'/downloads',query:{tab:'uploads'}}" @click="open=false">查看上传入库与批准</RouterLink>
    </section>
    <RouterLink v-if="detail?.kind==='strmRepair'" :to="{path:'/files',query:{tab:'tools',repairId:detail.id}}" @click="open=false">查看 STRM 修复批次</RouterLink>
    <RouterLink v-if="detail?.kind==='output'" :to="{path:'/organize',query:{tab:'recovery'}}" @click="open=false">进入输出恢复与归档</RouterLink>
    <RouterLink v-if="detail?.kind==='scan'" :to="{path:'/organize',query:{tab:'scans'}}" @click="open=false">进入扫描控制</RouterLink>
    <p v-if="detail?.details?.phase">阶段：{{ label(detail.details.phase) }}</p>
    <el-button v-if="['batch','file'].includes(detail?.kind)&&['running','needs_reconcile'].includes(detail?.state)" :loading="busy" @click="reconcile">核对结果（不重放）</el-button>
  </DetailDrawer>
  <ConfirmAction v-model:visible="removeOpen" title="移除整理历史" description="仅从历史列表移除选中项，进行中或未知结果不能移除。" impact-scope="不删除媒体、STRM、审计或恢复证据。" confirm-text="明确移除历史" :loading="busy" class="media-dialog" @confirm="remove"/>
</template>
