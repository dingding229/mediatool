<script setup lang="ts">
import { View, Delete, Search, Picture } from '@element-plus/icons-vue';
import {usePageNumber,usePageSize} from '../ui/page-query';

import {useRefresh} from '../composables/useRefresh';
import { pageResult } from './model';
import { formatBytes as bytes } from './model';
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { cacheTime } from '../utils/cache-time';
import { tmdbImage } from '../utils/tmdb-image';
import { date, errorText } from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
const notice = ref(''), posterErrors = ref(new Set<string>());
const route = useRoute(), router = useRouter(), kind = computed<'tmdb' | 'search'>(() => route.query.cacheKind === 'search' ? 'search' : 'tmdb'), page = usePageNumber(), pageSize = usePageSize(30), loadedAt = ref(Date.now()), now = ref(Date.now()), data = reactive(useResource<any>(() => request(`/cache/${kind.value}?page=${page.value}&pageSize=${pageSize.value}`).then(pageResult))), detail = ref<any>(null), open = ref(false), detailError = ref<unknown>(null), op = reactive(useOperation('cache')), confirm = ref(false), removeId = ref('');
function setKind(value: string) {
  void router.replace({ query: { ...route.query, cacheKind: value } });
}
async function load() {
  const out = await data.load();
  if (out)
    loadedAt.value = Date.now();
  if (op.pending && !data.error) {
    const p = JSON.parse(op.pending.key);
    if (p.kind === kind.value) {
      if (op.pending.status === 200)
        op.clear();
      else if (p.id)
        try {
          await request(`/cache/${p.kind}/${encodeURIComponent(p.id)}`);
        }
        catch (e: any) {
          if (e.status === 404)
            op.clear();
        }
    }
  }
}
async function inspect(id: string) {
  detailError.value = null;
  open.value = true;
  detail.value = null;
  try {
    detail.value = await request(`/cache/${kind.value}/${encodeURIComponent(id)}`);
  }
  catch (e) {
    detailError.value = e;
  }
}
function ask(id = '') {
  removeId.value = id;
  confirm.value = true;
}
async function apply() {
  const k = kind.value, id = removeId.value;
  const out = await op.run(JSON.stringify({ kind: k, id }), () => id ? request(`/cache/${k}/${encodeURIComponent(id)}?confirm=true`, { method: 'DELETE' }) : post(`/cache/${k}/clear`, { confirm: true }));
  if (out && op.pending?.status === 200)
    notice.value = id ? '此缓存项已处理，请核对当前列表。' : `本轮删除 ${out.deleted ?? '—'} 项；${out.remainingMayExist ? '仍可能有更多缓存，请核对后分批清理。' : '请核对当前列表。'}`;
  confirm.value = false;
  page.value = 1;
  await load();
}
watch(kind, () => {
  page.value = 1;
  data.data = null;
  open.value = false;
  void load();
});
onMounted(()=>void load());
useRefresh(()=>{now.value=Date.now()},{interval:1000,immediate:false});
defineExpose({ load, op, apply, ask, data, kind, inspect });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface ops-cache-panel" :class="{'is-search':kind==='search'}">
      <header class="media-section-heading">
        <h2>缓存管理</h2>
        <el-button :disabled="!!op.pending||!!data.error" @click="ask()">清理此类缓存</el-button>
      </header>
      <nav class="media-actions mt-segments" aria-label="缓存范围">
        <el-button :aria-pressed="kind==='tmdb'" @click="setKind('tmdb')">TMDB</el-button>
        <el-button :aria-pressed="kind==='search'" @click="setKind('search')">搜索缓存</el-button>
      </nav>
      <p class="ops-cache-note">只管理缓存复用；清理搜索缓存会隐藏记录，底层结果、下载证明和媒体保留。</p>
      <div class="ops-content-stage ops-cache-stage" :aria-busy="data.loading||data.refreshing">
      <ResourceState :stale="!!data.data" :loading="(!data.loaded||!data.data)&&!data.error" :error="data.error" :empty="!!data.data&&!data.data.items?.length" @reload="load"/>
      <p class="ops-cache-summary">当前检索共 {{ data.loading||data.error?'—':data.data?.total??'—' }} 项 · 本页 {{ data.loading||data.error?'—':bytes(data.data?.items.reduce((n:number,r:any)=>n+(r.bytes||0),0)) }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="data.data?.truncated">扫描存在上限，当前列表未必覆盖全部缓存；可分批清理并重新读取。</p>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" message="缓存清理结果尚未确认，请先查看当前结果，不重复清理。" @refresh="load"/>
      <div v-if="data.data?.items?.length" class="mt-list-columns ops-cache-columns" aria-hidden="true"><span>查询 / 作品</span><span>结果 / 缓存时间</span><span>操作</span></div>
      <article v-for="row in data.data?.items||[]" :key="row.id" class="mt-list-row ops-cache-row">
        <div class="u6-cache-content">
          <img v-if="tmdbImage(row.posterPath)&&!posterErrors.has(row.id)" :src="tmdbImage(row.posterPath)" :alt="row.query+'海报'" loading="lazy" @error="posterErrors.add(row.id)"/>
          <span v-else class="ops-cache-poster-placeholder" aria-hidden="true"><el-icon><Picture/></el-icon></span>
          <h3>{{ row.query||'媒体查询' }}</h3>
        </div>
        <div class="ops-cache-meta">
          <p>{{ row.count??'—' }} 个结果 · {{ cacheTime(kind,row,(now-loadedAt)/1000) }}</p>
          <p>{{ date(row.createdAt) }}</p>
        </div>
        <div class="media-actions mt-list-actions">
          <RouterLink v-if="kind==='search'" class="ops-cache-result-link" :to="'/search/results/'+encodeURIComponent(row.id)" aria-label="查看搜索结果"><el-icon class="mt-list-action-icon"><Search/></el-icon><span class="mt-list-action-text">查看搜索结果</span></RouterLink>
          <el-button size="small" aria-label="缓存详情" @click="inspect(row.id)"><el-icon class="mt-list-action-icon"><View/></el-icon><span class="mt-list-action-text">缓存详情</span></el-button>
          <el-button size="small" aria-label="删除缓存项" :disabled="!!op.pending" @click="ask(row.id)"><el-icon class="mt-list-action-icon"><Delete/></el-icon><span class="mt-list-action-text">删除缓存项</span></el-button>
        </div>
      </article>
      </div>
      <PagePagination v-model:current-page="page" :total="data.data?.total" v-model:page-size="pageSize" :page-sizes="[10,25,30,50,100]" layout="prev,pager,next" @current-change="load"/>
    <p class="ops-cache-note ops-cache-ttl">
      缓存有效期统一在
      <RouterLink to="/settings?tab=automation">设置 · 搜索订阅</RouterLink>
      中调整。
    </p>
    </section>
    <ConfirmAction v-model:visible="confirm" :title="removeId?'删除缓存项':'清理此类缓存'" description="确认清理所选范围？" :impact-scope="kind==='search'?'从搜索缓存移除并停止复用，保留底层结果、下载记录和媒体。':'仅删除当前影视缓存，不删除媒体或其他业务数据。'" confirm-text="确认清理" :loading="op.busy" class="media-dialog" @confirm="apply"/>
    <DetailDrawer v-model:visible="open" title="缓存内容" :technical-data="detail?.data??detail" size="large" class="media-detail">
      <ResourceState :error="detailError" @reload="load"/>
      <h3>{{ detail?.query||'—' }}</h3>
    </DetailDrawer>
  </div>
</template>
