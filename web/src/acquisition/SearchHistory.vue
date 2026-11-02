<script setup lang="ts">
import {joinMetadata} from '../presentation/metadata';

import {Film} from '@element-plus/icons-vue';
import {usePageNumber,usePageSize} from '../ui/page-query';

import {usePageRefresh} from '../ui/page-refresh';
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { tmdbImage } from '../utils/tmdb-image';
import { request } from '../api/client';
import StatusBadge from '../components/StatusBadge.vue';
import StateView from '../ui/StateView.vue';
import {useDebouncedFilter} from '../composables/useDebouncedFilter';
import { date, errorText, stateFor } from '../workflows/readiness';
import { label, type Run } from './model';
const props = defineProps<{
  runs: Run[];
  runsError?: unknown;
}>();
const broken = ref(new Set<string>()), entries = ref<any[]>([]), total = ref(0), page = usePageNumber(), pageSize = usePageSize(25), loading = ref(true), error = ref<unknown>(null), truncated = ref(false);
let epoch = 0, disposed = false;
const query=ref(''),appliedQuery=ref('');
useDebouncedFilter(query,value=>{appliedQuery.value=value.trim();page.value=1;return load()});
const rows = computed(() => {
  const combined = [...entries.value, ...(error.value&&page.value === 1&&!appliedQuery.value ? props.runs : [])];
  return combined.filter((r, i) => combined.findIndex(x => x.id === r.id) === i);
});
let hasLoaded=false;
  async function load() {
  const version = ++epoch;
  loading.value = !hasLoaded;
  error.value = null;
  try {
    const out = await request<{
      items: any[];
      total: number;
      truncated?: boolean;
    }>(`/cache/search?page=${page.value}&pageSize=${pageSize.value}&q=${encodeURIComponent(appliedQuery.value)}`);
    if (version === epoch && !disposed) {
      entries.value = out.items || [];
      total.value = out.total;
      truncated.value = !!out.truncated;
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
onMounted(load);
onUnmounted(() => {
  disposed = true;
  epoch++;
});
usePageRefresh(load, 'history');
</script>
<template>
  <section class="acquire-surface" aria-label="搜索历史">
    <header class="acquire-section-heading">
      <div>
        <h2>继续上次的搜索</h2>
        <p>这里查看记录与结果；缓存清理和有效期在运维中管理。</p>
      </div>
      <RouterLink to="/ops?tab=cache&amp;cacheKind=search">前往缓存管理</RouterLink>

    </header>
    <el-input v-model="query" clearable aria-label="筛选搜索历史" placeholder="筛选搜索历史"/>
    <p v-if="error&&hasLoaded" class="f3-stale-warning" role="status">刷新失败，保留上次内容。<button @click="load">重试</button></p>
    <StateView v-if="loading" state="loading" title="正在读取搜索历史…"/>
    <StateView v-else-if="error&&!hasLoaded" :state="stateFor(error)" title="部分搜索历史读取失败" :description="errorText(error)" @retry="load"/>
    <p v-if="runsError" role="alert">近期搜索记录暂不可用，已取得的缓存记录仍可查看。</p>
    <p v-if="truncated" role="status">仅取得部分记录，没有显示不代表没有历史。</p>
    <div v-if="!loading" class="acquire-history">
      <RouterLink v-for="row in rows" :key="row.id" class="search-history-row" :to="`/search/results/${row.id}`">
        <img v-if="tmdbImage(row.posterPath||row.media?.posterPath)&&!broken.has(row.id)" class="acquire-history-cover" :src="tmdbImage(row.posterPath||row.media?.posterPath)" alt="作品海报" @error="broken.add(row.id)"/>
        <span v-else class="acquire-history-cover history-cover-placeholder" aria-label="暂无海报"><Film aria-hidden="true"/></span>
        <span class="history-row-copy"><strong>{{row.query||row.keyword||'—'}}</strong><span class="history-row-meta"><StatusBadge :state="row.state" :label="label(row.state)"/><span>{{joinMetadata([(date(row.createdAt)),joinMetadata([(row.count??row.total??'—')," 个资源"], '')])}}</span></span></span>
        <span aria-hidden="true">查看结果</span>
      </RouterLink>
    </div>
    <StateView v-if="!loading&&!error&&!runsError&&!rows.length" state="empty" :title="appliedQuery?'没有匹配的搜索记录':'还没有搜索记录'" :description="appliedQuery?'清除关键词后查看全部历史。':'先选择作品并发起站点搜索，记录会保存在这里。'"/>
    <PagePagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" layout="prev,pager,next" @current-change="load"/>
  </section>
</template>
