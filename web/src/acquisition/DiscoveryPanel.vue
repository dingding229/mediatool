<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { Refresh } from '@element-plus/icons-vue';
import { useDiscovery } from './useDiscovery';
import { type MediaItem, sourceState } from '../media/model';
import { errorText } from '../workflows/readiness';
import PosterWall from '../media/PosterWall.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import '../media/catalog.css';
const emit = defineEmits<{
  select: [
    MediaItem
  ];
}>(), { kind, page, items, hasMore, truncated, loading, error, detail, detailOpen, detailLoading, detailError, load, inspect, change } = useDiscovery();
usePageRefresh(load, 'discover');
</script>
<template>
  <section class="discovery-panel catalog-workspace" aria-label="发现推荐">
    <header class="catalog-toolbar">
      <div>
        <h2>发现值得看的作品</h2>
        <p>来自已启用的发现与推荐服务，不代表作品已入库。</p>
      </div>

    </header>
    <nav class="catalog-subnav mt-segments" aria-label="发现推荐范围">
      <button :aria-pressed="kind==='discover.source'" @click="change('discover.source')">发现</button>
      <button :aria-pressed="kind==='recommend.source'" @click="change('recommend.source')">推荐</button>
    </nav>
    <p v-if="error&&(items.length)" class="f3-stale-warning" role="status">刷新失败，保留上次内容。<button @click="load">重试</button></p>
    <StateView v-if="loading" state="loading" title="正在读取推荐作品…"/>
    <StateView v-else-if="error&&!(items.length)" :state="sourceState(error)" :description="errorText(error)" @retry="load"/>
    <template v-else>
      <div class="discovery-grid">
        <article v-for="item in items" :key="item.id">
          <PosterWall :items="[item]" @select="inspect"/>
          <p>来源：{{item.source||'—'}}</p>
          <el-button :disabled="!item.tmdbId" @click="emit('select',item)">搜索资源</el-button>
        </article>
      </div>
      <StateView v-if="!items.length" state="empty" title="暂无发现推荐" description="已启用的服务尚未返回作品。"/>
      <p v-if="truncated" role="status">来源仅提供部分结果，未展示的作品不代表不存在。</p>
      <PagePagination :page-size="24" v-model:current-page="page" :has-next="hasMore" :disabled="loading" @current-change="load"/>
    </template>
    <DetailDrawer v-model:visible="detailOpen" :title="detail?.title||'作品详情'" class="catalog-detail">
      <StateView v-if="detailLoading" state="loading"/>
      <StateView v-else-if="detailError" :state="sourceState(detailError)" :description="errorText(detailError)" @retry="detail&&inspect(detail)"/>
      <p>{{detail?.year||'—'}} · {{detail?.genres?.join('、')||'—'}}</p>
      <p>{{detail?.overview||'—'}}</p>
      <p>来源：{{detail?.source||'—'}}</p>
      <el-button :disabled="!detail?.tmdbId" @click="detail&&emit('select',detail)">搜索资源</el-button>
    </DetailDrawer>
  </section>
</template>
<style scoped>
.discovery-panel{width:100%;margin:0}
.discovery-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:28px 20px;margin:24px 0}.discovery-grid article{min-width:0}.discovery-grid :deep(.catalog-wall){display:block}.discovery-grid p,.discovery-panel header p{font-size:13px;color:var(--mt-text-muted);overflow-wrap:anywhere}.discovery-grid .el-button{width:100%}.discovery-panel h2{font-size:20px}.discovery-panel button:disabled{cursor:not-allowed;color:var(--mt-text-muted);opacity:.6}@media(max-width:680px){.discovery-grid{grid-template-columns:repeat(2,minmax(0,1fr));gap:20px 12px}}
</style>
