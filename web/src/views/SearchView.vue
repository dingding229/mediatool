<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {useRouter,useRoute} from 'vue-router'
import {Search,Film} from '@element-plus/icons-vue'
import {request,post} from '../api/client'
import RecentSearches from '../components/RecentSearches.vue'
import {tmdbImage} from '../utils/tmdb-image'
interface Media{id:number;mediaType:string;title:string;year:string;originalTitle?:string;overview?:string;posterPath?:string}
interface History{id:string;keyword:string;state:string;total:number;media?:Media}
const router=useRouter(),route=useRoute(),mode=ref('media'),query=ref(''),kind=ref('auto'),year=ref(''),keyword=ref(''),media=ref<Media[]>([]),history=ref<History[]>([]),siteCount=ref(0),busy=ref(false),searching=ref(false),error=ref(''),searched=ref(false),cached=ref(false),posterErrors=ref(new Set<string>())
const ready=computed(()=>siteCount.value>0)
async function load(){try{const [accounts,runs]=await Promise.all([request<Array<{kind:string;enabled:boolean}>>('/integrations'),request<History[]>('/search')]);siteCount.value=accounts.filter(a=>a.kind==='ptsite'&&a.enabled).length;history.value=runs}catch(e){error.value=e instanceof Error?e.message:String(e)}}
async function findMedia(refresh=false){if(!query.value.trim()||searching.value)return;searching.value=true;error.value='';searched.value=false;try{const p=new URLSearchParams({q:query.value.trim(),mediaType:kind.value,year:year.value,refresh:String(refresh)});const out=await request<{items:Media[];cached:boolean}>(`/search/media?${p}`);media.value=out.items;cached.value=out.cached;searched.value=true}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{searching.value=false}}
async function start(value:string,selected?:Media,refresh=false){if(!value.trim()||busy.value||!ready.value)return;busy.value=true;error.value='';try{const out=await post<{id:string}>('/search',{keyword:value.trim(),refresh,media:selected?{id:selected.id,mediaType:selected.mediaType}:undefined});await router.push(`/search/results/${out.id}`)}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
const viewResult=(id:string)=>router.push(`/search/results/${id}`)
onMounted(()=>{load();if(typeof route.query.run==='string')router.replace(`/search/results/${route.query.run}`)})
</script>
<template>
<section class="page-heading"><div><span class="page-kicker">ACQUIRE / 资源搜索</span><h1>资源搜索</h1><p>发现媒体，查询全部启用站点；结果在独立页面查看。</p></div></section>
<el-alert v-if="error" :title="error" type="error" :closable="false"/>
<div class="search-mode-tabs"><el-radio-group v-model="mode" aria-label="搜索方式"><el-radio-button value="media">TMDB 媒体搜索</el-radio-button><el-radio-button value="site">站点直搜</el-radio-button></el-radio-group></div>
<section v-if="mode==='media'" class="panel">
<div class="panel-title"><h2>先找到你想看的作品</h2><span class="muted">TMDB 元数据</span></div>
<form class="tmdb-search" @submit.prevent="findMedia(false)"><label class="search-field" for="tmdb-media-query">媒体名称<el-input id="tmdb-media-query" v-model="query" aria-label="TMDB媒体名称" :prefix-icon="Search" placeholder="电影或剧集名称" clearable/></label><label class="search-field" for="tmdb-media-kind">类型<el-select id="tmdb-media-kind" v-model="kind" aria-label="TMDB媒体类型"><el-option label="自动识别" value="auto"/><el-option label="电影" value="movie"/><el-option label="剧集" value="series"/></el-select></label><label class="search-field" for="tmdb-media-year">年份（可选）<el-input id="tmdb-media-year" v-model="year" aria-label="TMDB年份" placeholder="例如 2024" :maxlength="4" inputmode="numeric"/></label><div class="search-submit-row"><el-button type="primary" native-type="submit" :loading="searching" :disabled="!query.trim()">搜索媒体</el-button><el-button :disabled="searching||!query.trim()" @click="findMedia(true)">忽略TMDB缓存</el-button></div></form>
<p class="field-hint">{{cached?'本次使用TMDB缓存':'TMDB查询与PT资源搜索独立，不会自动下载'}}。点击作品标题查询资源，进入独立结果页面。</p>
<el-skeleton v-if="searching" :rows="4" animated/>
<template v-else-if="searched"><div class="search-results-heading"><h2>匹配作品</h2><span>{{media.length}} 项</span></div><div class="tmdb-results"><article v-for="item in media" :key="item.mediaType+item.id" class="tmdb-media"><div class="tmdb-poster"><img v-if="tmdbImage(item.posterPath)&&!posterErrors.has(item.mediaType+item.id)" :src="tmdbImage(item.posterPath)" :alt="item.title" loading="lazy" @error="posterErrors.add(item.mediaType+item.id)"/><Film v-else aria-label="暂无海报"/></div><div><h2><button class="media-title" type="button" :disabled="busy||!ready" @click="start(item.title,item)">{{item.title}}</button><small>{{item.year||'年份未知'}} · {{item.mediaType==='movie'?'电影':'剧集'}}</small></h2><p>{{item.originalTitle}}</p><p class="field-hint">{{item.overview||'暂无简介'}}</p></div></article></div><el-empty v-if="!media.length" description="没有找到匹配作品，请换个名称或调整年份。"/></template>
<RecentSearches v-else :items="history" @select="viewResult"/>
</section>
<template v-else><section class="panel"><form class="search-form" @submit.prevent="start(keyword)"><el-input v-model="keyword" aria-label="资源关键词" placeholder="电影名称、剧集名称或发布关键词" size="large"/><el-button type="primary" native-type="submit" :loading="busy" :disabled="!keyword.trim()||!ready">开始搜索</el-button></form><p class="field-hint">默认搜索全部 {{siteCount}} 个已启用 PT 站点，错误与挑战会如实显示。</p><el-button :disabled="busy||!keyword.trim()||!ready" @click="start(keyword,undefined,true)">忽略站点缓存重新搜索</el-button></section><section class="panel section-gap"><RecentSearches :items="history" @select="viewResult"/></section></template>
</template>
<style scoped>
.media-title{background:none;border:0;padding:0;text-align:left;font:inherit;font-weight:700;color:var(--primary);cursor:pointer;min-height:44px}.media-title:hover{text-decoration:underline}.media-title:disabled{color:var(--muted);cursor:wait}.tmdb-media{background:none;padding:16px;border:1px solid var(--border);border-radius:12px}.tmdb-poster{width:88px;height:132px}.tmdb-media h2{margin-top:0}@media(max-width:600px){.tmdb-poster{width:64px;height:96px}.tmdb-media{padding:12px}}
</style>
