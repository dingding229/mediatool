<script setup lang="ts">
import {computed,ref} from 'vue';
import {Sort,Top,Bottom,CircleCheck,CircleClose,Loading} from '@element-plus/icons-vue';
import {providePageRefresh,usePageRefresh} from '../ui/page-refresh';
const ownRefresh=providePageRefresh();
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import MediaContext from '../acquisition/MediaContext.vue';
import DownloadSubmission from '../acquisition/DownloadSubmission.vue';
import ResourcePromotion from '../acquisition/ResourcePromotion.vue';
import { useResults } from '../acquisition/useResults';
import { sourceLabel, resourceSources } from '../utils/resource-source';
import { size, label } from '../acquisition/model';
import { errorText, stateFor } from '../workflows/readiness';
import '../acquisition/acquisition.css';
const { pageSize, run, loading, error, mediaSaving, mediaReady, page, filters, activeMedia, delivery, busy, searchIntent, refresh, applyFilter, queueFilter, sort, persistMedia, openDownload, searchAgain } = useResults();
const sitesOpen=ref(false);
const siteCounts=computed(()=>({success:run.value?.sites.filter(s=>s.state==='succeeded').length||0,failed:run.value?.sites.filter(s=>['failed','partial'].includes(s.state)).length||0,running:run.value?.sites.filter(s=>['pending','running'].includes(s.state)).length||0}));
const direction=(key:string)=>filters.sort.startsWith(key+'_')?(filters.sort.endsWith('_asc')?'ascending':'descending'):'none';
const sortIcon=(key:string)=>direction(key)==='ascending'?Top:direction(key)==='descending'?Bottom:Sort;
const failedSite=(state:string)=>['failed','partial'].includes(state);
const sortedSites=computed(()=>[...(run.value?.sites||[])].sort((a,b)=>Number(failedSite(b.state))-Number(failedSite(a.state))));
ownRefresh.register(()=>refresh(),()=>true);
</script>
<template>
  <div class="acquire-workspace">
    <PageHeader title="搜索结果" :description="run?`“${run.keyword}” · 先看站点覆盖，再择优投递。`:'正在读取搜索记录…'">
      <template #actions>
        <RouterLink to="/search?tab=query">返回搜索</RouterLink>

        <el-button type="primary" :disabled="!run||!!searchIntent" :loading="busy" @click="searchAgain">重新搜索</el-button>
      </template>
    </PageHeader>
    <StateView v-if="error" :state="stateFor(error)" title="搜索记录读取失败" :description="errorText(error)" @retry="refresh()"/>
    <StateView v-else-if="loading&&!run" state="loading" title="正在读取资源快照…"/>
    <AsyncResultBanner v-if="searchIntent" :status="searchIntent.phase" message="搜索请求已提交，结果尚未确认；请先核对记录。" @refresh="refresh()"/>
    <p v-if="!mediaReady&&!mediaSaving" role="alert">媒体身份尚未核对，请刷新结果后再操作。</p><template v-if="run">
      <MediaContext :key="run.id" :keyword="run.keyword" :initial="activeMedia" :disabled="mediaSaving||!mediaReady" @selected="persistMedia"/>
      <section class="acquire-surface f3-site-summary">
        <button class="f3-summary-toggle" :aria-expanded="sitesOpen" @click="sitesOpen=!sitesOpen">站点查询状态 · 成功 {{siteCounts.success}} / 失败 {{siteCounts.failed}} / 进行中 {{siteCounts.running}} <span>{{sitesOpen?'收起':'查看各站点'}} {{sitesOpen?'▴':'▾'}}</span></button>
        <div v-show="sitesOpen" class="acquire-site-status f3-site-details f4-site-grid" role="list" aria-label="各站点查询结果">
          <ElTooltip v-for="site in sortedSites" :key="site.accountId" :content="site.message" :disabled="!site.message" placement="top">
            <article role="listitem" tabindex="0" :data-state="site.state" :aria-label="site.name+' · '+label(site.state)+' · '+(site.count??'—')">
              <CircleClose v-if="failedSite(site.state)" class="f4-site-icon is-failed" aria-hidden="true"/><CircleCheck v-else-if="site.state==='succeeded'" class="f4-site-icon is-success" aria-hidden="true"/><Loading v-else class="f4-site-icon" aria-hidden="true"/>
              <strong>{{site.name}}</strong><span>{{label(site.state)}} · {{site.count??'—'}}</span>
            </article>
          </ElTooltip>
        </div>
        <p v-if="run.state==='partial'">已取得部分结果；失败站点不代表没有资源。</p>
      </section>
      <section class="acquire-surface">
        <header class="acquire-section-heading">
          <h2>择优选择资源 <small>{{run.total??run.resources.length}} 条匹配</small></h2>
          <el-select v-model="filters.sort" aria-label="择优排序" @change="applyFilter">
            <el-option value="seeders_desc" label="做种人数优先"/><el-option value="seeders_asc" label="做种人数从少到多"/>
            <el-option value="promotion_asc" label="促销优先"/><el-option value="promotion_desc" label="促销从高到低"/>
            <el-option value="size_asc" label="体积从小到大"/>
            <el-option value="size_desc" label="体积从大到小"/>
          </el-select>
        </header>
        <div class="acquire-filters">
          <label>
            分辨率
            <el-select v-model="filters.resolution" multiple clearable aria-label="资源分辨率" @change="applyFilter">
              <el-option v-for="r in ['480p','720p','1080p','2160p','4320p','unknown']" :key="r" :value="r" :label="r==='unknown'?'未知':r"/>
            </el-select>
          </label>
          <label>
            来源
            <el-select v-model="filters.source" multiple clearable aria-label="资源来源" @change="applyFilter">
              <el-option v-for="s in resourceSources" :key="s.value" :value="s.value" :label="s.label"/>
            </el-select>
          </label>
          <label>
            编码
            <el-select v-model="filters.codec" multiple clearable aria-label="资源编码" @change="applyFilter">
              <el-option v-for="c in ['h264','h265','av1','unknown']" :key="c" :value="c" :label="c==='unknown'?'未知':c.toUpperCase()"/>
            </el-select>
          </label>
          <label>
            站点
            <el-select v-model="filters.accountId" multiple clearable aria-label="结果站点" @change="applyFilter">
              <el-option v-for="site in run.sites" :key="site.accountId" :value="site.accountId" :label="site.name"/>
            </el-select>
          </label>
          <label>
            关键字筛选
            <el-input v-model="filters.keyword" aria-label="资源关键字筛选" clearable @input="queueFilter" @keyup.enter="applyFilter"/>
          </label>
        </div>
        <p>筛选整个已取得快照；标题与编码仅为线索，投递前仍需核对媒体身份及 H&amp;R 要求。</p>
        <div class="f3-stable-scroll resource-results-table">
        <div class="resource-columns">
          <span>资源与站点</span>
          <span role="columnheader" tabindex="0" :aria-sort="direction('size')" aria-label="按文件大小排序" @click="sort('size')" @keydown.enter="sort('size')" @keydown.space.prevent="sort('size')"><span class="resource-sort-label">大小</span><component :is="sortIcon('size')" class="f4-sort-icon" aria-hidden="true"/></span>
          <span role="columnheader" tabindex="0" :aria-sort="direction('seeders')" aria-label="按做种人数排序" @click="sort('seeders')" @keydown.enter="sort('seeders')" @keydown.space.prevent="sort('seeders')"><span class="resource-sort-label">做种人数</span><component :is="sortIcon('seeders')" class="f4-sort-icon" aria-hidden="true"/></span>
          <span role="columnheader" tabindex="0" :aria-sort="direction('promotion')" aria-label="按促销排序" @click="sort('promotion')" @keydown.enter="sort('promotion')" @keydown.space.prevent="sort('promotion')"><span class="resource-sort-label">促销</span><component :is="sortIcon('promotion')" class="f4-sort-icon" aria-hidden="true"/></span>
          <span>操作</span>
        </div>
        <article v-for="item in run.resources" :key="item.id" class="resource-entry">
          <div>
            <h3>{{ item.title }}</h3>
            <p>{{ item.metadata?.subtitle||'—' }}</p>
            <p>{{ item.siteName||'—' }} · {{ item.metadata?.attributes?.codec?.toUpperCase()||'编码未知' }} · {{ sourceLabel(item.metadata?.attributes?.source) }} · {{ item.metadata?.attributes?.resolution||'分辨率未知' }}</p>
            <p v-if="item.metadata?.yearWarning" role="note">{{ item.metadata.yearWarning }}</p>
          </div>
          <span class="resource-stat">{{ size(item.size) }}</span>
          <span class="resource-stat">{{ item.seeders??'—' }}</span>
          <ResourcePromotion class="resource-stat" :factor="item.downloadFactor"/>
          <el-button :disabled="mediaSaving||!mediaReady" @click="openDownload(item)">确认并下载</el-button>
        </article>
        <StateView v-if="!run.resources.length" :state="['pending','running'].includes(run.state)?'loading':'empty'" :title="['pending','running'].includes(run.state)?'站点正在查询…':'没有匹配资源'" description="若有站点失败或部分结果，请先核对上方原因。"/>
        </div>
        <PagePagination v-model:current-page="page" v-model:page-size="pageSize" :total="run.total||0" layout="prev,pager,next" @current-change="refresh()"/>
      </section>
    </template>
    <DownloadSubmission :model="delivery"/>
  </div>
</template>
