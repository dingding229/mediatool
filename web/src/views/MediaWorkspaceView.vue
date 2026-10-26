<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { Refresh } from '@element-plus/icons-vue';
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import PosterWall from '../media/PosterWall.vue';
import SubtitlePanel from '../media/SubtitlePanel.vue';
import MetadataEditor from '../media/MetadataEditor.vue';
import ImageEditor from '../media/ImageEditor.vue';
import {managedDetail} from '../api/mediaCatalog';
import { useCatalog } from '../media/useCatalog';
import { image, mediaType, sourceState, displayRating, displayResolution, displayMediaSize } from '../media/model';
import { errorText } from '../workflows/readiness';
import { formatBytes } from '../operations/model';
import '../media/catalog.css';
const { selectedSeason, chooseSeason, displayItems, source, view, page, query, kind, parentId, items, total, loading, error, detail, detailOpen, detailLoading, detailError, children, childTotal, childPage, childError, childLoading, episodes, series, category, categories, load, search, all, browse, inspect, refresh, readChildren } = useCatalog();
usePageRefresh(refresh,'*');
async function metadataSaved(){await load();if(detail.value){try {const updated=await managedDetail(detail.value.id);detail.value={...detail.value,...updated};}catch(e){detailError.value=e;}}}
</script>
<template>
  <div class="catalog-workspace">
    <PageHeader title="媒体库" description="浏览已入库的作品，了解影片与剧集内容。"/>
      <form class="catalog-filters" @submit.prevent="search">
        <label>
          标题
          <el-input v-model="query" aria-label="搜索媒体标题" maxlength="200" clearable/>
        </label>
        <label>
          类型
          <el-select popper-class="catalog-options" placeholder="全部类型" v-model="category" aria-label="目录分类筛选" @change="search">
            <el-option label="全部类型" value=""/>
            <el-option v-for="option in categories" :key="option.id" :label="option.name" :value="option.id"/>
          </el-select>
        </label>
        <el-button native-type="submit">筛选</el-button>
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
        <PagePagination v-model:current-page="page" :total="total??undefined" :page-size="24" layout="prev,pager,next" @current-change="load"/>
      </section>
    <DetailDrawer :key="detail?.id" v-model:visible="detailOpen" :title="detail?.title||'作品详情'" class="catalog-detail">
      <StateView v-if="detailLoading" state="loading" title="正在读取作品详情…"/>
      <StateView v-if="detailError" :state="sourceState(detailError)" :description="errorText(detailError)" @retry="detail&&inspect(detail)"/>
      <MetadataEditor v-if="source==='managed'&&detail&&!detailLoading&&!detailError" :item-id="detail.id" @saved="metadataSaved"/>
 <ImageEditor v-if="source==='managed'&&detail&&!detailLoading&&!detailError" :item-id="detail.id" @saved="metadataSaved"/>
      <div class="catalog-summary" :class="{'catalog-summary--text':!image(detail?.poster)}">
        <img v-if="image(detail?.poster)" :src="image(detail?.poster)" :alt="detail?.title+'海报'"/>
        <div>
          <p>{{detail?.year||'—'}} · {{mediaType(detail?.mediaType)}}</p>
          <dl>
            <div>
              <dt>评分</dt>
              <dd>{{source==='managed'?displayRating(detail?.rating):detail?.rating??'—'}}</dd>
            </div>
            <div>
              <dt>文件大小</dt>
              <dd>{{source==='managed'?displayMediaSize(detail?.size):formatBytes(detail?.size)}}</dd>
            </div>
            <div>
              <dt>清晰度</dt>
              <dd>{{source==='managed'?displayResolution(detail?.resolutions):detail?.resolution||'—'}}</dd>
            </div>
            <div>
              <dt>类型</dt>
              <dd>{{source==='managed'?detail?.category||'未分类':detail?.genres?.join('、')||'—'}}</dd>
            </div>
          </dl>
        </div>
      </div>
      <p v-if="source==='managed'&&detail?.aggregate" class="catalog-note">文件大小为已入库的 {{detail.assetCount}} 个物理文件合计；清晰度为已有探测结果汇总，不代表整部剧集已收齐。</p>
      <h3>作品简介</h3>
      <p>{{detail?.overview||'—'}}</p>
      <section v-if="series" aria-label="季与集">
        <h3>{{series.title}} · 季与集</h3>
        <StateView v-if="childLoading" state="loading"/>
        <StateView v-else-if="childError" :state="sourceState(childError)" :description="errorText(childError)" @retry="readChildren"/>
        <template v-else>
          <p>{{selectedSeason ? selectedSeason.title + ' · 集列表' : '季列表'}} · 共 {{childTotal}} 项，按季号、集号排序。</p>
          <el-button v-if="selectedSeason" plain @click="chooseSeason(null)">返回季列表</el-button>
          <ol class="catalog-episodes">
            <li v-for="episode in episodes" :key="episode.id">
              <button @click="episode.type==='Season' ? chooseSeason(episode) : inspect(episode,true)">{{episode.season!=null?'第 '+episode.season+' 季 · ':''}}{{episode.episode!=null?'第 '+episode.episode+' 集 · ':''}}{{episode.title}}</button>
            </li>
          </ol>
          <StateView v-if="!children.length" state="empty" title="来源尚未返回季集内容"/>
          <PagePagination v-model:current-page="childPage" :total="childTotal" :page-size="24" layout="prev,pager,next" @current-change="readChildren"/>
        </template>
      </section>
      <section v-if="detail?.episodeDetails?.length">
        <h3>剧集资料</h3>
        <p v-for="episode in detail.episodeDetails" :key="episode.id">第 {{episode.season}} 季 · 第 {{episode.number}} 集 · {{episode.title}}</p>
      </section>
      <p v-if="source==='managed'&&detail?.mediaType==='series'">内置媒体库按作品、季、集分层读取，仅反映已入库内容，不代表整部剧集已收齐。</p>
      <SubtitlePanel v-if="source==='managed'&&detail&&!['Series','Season'].includes(detail.type||'')&&['movie','series'].includes(detail.mediaType||'')" :key="detail.id" :asset-id="detail.id"/>
    </DetailDrawer>
  </div>
</template>
