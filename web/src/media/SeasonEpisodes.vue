<script setup lang="ts">
import {onMounted,reactive,ref} from 'vue';
import {managedCatalog} from '../api/mediaCatalog';
import {useResource} from '../workflows/useResource';
import ResourceState from '../workflows/ResourceState.vue';
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
import {displayMediaSize,type MediaItem} from './model';
const props=defineProps<{seasonId:string}>();const emit=defineEmits<{select:[MediaItem]}>();
const page=ref(1),pageSize=ref(25);
const data=reactive(useResource(()=>managedCatalog('',props.seasonId,'',page.value,pageSize.value),{items:[],total:0,page:1,pageSize:25}));
onMounted(data.load);
</script>
<template><div class="season-episode-list"><ResourceState :loading="data.loading" :error="data.error" :stale="data.loaded" @reload="data.load"/><p v-if="data.loaded&&!data.error&&!data.data.items.length">此季暂无已入库的集。</p><button v-for="episode in data.data.items" :key="episode.id" type="button" class="season-episode" @click="emit('select',episode)"><span>第 {{episode.episode??'—'}} 集</span><strong><MiddleEllipsis :text="episode.title" :copy="false" embedded/></strong><span>{{episode.resolution||'清晰度未探测'}} · {{displayMediaSize(episode.fileSize??episode.size)}}</span></button><PagePagination v-if="data.data.total>pageSize" v-model:current-page="page" v-model:page-size="pageSize" :total="data.data.total" label="单季已入库文件分页" @current-change="data.load"/></div></template>
<style scoped>.season-episode-list{padding:8px 0 8px 16px}.season-episode{display:grid;grid-template-columns:72px minmax(0,1fr) auto;gap:12px;align-items:center;width:100%;min-height:44px;border:0;border-top:1px solid var(--mt-border-color);background:none;text-align:left;color:var(--mt-text-primary);font:inherit;cursor:pointer}.season-episode>span{font-size:12px;color:var(--mt-text-muted)}.season-episode strong{min-width:0;font-size:13px}.season-episode-list p{font-size:12px;color:var(--mt-text-muted)}@media(max-width:600px){.season-episode{grid-template-columns:64px minmax(0,1fr)}.season-episode>span:last-child{grid-column:2;padding-bottom:8px}}</style>
