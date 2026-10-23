<script setup lang="ts">
import {tmdbImage} from '../api/images';
const props=defineProps<{identity:any;draft:any;disabled?:boolean;batch?:boolean}>();
const name=(normal:string,bulk:string)=>props.batch?bulk:normal;
</script>
<template>
 <section class="f6-identity" aria-label="TMDB 识别与目录分类" :aria-busy="identity.loading">
  <header><h3>识别作品</h3><span v-if="identity.loading" role="status">正在匹配 TMDB…</span><span v-else-if="draft.tmdbId">TMDB {{draft.tmdbId}}</span></header>
  <div class="f6-identity-selected">
   <img v-if="identity.selected?.posterPath" :src="tmdbImage(identity.selected.posterPath,'w185')" alt="所选作品海报" loading="lazy" width="44" height="66"/>
   <div><strong>{{identity.selected?.title||draft.title||'选择作品身份'}}</strong><p v-if="identity.selected">{{identity.selected.year||'年份未知'}} · {{identity.selected.mediaType==='movie'?'电影':'剧集'}} · 已选中</p><p v-else role="status">{{identity.warning||'识别后可更换作品；未识别时仍可手动填写。'}}</p></div>
   <el-button v-if="draft.tmdbId" :disabled="disabled||identity.loading" @click="identity.manual">手动填写</el-button>
  </div>
  <div class="f6-identity-search">
   <el-input v-model="identity.query" aria-label="搜索 TMDB 作品" placeholder="作品名称，可搜索更换" :disabled="disabled||identity.loading" @keyup.enter="identity.search"/>
   <el-select v-model="identity.queryType" aria-label="TMDB 搜索类型" :disabled="disabled||identity.loading"><el-option label="全部类型" value="auto"/><el-option label="电影" value="movie"/><el-option label="剧集" value="series"/></el-select>
   <el-button :disabled="disabled||identity.loading||!identity.query.trim()" @click="identity.search">搜索 TMDB</el-button>
  </div>
  <div class="f6-identity-candidates" aria-label="TMDB 候选">
   <button v-for="item in identity.matches" :key="item.mediaType+':'+item.id" type="button" :aria-pressed="draft.tmdbId===item.id&&draft.mediaType===item.mediaType" :disabled="disabled||identity.loading" @click="identity.choose(item)">
    <img v-if="item.posterPath" :src="tmdbImage(item.posterPath,'w185')" alt="" loading="lazy" width="32" height="48"/><span><strong>{{item.title}}</strong><small>{{item.year||'年份未知'}} · {{item.mediaType==='movie'?'电影':'剧集'}} · {{item.id}}</small></span>
   </button>
   <p v-if="!identity.matches.length">{{identity.loading?'正在读取候选…':'暂无候选，可搜索或手动填写。'}}</p>
  </div>
  <div class="f6-identity-fields">
   <label>标题<el-input v-model="draft.title" :aria-label="name('媒体标题','批次文件标题')" :readonly="draft.tmdbId>0" :disabled="disabled||identity.loading"/></label>
   <label>年份<el-input v-model="draft.year" :aria-label="name('媒体年份','批次年份')" :readonly="draft.tmdbId>0" :disabled="disabled||identity.loading"/></label>
   <label>媒体类型<el-select v-model="draft.mediaType" :aria-label="name('媒体类型','批次媒体类型')" :disabled="disabled||identity.loading||draft.tmdbId>0"><el-option value="unknown" label="未识别"/><el-option value="movie" label="电影"/><el-option value="series" label="剧集"/></el-select></label>
   <label>目录分类<el-select v-model="draft.category" :aria-label="name('目录分类','批次目录分类')" placeholder="请选择已配置分类" :disabled="disabled||identity.loading"><el-option v-for="choice in identity.choices" :key="choice.directory" :label="choice.name" :value="choice.directory"/></el-select></label>
  </div>
  <div class="f6-identity-episodes"><label>分辨率<el-input v-model="draft.resolution" :aria-label="name('分辨率','批次分辨率')" :disabled="disabled||identity.loading"/></label><template v-if="draft.mediaType==='series'">
   <label>季号<el-input-number v-model="draft.season" :aria-label="name('季号','批次季号')" :min="0" :max="999" :disabled="disabled||identity.loading" placeholder="从文件名解析或填写"/></label>
   <label>集号<el-input-number v-model="draft.episode" :aria-label="name('集号','批次集号')" :min="1" :max="9999" :disabled="disabled||identity.loading" placeholder="从文件名解析或填写"/></label>
  </template></div>
 </section>
</template>
<style scoped>
.f6-identity{min-width:0}.f6-identity header{display:flex;align-items:center;justify-content:space-between;gap:12px}.f6-identity-selected{display:flex;align-items:center;gap:12px;min-height:82px}.f6-identity-selected>div{flex:1;min-width:0}.f6-identity p{font-size:13px;color:var(--mt-text-secondary);margin:6px 0}.f6-identity img{object-fit:cover;flex:none;border-radius:4px;background:var(--mt-bg-page)}.f6-identity-search{display:grid;grid-template-columns:minmax(120px,1fr) 112px auto;gap:8px;margin:12px 0}.f6-identity-candidates{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;max-height:176px;min-height:64px;overflow:auto;margin-bottom:16px}.f6-identity-candidates button{display:flex;align-items:center;gap:10px;text-align:left;padding:8px;border:1px solid var(--mt-border-color);border-radius:6px;background:var(--mt-bg-surface);color:var(--mt-text-primary);font:inherit;cursor:pointer;min-width:0}.f6-identity-candidates button[aria-pressed=true]{border-color:var(--mt-color-primary)}.f6-identity-candidates small{display:block;color:var(--mt-text-secondary)}.f6-identity-fields,.f6-identity-episodes{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.f6-identity-episodes{min-height:78px;margin-top:12px}.f6-identity label{display:grid;grid-template-columns:minmax(0,1fr);gap:6px;min-width:0}.f6-identity strong,.f6-identity-candidates span{min-width:0;overflow-wrap:anywhere}@media(max-width:560px){.f6-identity-search{grid-template-columns:minmax(0,1fr) 112px}.f6-identity-search>button{grid-column:1/-1}.f6-identity-candidates{grid-template-columns:1fr}.f6-identity-fields,.f6-identity-episodes{grid-template-columns:1fr}.f6-identity-selected{flex-wrap:wrap}}
</style>
