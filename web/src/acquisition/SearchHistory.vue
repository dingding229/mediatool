<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { tmdbImage } from '../utils/tmdb-image';
import { request } from '../api/client';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import { date, errorText, stateFor } from '../workflows/readiness';
import { label, type Run } from './model';
const props = defineProps<{
  runs: Run[];
  runsError?: unknown;
}>();
const broken = ref(new Set<string>()), entries = ref<any[]>([]), total = ref(0), page = ref(1), loading = ref(true), error = ref<unknown>(null), detail = ref<any>(null), open = ref(false), truncated = ref(false);
let epoch = 0, disposed = false;
const rows = computed(() => {
  const combined = [...entries.value, ...(page.value === 1 ? props.runs : [])];
  return combined.filter((r, i) => combined.findIndex(x => x.id === r.id) === i);
});
async function load() {
  const version = ++epoch;
  loading.value = true;
  error.value = null;
  try {
    const out = await request<{
      items: any[];
      total: number;
      truncated?: boolean;
    }>(`/cache/search?page=${page.value}`);
    if (version === epoch && !disposed) {
      entries.value = out.items || [];
      total.value = out.total;
      truncated.value = !!out.truncated;
    }
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
async function inspect(id: string) {
  try {
    detail.value = await request(`/cache/search/${encodeURIComponent(id)}`);
    if (!disposed)
      open.value = true;
  }
  catch (e) {
    error.value = e;
  }
}
onMounted(load);
onUnmounted(() => {
  disposed = true;
  epoch++;
});
</script>
<template>
  <section class="acquire-surface" aria-label="搜索历史">
    <header class="acquire-section-heading">
      <div>
        <h2>继续上次的搜索</h2>
        <p>这里查看记录与结果；缓存清理和有效期在运维中管理。</p>
      </div>
      <RouterLink to="/ops?tab=cache&amp;cacheKind=search">前往缓存管理</RouterLink>
      <el-button :loading="loading" @click="load">刷新记录</el-button>
    </header>
    <StateView v-if="loading" state="loading" title="正在读取搜索历史…"/>
    <StateView v-else-if="error" :state="stateFor(error)" title="部分搜索历史读取失败" :description="errorText(error)" @retry="load"/>
    <p v-if="runsError" role="alert">近期搜索记录暂不可用，已取得的缓存记录仍可查看。</p>
    <p v-if="truncated" role="status">仅取得部分记录，没有显示不代表没有历史。</p>
    <div v-if="!loading" class="acquire-history">
      <article v-for="row in rows" :key="row.id">
        <div>
          <img v-if="tmdbImage(row.posterPath||row.media?.posterPath)&&!broken.has(row.id)" class="acquire-history-cover" :src="tmdbImage(row.posterPath||row.media?.posterPath)" alt="作品海报" @error="broken.add(row.id)"/>
          <h3>{{ row.query||row.keyword||'—' }}</h3>
          <p>{{ label(row.state) }} · {{ date(row.createdAt) }} · {{ row.count??row.total??'—' }} 个资源</p>
        </div>
        <div class="acquire-actions">
          <RouterLink :to="`/search/results/${row.id}`">查看结果</RouterLink>
          <el-button v-if="entries.some(e=>e.id===row.id)" @click="inspect(row.id)">查看记录详情</el-button>
        </div>
      </article>
    </div>
    <StateView v-if="!loading&&!error&&!runsError&&!rows.length" state="empty" title="还没有搜索记录" description="先选择作品并发起站点搜索，记录会保存在这里。"/>
    <el-pagination v-if="total>25" v-model:current-page="page" :page-size="25" :total="total" layout="prev,pager,next" @current-change="load"/>
  </section>
  <DetailDrawer v-model:visible="open" title="搜索记录详情" :technical-data="detail">
    <h3>{{ detail?.query||detail?.keyword||'—' }}</h3>
    <p>{{ label(detail?.state) }}</p>
    <p>{{ date(detail?.createdAt) }}</p>
  </DetailDrawer>
</template>
