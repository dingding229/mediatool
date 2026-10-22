<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';

import { computed, onMounted, onUnmounted, ref } from 'vue';
import { request, post } from '../api/client';
import DetailDrawer from '../ui/DetailDrawer.vue';
import StateView from '../ui/StateView.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { readIntent, writeIntent } from '../workflows/intents';
import { sourceState, image } from '../media/model';
import { label, rows } from '../organize/model';
import { errorText } from '../workflows/readiness';
const items = ref<any[]>([]), page = usePageNumber(), total = ref(0), query = ref(''), kind = ref(''), directory = ref(''), loading = ref(true), error = ref<unknown>(null), asset = ref<any>(null), visible = ref(false), content = ref<any>(null), contentLoading = ref(false), contentError = ref<unknown>(null), feedback = ref(''), busy = ref(false), phase = ref<'queued' | 'reconciling' | null>(null);
const contentPath = ref('');
let epoch = 0, contentEpoch = 0, disposed = false;
const files = computed(() => items.value.flatMap(a => rows(a.outputs).filter(o => o.kind === 'strm').map(o => ({ ...o, asset: a }))).filter(o => !directory.value || o.path.startsWith(directory.value + '/'))), folders = computed(() => [...new Set(files.value.map(o => o.path.slice(directory.value ? directory.value.length + 1 : 0).split('/').slice(0, -1)[0]).filter(Boolean))]), directFiles = computed(() => files.value.filter(o => !o.path.slice(directory.value ? directory.value.length + 1 : 0).includes('/'))), crumbs = computed(() => [{ name: '本页受管目录', path: '' }, ...directory.value.split('/').filter(Boolean).map((name, i, a) => ({ name, path: a.slice(0, i + 1).join('/') }))]);
async function load() {
  const current = ++epoch;
  loading.value = true;
  error.value = null;
  try {
    const out: any = await request(`/library/outputs?kind=strm&page=${page.value}&pageSize=25&mediaType=${kind.value}&q=${encodeURIComponent(query.value)}`);
    if (disposed || current !== epoch)
      return;
    items.value = rows(out.items);
    total.value = out.total;
    if (asset.value) {
      const fresh = items.value.find(a => a.id === asset.value.id);
      if (fresh) {
        asset.value = fresh;
        if (['visible', 'failed', 'partial', 'unavailable'].includes(fresh.embyState)) {
          writeIntent('emby-check:' + fresh.id, null);
          phase.value = null;
        }
      }
    }
  }
  catch (e) {
    if (current === epoch)
      error.value = e;
  }
  finally {
    if (current === epoch)
      loading.value = false;
  }
}
function search() {
  page.value = 1;
  directory.value = '';
  void load();
}
function inspect(row: any) {
  contentEpoch++;
  asset.value = row;
  visible.value = true;
  content.value = null;
  feedback.value = '';
  contentError.value = null;
  contentLoading.value = false;
  phase.value = readIntent('emby-check:' + row.id)?.phase || null;
}
async function readContent(row: any, path: string) {
  if (!visible.value || asset.value?.id !== row.id)
    inspect(row);
  contentPath.value = path;
  const current = ++contentEpoch;
  contentLoading.value = true;
  content.value = null;
  contentError.value = null;
  try {
    const out: any = await request(`/library/outputs/${row.id}/content?path=${encodeURIComponent(path)}`);
    if (!disposed && current === contentEpoch)
      content.value = out;
  }
  catch (e) {
    if (current === contentEpoch)
      contentError.value = e;
  }
  finally {
    if (current === contentEpoch)
      contentLoading.value = false;
  }
}
async function refreshEmby() {
  if (!asset.value || phase.value || busy.value)
    return;
  const id = asset.value.id;
  busy.value = true;
  phase.value = 'reconciling';
  writeIntent('emby-check:' + id, { phase: 'reconciling' });
  try {
    await post(`/library/outputs/${id}/emby`, {});
    if (asset.value?.id === id)
      phase.value = 'queued';
    writeIntent('emby-check:' + id, { phase: 'queued' });
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function copy() {
  try {
    await navigator.clipboard.writeText(content.value.content);
    feedback.value = '原始内容已复制';
  }
  catch {
    feedback.value = '复制失败，请在下方原文中手动选择复制。';
  }
}
onMounted(load);
onUnmounted(() => {
  disposed = true;
  epoch++;
  contentEpoch++;
});
defineExpose({ refresh: load });
</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>STRM 目录</h2>
      <span>{{ loading||error?'—':total }} 项资产</span>
    </header>
    <form class="files-browser-toolbar" @submit.prevent="search">
      <label>
        标题或路径
        <el-input v-model="query" aria-label="托管输出关键词"/>
      </label>
      <label>
        媒体类型
        <el-select v-model="kind" aria-label="STRM媒体类型" @change="search">
          <el-option value="" label="全部类型"/>
          <el-option value="movie" label="电影"/>
          <el-option value="series" label="剧集"/>
        </el-select>
      </label>
      <el-button native-type="submit">筛选受管输出</el-button>
    </form>
    <p>目录按当前页受管资产的真实 STRM 路径分组；筛选先于分页，不展示或删除已有 NFO。</p>
    <StateView v-if="loading" state="loading" title="正在读取受管目录…"/>
    <StateView v-else-if="error" :state="sourceState(error)" title="受管目录读取失败" :description="errorText(error)" @retry="load"/>
    <template v-else>
      <nav class="files-breadcrumbs" aria-label="STRM目录路径">
        <button v-for="crumb in crumbs" :key="crumb.path" @click="directory=crumb.path">{{ crumb.name }}</button>
      </nav>
      <StateView v-if="!items.length" state="empty" title="暂无匹配的受管 STRM"/>
      <ul class="files-list">
        <li v-for="folder in folders" :key="folder" class="files-entry">
          <button class="files-directory" @click="directory=directory?directory+'/'+folder:folder">
            {{ folder }}
            <small>文件夹</small>
          </button>
        </li>
        <li v-for="row in directFiles" :key="row.path" class="files-entry">
          <div class="files-entry-copy">
            <strong>{{ row.path.split('/').pop() }}</strong>
            <span>{{ row.asset.title||'—' }} · STRM</span>
          </div>
          <el-button @click="readContent(row.asset,row.path)">预览STRM内容</el-button>
        </li>
      </ul>
      <details class="files-technical">
        <summary>本页资产与输出</summary>
        <article v-for="row in items" :key="row.id" class="organize-record">
          <h3>{{ row.title||'—' }} {{ row.year||'' }}</h3>
          <p>{{ row.source }} · {{ label(row.cloudState) }} · {{ label(row.artifactsState) }}</p>
          <div class="media-actions">
            <el-button @click="inspect(row)">资产与输出详情</el-button>
          </div>
        </article>
      </details>
      <PagePagination v-model:current-page="page" :page-size="25" :total="total" layout="prev,pager,next,total" @current-change="directory='';load()"/>
    </template>
  </section>
  <DetailDrawer v-model:visible="visible" title="受管 STRM 与媒体详情" :technical-data="{asset,content}" class="media-detail">
    <h3>{{ asset?.title||'—' }}</h3>
    <img v-if="image(asset?.poster)" :src="image(asset.poster)" :alt="asset?.title" class="files-detail-poster"/>
    <p>{{ asset?.source||'—' }}</p>
    <p>{{ label(asset?.cloudState) }} · {{ label(asset?.artifactsState) }} · Emby：{{ label(asset?.embyState) }}</p>
    <p v-for="report in asset?.scrapeReport||[]" :key="report.kind">{{ report.kind }} · {{ label(report.state) }} · {{ report.message||'—' }}</p>
    <p v-for="out in asset?.outputs?.filter((o:any)=>o.kind==='strm')||[]" :key="out.path">
      {{ out.path }}
      <el-button @click="readContent(asset,out.path)">读取原文</el-button>
    </p>
    <p v-for="path in asset?.embyPaths||[]" :key="path.path">{{ path.path }} · {{ path.visible?'Emby 已可见':'未确认可见' }} · {{ path.reason||'—' }}</p>
    <StateView v-if="contentLoading" state="loading" title="正在读取原文…"/>
    <StateView v-else-if="contentError" :state="sourceState(contentError)" title="原文读取失败" :description="errorText(contentError)" retry-text="重新读取原文" @retry="readContent(asset,contentPath)"/>
    <template v-else-if="content">
      <el-input :model-value="content.content" type="textarea" :rows="7" readonly aria-label="托管输出原始内容"/>
      <el-button @click="copy">复制原始内容</el-button>
      <p role="status">{{ feedback }}</p>
    </template>
    <AsyncResultBanner v-if="phase" :status="phase" message="同步请求已排队或正在核对，尚未确认作品已出现在 Emby。" @refresh="load"/>
    <div class="media-actions">
      <el-button :disabled="!asset||!!phase||busy||!!error" @click="refreshEmby">同步此作品到 Emby</el-button>
      <el-button @click="load">核对 Emby 可见性</el-button>
    </div>
  </DetailDrawer>
</template>
