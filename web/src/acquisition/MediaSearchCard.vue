<script setup lang="ts">
import {ref} from 'vue';
import type {Media} from './model';
import {tmdbImage} from '../utils/tmdb-image';
const props=defineProps<{item:Media;disabled:boolean;loading:boolean;selected:boolean}>();
const emit=defineEmits<{select:[Media]}>(),broken=ref(false);
function activate(){if(!props.disabled)emit('select',props.item)}
</script>
<template>
 <button type="button" class="media-search-card" :class="{'is-selected':selected}" :disabled="disabled" :aria-label="`搜索《${item.title}》(${item.year||'年份未知'}) 的站点资源`" :aria-pressed="selected" :aria-busy="loading" @click="activate" @keydown.enter.prevent="activate" @keydown.space.prevent="activate">
  <img v-if="tmdbImage(item.posterPath)&&!broken" :src="tmdbImage(item.posterPath)" :alt="item.title+'海报'" @error="broken=true"/>
  <span v-else class="acquire-poster-fallback">暂无海报</span>
  <span class="media-card-copy">
   <strong class="media-card-title">{{item.title}}</strong>
   <span>{{item.mediaType==='series'?'剧集':'电影'}} · {{item.year||'—'}}</span>
   <span>{{item.originalTitle||'—'}}</span>
   <span class="acquire-overview">{{item.overview||'暂无简介'}}</span>
   <span v-if="loading" role="status">正在搜索全部站点…</span>
  </span>
 </button>
</template>
<style scoped>
.media-search-card{display:grid;grid-template-columns:88px minmax(0,1fr);gap:16px;width:100%;min-width:0;min-height:44px;padding:16px;border:1px solid var(--mt-border-color);border-radius:10px;background:var(--mt-bg-surface);color:var(--mt-text-primary);text-align:left;cursor:pointer;font:inherit}
.media-search-card.is-selected{border-color:var(--mt-color-primary)}
.media-search-card:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:3px}
.media-search-card:disabled{cursor:not-allowed;color:var(--mt-text-muted);opacity:1}
.media-card-copy{display:grid;align-content:start;gap:6px;min-width:0;font-size:13px;overflow-wrap:anywhere}
.media-card-title{font-size:16px;font-weight:650;line-height:1.5}
.media-card-copy [role=status]{color:var(--mt-color-primary)}
</style>
