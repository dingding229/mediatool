<script setup lang="ts">
import { computed, onMounted, ref, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Search } from '@element-plus/icons-vue';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import StateView from '../ui/StateView.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import DiscoveryPanel from '../acquisition/DiscoveryPanel.vue';
import type { MediaItem } from '../media/model';
import SearchHistory from '../acquisition/SearchHistory.vue';
import { label } from '../acquisition/model';
import { useSearch } from '../acquisition/useSearch';
import { errorText, stateFor } from '../workflows/readiness';
import { tmdbImage } from '../utils/tmdb-image';
import '../acquisition/acquisition.css';
const route = useRoute(), router = useRouter(), tab = computed(() => ['history', 'discover'].includes(String(route.query.tab)) ? String(route.query.tab) : 'query'), tabs = [{ id: 'query', label: '搜索' }, { id: 'history', label: '搜索历史' }, { id: 'discover', label: '发现推荐' }], broken = ref(new Set<string>()), discovered = ref(false);
const { loading, mode, query, kind, year, keyword, selected, media, history, sites, accountIds, searching, busy, searched, cached, error, accountsError, historyError, ready, intent, load, findMedia: findMediaRequest, start, reconcile } = useSearch();
function findMedia(refresh = false) {
  discovered.value = false;
  return findMediaRequest(refresh);
}
async function fromDiscovery(item: MediaItem) {
  if (!item.tmdbId)
    return;
  mode.value = 'media';
  query.value = item.title;
  kind.value = item.mediaType || 'auto';
  year.value = String(item.year || '');
  await nextTick();
  const chosen = { ...item, id: item.tmdbId, mediaType: item.mediaType || 'movie', year: String(item.year || ''), posterPath: item.poster };
  selected.value = chosen;
  media.value = [chosen];
  searched.value = true;
  discovered.value = true;
  cached.value = false;
  error.value = null;
  await router.replace({ query: { ...route.query, tab: 'query' } });
}
onMounted(() => {
  if (typeof route.query.run === 'string')
    void router.replace(`/search/results/${route.query.run}`);
});
</script>
<template>
  <div class="acquire-workspace">
    <PageHeader title="资源搜索" description="先确认作品，再选择站点；投递前核对身份与做种要求。"/>
    <PageTabs :tabs="tabs" default-tab="query"/>
    <template v-if="tab==='query'">
      <section class="acquire-surface">
        <div class="acquire-mode" role="group" aria-label="搜索方式">
          <button type="button" :aria-pressed="mode==='media'" @click="mode='media'">TMDB 媒体搜索</button>
          <button type="button" :aria-pressed="mode==='site'" @click="mode='site'">站点直搜</button>
        </div>
        <form v-if="mode==='media'" class="acquire-form" @submit.prevent="findMedia(false)">
          <div>
            <h2>你想找哪部作品？</h2>
            <p>先核对年份与类型，避免同名作品选错。</p>
          </div>
          <div class="acquire-query-row">
            <label>
              媒体名称
              <el-input v-model="query" aria-label="TMDB媒体名称" :prefix-icon="Search" placeholder="电影或剧集名称" clearable/>
            </label>
            <label>
              类型
              <el-select v-model="kind" aria-label="TMDB媒体类型">
                <el-option value="auto" label="自动识别"/>
                <el-option value="movie" label="电影"/>
                <el-option value="series" label="剧集"/>
              </el-select>
            </label>
            <label>
              年份（可选）
              <el-input v-model="year" aria-label="TMDB年份" maxlength="4" inputmode="numeric"/>
            </label>
          </div>
          <div class="acquire-actions">
            <el-button :type="selected?'default':'primary'" native-type="submit" :loading="searching" :disabled="!query.trim()">搜索媒体</el-button>
            <el-button :disabled="searching||!query.trim()" @click="findMedia(true)">忽略TMDB缓存</el-button>
            <span v-if="searched">{{discovered?'已从发现推荐选择作品':cached?'本次使用TMDB缓存':'本次已查询TMDB'}}</span>
          </div>
        </form>
        <form v-else class="acquire-form" @submit.prevent="start(false)">
          <h2>按资源关键词搜索站点</h2>
          <el-input v-model="keyword" aria-label="资源关键词" placeholder="影片、剧集或发布关键词"/>
          <p>关键词不是媒体身份，下载前仍需核对具体作品。</p>
          <div class="acquire-actions">
            <el-button type="primary" native-type="submit" :disabled="!keyword.trim()||!ready||!!intent" :loading="busy">开始搜索</el-button>
            <el-button :disabled="!keyword.trim()||!ready||busy||!!intent" @click="start(true)">忽略站点缓存重新搜索</el-button>
          </div>
        </form>
      </section>
      <StateView v-if="error" :state="stateFor(error)" title="搜索暂未完成" :description="errorText(error)" @retry="mode==='media'?findMedia(false):reconcile()"/>
      <StateView v-if="searching" state="loading" title="正在查找匹配作品…"/>
      <section v-else-if="mode==='media'&&searched" class="acquire-surface">
        <header class="acquire-section-heading">
          <h2>匹配作品</h2>
          <span>{{ media.length }} 项</span>
        </header>
        <div class="acquire-media-grid">
          <article v-for="item in media" :key="item.mediaType+item.id" :class="{selected:selected?.id===item.id&&selected?.mediaType===item.mediaType}">
            <img v-if="tmdbImage(item.posterPath)&&!broken.has(item.mediaType+item.id)" :src="tmdbImage(item.posterPath)" :alt="item.title+'海报'" @error="broken.add(item.mediaType+item.id)"/>
            <div v-else class="acquire-poster-fallback">暂无海报</div>
            <div>
              <h3>
                <button type="button" :aria-label="item.title" :aria-pressed="selected?.id===item.id&&selected?.mediaType===item.mediaType" @click="selected=item">{{ item.title }}</button>
              </h3>
              <p>{{ item.mediaType==='series'?'剧集':'电影' }} · {{ item.year||'—' }}</p>
              <p>{{ item.originalTitle||'—' }}</p>
              <p class="acquire-overview">{{ item.overview||'暂无简介' }}</p>
            </div>
          </article>
        </div>
        <StateView v-if="!media.length" state="empty" title="没有匹配作品" description="试试原标题、另一年份或调整类型。"/>
      </section>
      <section class="acquire-surface">
        <h2>{{ mode==='media'?'选择站点，查找资源':'站点范围' }}</h2>
        <p v-if="selected&&mode==='media'">已选择：{{ selected.title }} · {{ selected.year||'—' }}</p>
        <el-select v-model="accountIds" multiple clearable aria-label="搜索站点" placeholder="全部已启用站点">
          <el-option v-for="site in sites" :key="site.id" :value="site.id" :label="site.name"/>
        </el-select>
        <p>默认查询全部 {{ loading||accountsError?'—':sites.length }} 个已启用站点；可指定站点缩小范围。</p>
        <StateView v-if="loading" state="loading" title="正在读取可用站点…"/>
        <StateView v-else-if="accountsError" :state="stateFor(accountsError)" title="站点配置读取失败" @retry="load"/>
        <p v-else-if="!ready">
          还没有可用站点。
          <RouterLink to="/sites?tab=accounts">配置站点账号</RouterLink>
        </p>
        <el-button v-if="mode==='media'" type="primary" :disabled="!selected||!ready||!!intent" :loading="busy" @click="start(false)">搜索站点资源</el-button>
      </section>
      <AsyncResultBanner v-if="intent" :status="intent.phase" :message="intent.phase==='queued'?'搜索已排队，请进入记录查看查询进度。':'搜索回执未知，先读取记录核对，不重复发起。'" @refresh="reconcile"/>
      <section class="acquire-surface" aria-label="近期搜索">
        <header class="acquire-section-heading">
          <h2>近期搜索</h2>
          <RouterLink to="/search?tab=history">全部搜索历史</RouterLink>
        </header>
        <p v-if="historyError" role="alert">
          搜索记录读取失败。
          <el-button @click="load">重新读取</el-button>
        </p>
        <div class="acquire-history">
          <article v-for="item in history.slice(0,5)" :key="item.id">
            <div>
              <h3>{{ item.keyword }}</h3>
              <p>{{ label(item.state) }} · {{ item.total??'—' }} 个资源</p>
            </div>
            <RouterLink :to="`/search/results/${item.id}`">查看结果</RouterLink>
          </article>
        </div>
        <p v-if="!loading&&!history.length&&!historyError">暂无搜索记录</p>
      </section>
    </template>
    <DiscoveryPanel v-else-if="tab==='discover'" @select="fromDiscovery"/>
    <SearchHistory v-else :runs="history" :runs-error="historyError"/>
  </div>
</template>
