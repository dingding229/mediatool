<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { Refresh } from '@element-plus/icons-vue';
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import PosterWall from '../media/PosterWall.vue';
import { useCatalog } from '../media/useCatalog';
import { image, originalSTRM, mediaType, sourceState } from '../media/model';
import { errorText } from '../workflows/readiness';
import { formatBytes } from '../operations/model';
import '../media/catalog.css';
const { selectedSeason, chooseSeason, displayItems, source, view, page, query, kind, parentId, items, total, home, groups, loading, error, embyConnected, connectionError, detail, detailOpen, detailLoading, detailError, children, childTotal, childPage, childError, childLoading, episodes, series, copyMessage, playLink, copyBusy, load, search, chooseSource, all, browse, inspect, homeItem, refresh, readChildren, copyLink } = useCatalog();
usePageRefresh(refresh,'*');
</script>
<template>
  <div class="catalog-workspace">
    <PageHeader title="媒体库" description="浏览已入库的作品，了解影片与剧集内容。"/>
    <section class="catalog-toolbar">
      <nav aria-label="媒体库来源" class="mt-segments">
        <button :aria-pressed="source==='managed'" @click="chooseSource('managed')">内置媒体库</button>
        <button :aria-pressed="source==='emby'" aria-label="Emby" aria-describedby="emby-connection-state" @click="chooseSource('emby')">
          Emby
          <small id="emby-connection-state">{{embyConnected===false?'未连接':embyConnected===null?'状态未确认':''}}</small>
        </button>
      </nav>

    </section>
    <p v-if="source==='emby'&&embyConnected===false" class="catalog-note">Emby 未连接。需要浏览时，请在“连接”页配置。</p>
    <p v-else-if="source==='emby'&&connectionError" class="catalog-note">连接状态暂未确认；以下为本次媒体读取结果。</p>
    <template v-if="source!=='emby'||embyConnected!==false">
      <h2 v-if="source==='emby'&&view==='home'">Emby 媒体库</h2>
      <nav v-if="source==='emby'" class="catalog-subnav mt-segments" aria-label="Emby 浏览范围">
        <button :aria-pressed="view==='home'" @click="chooseSource('emby')">Emby 首页</button>
        <button :aria-pressed="view==='all'" @click="all">全部媒体</button>
      </nav>
      <form v-if="source==='managed'||view==='all'" class="catalog-filters" @submit.prevent="search">
        <label>
          标题
          <el-input v-model="query" aria-label="搜索媒体标题" maxlength="200" clearable/>
        </label>
        <label>
          类型
          <el-select popper-class="catalog-options" placeholder="全部类型" v-model="kind" aria-label="媒体类型筛选" @change="search">
            <el-option label="全部类型" value=""/>
            <el-option label="电影" value="movie"/>
            <el-option label="电视剧" value="series"/>
          </el-select>
        </label>
        <el-button native-type="submit">筛选</el-button>
      </form>
      <button v-if="parentId" class="catalog-back" @click="browse('')">全部媒体 / 返回目录</button>
      <StateView v-if="loading" state="loading" title="正在读取媒体…"/>
      <StateView v-else-if="error" :state="sourceState(error)" title="当前来源暂不可用" :description="errorText(error)" @retry="load"/>
      <template v-else-if="source==='emby'&&view==='home'&&home">
        <section v-for="group in groups" :key="group.key" class="catalog-section">
          <header>
            <h2>{{group.title}}</h2>
            <span>{{group.items.length}} 项</span>
          </header>
          <PosterWall :items="group.items" @select="item=>homeItem(group.key,item)"/>
          <StateView v-if="!group.items.length" state="empty" title="暂无内容"/>
        </section>
      </template>
      <section v-else class="catalog-section" aria-label="作品展示">
        <header>
          <h2>{{source==='managed'?'内置媒体库':'Emby 媒体库'}}</h2>
          <span>{{total!=null&&total>items.length?'本页 ':''}}{{loading?'—':displayItems.length}} 部作品</span>
        </header>
        <PosterWall :items="displayItems" @select="inspect"/>
        <StateView v-if="!items.length" state="empty" title="暂无匹配作品" description="可调整标题或类型后再查看。"/>
        <PagePagination v-if="total" v-model:current-page="page" :total="total" :page-size="24" layout="prev,pager,next" @current-change="load"/>
      </section>
    </template>
    <DetailDrawer :key="detail?.id" v-model:visible="detailOpen" :title="detail?.title||'作品详情'" class="catalog-detail">
      <StateView v-if="detailLoading" state="loading" title="正在读取作品详情…"/>
      <StateView v-if="detailError" :state="sourceState(detailError)" :description="errorText(detailError)" @retry="detail&&inspect(detail)"/>
      <div class="catalog-summary">
        <img v-if="image(detail?.poster)" :src="image(detail?.poster)" :alt="detail?.title+'海报'"/>
        <div>
          <p>{{detail?.year||'—'}} · {{mediaType(detail?.mediaType)}}</p>
          <dl>
            <div>
              <dt>评分</dt>
              <dd>{{detail?.rating??'—'}}</dd>
            </div>
            <div>
              <dt>文件大小</dt>
              <dd>{{formatBytes(detail?.size)}}</dd>
            </div>
            <div>
              <dt>清晰度</dt>
              <dd>{{detail?.resolution||'—'}}</dd>
            </div>
            <div>
              <dt>类型</dt>
              <dd>{{detail?.genres?.join('、')||'—'}}</dd>
            </div>
          </dl>
        </div>
      </div>
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
          <PagePagination v-if="childTotal>24" v-model:current-page="childPage" :total="childTotal" :page-size="24" layout="prev,pager,next" @current-change="readChildren"/>
        </template>
      </section>
      <section v-if="detail?.episodeDetails?.length">
        <h3>剧集资料</h3>
        <p v-for="episode in detail.episodeDetails" :key="episode.id">第 {{episode.season}} 季 · 第 {{episode.number}} 集 · {{episode.title}}</p>
      </section>
      <p v-if="source==='managed'&&detail?.mediaType==='series'">内置媒体库按作品、季、集分层读取，仅反映已入库内容，不代表整部剧集已收齐。</p>
      <div v-if="!detail?.groupFiles&&originalSTRM(detail)" class="catalog-link">
        <el-button :loading="copyBusy" @click="copyLink(originalSTRM(detail))">复制播放链接</el-button>
        <p>仅复制给兼容播放器使用，本页不播放。链接含此文件的访问权限，请勿公开分享。</p>
        <p v-if="copyMessage" role="status">{{copyMessage}}</p>
        <el-input v-if="playLink&&copyMessage.includes('失败')" :model-value="playLink" readonly type="textarea" aria-label="待复制播放链接"/>
      </div>
    </DetailDrawer>
  </div>
</template>
