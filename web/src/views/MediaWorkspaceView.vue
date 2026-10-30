<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import {usePageRefresh} from '../ui/page-refresh';
import { Refresh } from '@element-plus/icons-vue';
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import PosterWall from '../media/PosterWall.vue';
import SubtitlePanel from '../media/SubtitlePanel.vue';
import MediaEditDialog from '../media/MediaEditDialog.vue';
import {mediaFacts,mediaFileFacts,joinMediaFacts} from '../media/detail-presentation';
import DescriptionGrid from '../ui/DescriptionGrid.vue';
import PosterArtwork from '../media/PosterArtwork.vue';
import {ArrowRight} from '@element-plus/icons-vue';
import SeasonEpisodes from '../media/SeasonEpisodes.vue';
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
import {seasonLabel} from '../media/season-label';
import {managedDetail} from '../api/mediaCatalog';
import { useCatalog } from '../media/useCatalog';
import { image, mediaType, sourceState, displayRating, displayResolution, displayMediaSize } from '../media/model';
import { errorText } from '../workflows/readiness';
import { formatBytes } from '../operations/model';
import '../media/catalog.css';
const { pageSize,childSize, selectedSeason, chooseSeason, displayItems, source, view, page, query, kind, parentId, items, total, loading, error, detail, detailOpen, detailLoading, detailError, children, childTotal, childPage, childError, childLoading, episodes, series, category, categories, load, search, all, browse, inspect, refresh, readChildren } = useCatalog();
usePageRefresh(refresh,'*');
const editVisible=ref(false),expandedSeasons=ref(new Set<string>());
watch(()=>detail.value?.id,()=>{editVisible.value=false;expandedSeasons.value=new Set()});
function expandSeason(id:string){const next=new Set(expandedSeasons.value);if(next.has(id))next.delete(id);else next.add(id);expandedSeasons.value=next;}
async function metadataSaved(){await load();if(detail.value){try{const updated=await managedDetail(detail.value.id);detail.value={...detail.value,...updated}}catch(e){detailError.value=e}}}
</script>
<template>
  <div class="catalog-workspace">
    <PageHeader title="媒体库" description="浏览已入库的作品，了解影片与剧集内容。"/>
      <form class="catalog-filters catalog-library-filters" @submit.prevent="search">
        <label>
          标题
          <el-input v-model="query" aria-label="搜索媒体标题" placeholder="搜索作品名称" maxlength="200" clearable/>
        </label>
        <label>
          类型
          <el-select popper-class="catalog-options" placeholder="全部类型" v-model="category" aria-label="目录分类筛选" @change="search">
            <el-option label="全部类型" value=""/>
            <el-option v-for="option in categories" :key="option.id" :label="option.name" :value="option.id"/>
          </el-select>
        </label>
      </form>
      <button v-if="parentId" class="catalog-back" @click="browse('')">全部媒体 / 返回目录</button>
      <p v-if="error&&(items.length)" class="f3-stale-warning" role="status">刷新失败，保留上次内容。<button @click="load">重试</button></p>
    <StateView v-if="loading" state="loading" title="正在读取媒体…"/>
      <StateView v-else-if="error&&!(items.length)" :state="sourceState(error)" title="当前来源暂不可用" :description="errorText(error)" @retry="load"/>
      <section v-else class="catalog-section" aria-label="作品展示">
        <header>
          <h2>内置媒体库</h2>
          <span>{{total!=null&&total>items.length?'本页 ':''}}{{loading?'—':displayItems.length}} 部作品</span>
        </header>
        <PosterWall :items="displayItems" @select="inspect"/>
        <StateView v-if="!items.length" state="empty" title="暂无匹配作品" description="可调整标题或类型后再查看。"/>
        <PagePagination v-model:current-page="page" :total="total??undefined" v-model:page-size="pageSize" layout="prev,pager,next" @current-change="load"/>
      </section>
    <DetailDrawer :key="detail?.id" v-model:visible="detailOpen" title="作品详情" size="large" class="catalog-detail">
      <StateView v-if="detailLoading" state="loading" title="正在读取作品详情…"/>
      <StateView v-else-if="detailError" :state="sourceState(detailError)" :description="errorText(detailError)" @retry="detail&&inspect(detail)"/>
      <template v-else-if="detail">
        <div class="catalog-summary" :class="{'catalog-summary--editing':editVisible}">
          <PosterArtwork class="catalog-detail-poster" :src="image(detail.poster)" :title="detail.title"/>
          <div class="catalog-summary-info">
            <div class="catalog-title-row">
              <h2>{{detail.title}}</h2>
              <div v-if="source==='managed'" class="catalog-detail-actions"><el-button @click="editVisible=true">编辑</el-button></div>
            </div>
            <p v-show="!editVisible" class="catalog-subtitle">{{joinMediaFacts([detail.year,mediaType(detail.mediaType)])}}</p>
            <DescriptionGrid v-show="!editVisible" :items="mediaFacts(detail)"/>

          </div>
        </div>
        <div v-show="!editVisible" class="catalog-readonly">
        <section v-if="detail.overview?.trim()&&detail.overview!=='—'" class="catalog-detail-section" aria-label="作品简介">
          <h3>作品简介</h3>
          <p>{{detail.overview||'—'}}</p>
        </section>
        <section class="catalog-detail-section" aria-label="版本与文件">
          <h3>版本与文件</h3>
          <p v-if="series" class="catalog-note">按季展开已入库的集；大小按物理文件合计，不代表整部剧集已收齐。</p>
          <section v-if="series" aria-label="季与集">
            <StateView v-if="childLoading" state="loading"/>
            <StateView v-else-if="childError" :state="sourceState(childError)" :description="errorText(childError)" @retry="readChildren"/>
            <template v-else><div class="catalog-season-list"><div v-for="season in children" :key="season.id" class="catalog-season"><button class="catalog-season-toggle" type="button" :aria-expanded="expandedSeasons.has(season.id)" @click="expandSeason(season.id)"><el-icon class="season-chevron" :class="{'is-expanded':expandedSeasons.has(season.id)}"><ArrowRight/></el-icon><strong>{{seasonLabel(season,series.title)}}</strong><span>{{season.episodeCount??'—'}} 集 · {{displayMediaSize(season.fileSize??season.size)}}</span></button><SeasonEpisodes v-if="expandedSeasons.has(season.id)" :season-id="season.id" @select="inspect($event,true)"/></div></div><StateView v-if="!children.length" state="empty" title="来源尚未返回季集内容"/><PagePagination v-if="childTotal>childSize" v-model:current-page="childPage" :total="childTotal" v-model:page-size="childSize" @current-change="readChildren"/></template>
          </section>
          <div v-else-if="detail.filePath" class="catalog-file-summary"><MiddleEllipsis :path="detail.filePath"/><span v-if="mediaFileFacts(detail)">{{mediaFileFacts(detail)}}</span></div>
          <section v-if="detail.episodeDetails?.length">
            <h4>剧集资料</h4>
            <p v-for="episode in detail.episodeDetails" :key="episode.id">第 {{episode.season}} 季 · 第 {{episode.number}} 集 · {{episode.title}}</p>
          </section>
        </section>
        <SubtitlePanel v-if="source==='managed'&&['movie','series'].includes(detail.mediaType||'')" :key="detail.id" :item-id="detail.id"/>
        </div>
      </template>
    </DetailDrawer>
    <MediaEditDialog v-if="detail" v-model:visible="editVisible" :item="detail" @saved="metadataSaved"/>
  </div>
</template>

<style scoped>
.catalog-summary{grid-template-columns:64px minmax(0,1fr);gap:16px;align-items:start}.catalog-detail-poster{width:64px;height:96px}.catalog-detail-poster :deep(small){display:none}.season-chevron{flex:none;font-size:16px;transition:transform .15s}.season-chevron.is-expanded{transform:rotate(90deg)}

.catalog-library-filters{grid-template-columns:minmax(0,2fr) minmax(0,1fr);align-items:end}.catalog-season-toggle{display:flex;align-items:center;gap:12px;width:100%;min-height:52px;padding:10px 0;border:0;border-top:1px solid var(--mt-border-color);background:none;color:var(--mt-text-primary);font:inherit;text-align:left;cursor:pointer}.catalog-season-toggle strong{flex:1;min-width:0}.catalog-season-toggle>span{font-size:12px;color:var(--mt-text-muted)}.catalog-file-summary{display:grid;gap:8px}.catalog-file-summary>span{font-size:12px;color:var(--mt-text-muted)}
</style>
