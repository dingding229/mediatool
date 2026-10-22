<script setup lang="ts">
import {Refresh,Delete,Setting,CopyDocument,Download,DArrowLeft,ArrowLeft,ArrowRight,DArrowRight} from '@element-plus/icons-vue';
import { pageResult } from './model';
import { formatBytes as bytes } from './model';
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { logText, formatLogTime, logSource } from '../logs/format';
import { friendlyMessage, moduleLabel, levelLabel, logAdvice } from '../logs/friendly';
import { copyOriginal } from './model';
import { errorText } from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
const route = useRoute(), filter = reactive({ page: 1, pageSize: 200, level: '', module: String(route.query.module || ''), instance: String(route.query.instance || ''), keyword: '', before: '' }), from = ref(''), to = ref(''), clean = ref(false), friendly = ref(true), detail = ref<any>(null), open = ref(false), settings = ref(false), confirm = ref(false), copied = ref(''), copyContent = ref(''), copyOpen = ref(false), logLevel = ref(''), op = reactive(useOperation('logs')), data = reactive(useResource<any>(() => {
  const q = new URLSearchParams(Object.fromEntries(Object.entries(filter).map(([k, v]) => [k, String(v)])));
  if (!filter.instance)
    q.delete('instance');
  if (!filter.before)
    q.delete('before');
  if (from.value)
    q.set('from', new Date(from.value).toISOString());
  if (to.value)
    q.set('to', new Date(to.value).toISOString());
  return request('/logs/query?' + q).then(pageResult);
})), summary = reactive(useResource<any>(() => request('/logs/summary')));
async function load() {
  await Promise.all([data.load(), summary.load()]);
  if (data.data && !data.error)
    filter.before = data.data.before || '';
  if (op.pending && !summary.error && summary.data) {
    const p = JSON.parse(op.pending.key);
    if (p.kind === 'level' && summary.data.level === p.level || p.kind === 'clear' && op.pending.status === 200 && !data.error)
      op.clear();
  }
}
function refresh() {
  filter.before = '';
  filter.page = 1;
  void load();
}
async function clear() {
  await op.run(JSON.stringify({ kind: 'clear' }), () => post('/logs/clear', { confirm: true }));
  confirm.value = false;
  filter.before = '';
  await load();
}
async function change() {
  await op.run(JSON.stringify({ kind: 'level', level: logLevel.value }), () => post('/logs/level', { level: logLevel.value }));
  await load();
}
async function copy(value: string) {
  copied.value = await copyOriginal(value, s => {
    copyContent.value = s;
    copyOpen.value = true;
  });
}
function download() {
  const blob = new Blob([logText(data.data?.items || [], false)], { type: 'text/plain;charset=utf-8' }), url = URL.createObjectURL(blob), link = document.createElement('a');
  link.href = url;
  link.download = 'mediatool-logs-' + new Date().toISOString().slice(0, 10) + '.txt';
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
watch(() => [route.query.module, route.query.instance], ([m, i]) => {
  filter.module = typeof m === 'string' ? m : '';
  filter.instance = typeof i === 'string' ? i : '';
  refresh();
});
let timer: ReturnType<typeof setTimeout>;
watch(() => filter.keyword, () => {
  clearTimeout(timer);
  timer = setTimeout(refresh, 350);
});
onMounted(load);
onUnmounted(() => clearTimeout(timer));
const pageCount=computed(()=>Math.max(1,Math.ceil((data.data?.total||0)/filter.pageSize)));
function fields(row:any) { return Object.entries(row).filter(([key])=>!['time','level','msg','source','module','traceId','trace_id','trace','spanId','span_id','span'].includes(key)).map(([key,value])=>key+'='+(typeof value==='string'?value:JSON.stringify(value))).join('  '); }
function trace(row:any) { return [row.traceId||row.trace_id||row.trace, row.spanId||row.span_id||row.span].filter(Boolean).join(' / '); }
defineExpose({ load, refresh, filter, data, summary, from, to, clear, change, op, copy });
</script>
<template>
  <section class="media-surface f3-log-console" aria-label="系统日志控制台">
    <header class="media-section-heading">
      <h2>程序日志</h2>
      <div class="media-actions f3-log-tools">
        <label><input v-model="clean" type="checkbox" aria-label="清爽模式"/> 清爽模式</label>
        <el-button :icon="Refresh" aria-label="刷新日志" title="刷新日志" @click="load"/>
        <el-button :icon="Delete" :disabled="!!op.pending" aria-label="清理日志" title="清理日志" @click="confirm=true"/>
        <el-button :icon="Setting" aria-label="日志设置" title="日志设置" @click="settings=true;logLevel=summary.data?.level||'info'"/>
        <el-button :icon="CopyDocument" aria-label="复制当前页日志" title="复制当前页日志" :disabled="!data.data?.items.length" @click="copy(logText(data.data.items,false))"/>
        <el-button :icon="Download" aria-label="下载原始日志" title="下载原始日志" :disabled="!data.data?.items.length" @click="download"/>
      </div>
    </header>
    <p class="f3-log-privacy">仅管理员可读：保留原始字段与完整请求、跳转地址，请勿公开分享。</p>
    <p v-if="filter.instance">已按插件实例筛选。<el-button @click="filter.instance='';refresh()">清除实例筛选</el-button></p>
    <form class="f3-log-filters" @submit.prevent="refresh">
      <label>关键词<input v-model="filter.keyword" aria-label="搜索日志" placeholder="搜索日志"/></label>
      <label>级别<select v-model="filter.level" aria-label="筛选日志等级" @change="refresh"><option value="">全部等级</option><option v-for="v in ['debug','info','warn','error']" :key="v" :value="v">{{v.toUpperCase()}}</option></select></label>
      <label>模块<select v-model="filter.module" aria-label="筛选日志模块" @change="refresh"><option value="">全部模块</option><option v-for="m in data.data?.modules||[]" :key="m" :value="m">{{moduleLabel(m)}}</option></select></label>
      <label>开始时间<input v-model="from" type="datetime-local" step="1" aria-label="日志开始时间" placeholder="开始时间" @change="refresh"/></label>
      <label>结束时间<input v-model="to" type="datetime-local" step="1" aria-label="日志结束时间" placeholder="结束时间" @change="refresh"/></label>
      <button class="f3-visually-hidden" type="submit">筛选日志</button>
    </form>
    <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" message="日志设置或清理结果尚未确认，请先读取核对，不重复操作。" @refresh="load"/>
    <div class="f3-log-table" role="table" aria-label="日志记录" :aria-busy="data.loading||data.refreshing">
      <div class="f3-log-row f3-log-head" role="row"><span>时间</span><span>级别</span><span>消息</span><span>来源 file:line</span></div>
      <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" :empty="data.loaded&&!data.data?.items.length" empty-title="没有符合筛选条件的日志" @reload="load"/>
      <article v-for="(row,index) in data.data?.items||[]" :key="row.time+':'+index" class="u6-log f3-log-row" role="row">
        <time role="cell">{{formatLogTime(row.time)}}</time>
        <span role="cell" class="f3-log-level" :data-level="String(row.level).toLowerCase()">{{String(row.level||'—').toLowerCase()}}</span>
        <div class="f3-log-message" role="cell"><span>{{row.msg||'—'}}</span><small v-if="trace(row)">trace / span: {{trace(row)}}</small><pre v-if="!clean" class="u6-raw">{{fields(row)}}</pre></div>
        <div role="cell"><button class="f3-log-source" :aria-label="row.url||row.location?'查看完整链接':'完整原始记录'" @click="detail=row;open=true">{{logSource(row)||moduleLabel(row.module)||'—'}}</button></div>
      </article>
    </div>
    <footer class="f3-log-footer">
      <label>每页行数 <select v-model.number="filter.pageSize" aria-label="每页日志数" @change="refresh"><option v-for="n in [50,100,200,500]" :key="n" :value="n">{{n}}</option></select></label>
      <span v-if="data.loading&&!data.loaded">读取中，数量未知</span>
      <span v-else-if="data.error&&!data.loaded">读取失败，数量未知</span>
      <span v-else>{{data.data?.total?((filter.page-1)*filter.pageSize+1):0}} – {{Math.min(filter.page*filter.pageSize,data.data?.total||0)}}，共 {{data.data?.total??'—'}} 条</span>
      <span v-if="data.data?.truncated">检索窗口已达上限，请缩小范围。</span>
      <div class="media-actions"><el-button :icon="DArrowLeft" aria-label="日志首页" :disabled="filter.page<=1" @click="filter.page=1;load()"/><el-button :icon="ArrowLeft" aria-label="上一页日志" :disabled="filter.page<=1" @click="filter.page--;load()"/><span>{{filter.page}} / {{pageCount}}</span><el-button :icon="ArrowRight" aria-label="下一页日志" :disabled="filter.page>=pageCount" @click="filter.page++;load()"/><el-button :icon="DArrowRight" aria-label="日志末页" :disabled="filter.page>=pageCount" @click="filter.page=pageCount;load()"/></div>
      <span role="status">{{copied}}</span>
    </footer>
    <ConfirmAction v-model:visible="confirm" title="清理日志" description="清理全部程序文件日志？" impact-scope="不删除数据库审计或启动控制台日志，也不修改媒体；现有文件日志将不可恢复。" confirm-text="确认清理" danger-level="danger" :loading="op.busy" class="media-dialog" @confirm="clear"/>
    <DetailDrawer v-model:visible="open" title="完整原始日志" class="media-detail">
      <template v-if="detail">
        <label v-if="detail.url" class="u5-field">
          完整请求 URL
          <textarea :value="detail.url" readonly rows="4" aria-label="完整请求URL"/>
        </label>
        <el-button v-if="detail.url" @click="copy(String(detail.url))">复制请求地址</el-button>
        <label v-if="detail.location" class="u5-field">
          完整 302 URL
          <textarea :value="detail.location" readonly rows="4" aria-label="完整302目标URL"/>
        </label>
        <el-button v-if="detail.location" @click="copy(String(detail.location))">复制跳转地址</el-button>
        <p v-if="friendly">{{ friendlyMessage(detail) }} {{ logAdvice(detail) }}</p>
        <pre class="u6-raw">{{ JSON.stringify(detail,null,2) }}</pre>
        <el-button @click="copy(JSON.stringify(detail,null,2))">复制原始记录</el-button>
      </template>
    </DetailDrawer>
    <DetailDrawer v-model:visible="settings" title="日志设置" :technical-data="summary.data" class="media-detail">
      <ResourceState :error="summary.error" @reload="summary.load"/>
      <label class="u5-check"><input v-model="friendly" type="checkbox" aria-label="友好说明"/>详情中显示友好说明</label>
      <h2>日志大小</h2>
      <p>{{ bytes(summary.data?.totalBytes) }} · 上限 {{ bytes(summary.data?.limitBytes) }}</p>
      <label class="u5-field">
        本次运行日志级别
        <select v-model="logLevel" aria-label="本次运行日志级别">
          <option v-for="v in ['debug','info','warn','error']" :key="v" :value="v">{{ v.toUpperCase() }}</option>
        </select>
      </label>
      <el-button :disabled="!!op.pending||!!summary.error" @click="change" type="primary" class="mt-save-button">保存运行日志级别</el-button>
      <p>不回填过去日志；永久默认值在部署配置中设置。</p>
      <article v-for="file in summary.data?.files||[]" :key="file.name" class="organize-record">
        <h3>{{ file.name }}</h3>
        <p>{{ bytes(file.size) }} · {{ formatLogTime(file.modified) }}</p>
      </article>
    </DetailDrawer>
    <el-dialog v-model="copyOpen" title="复制原始日志" width="min(740px,94vw)" class="media-dialog u5-dialog">
      <p>复制失败，请手动选择下方完整内容。</p>
      <textarea :value="copyContent" readonly rows="14" aria-label="待复制日志" class="u6-raw"/>
      <template #footer>
        <el-button @click="copyOpen=false">关闭</el-button>
      </template>
    </el-dialog>
  </section>
</template>
