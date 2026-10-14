<script setup lang="ts">
import {ref,onMounted} from 'vue'
import {request,post} from '../api/client'
import StatusBadge from './StatusBadge.vue'
interface Asset{id:string;title:string;year:string;mediaType:string;source:string;cloudState:string;artifactsState:string;revision:number;revoked:boolean;outputs:{path:string;kind:string;owner:string}[];embyState:string;embyPaths:{path:string;visible:boolean;reason:string}[]}
const items=ref<Asset[]>([]),total=ref(0),page=ref(1),query=ref(''),busy=ref(false),error=ref(''),detail=ref<Asset|null>(null),dialog=ref(false),refreshing=ref('')
const content=ref<{path:string;kind:string;owner:string;digest:string;content:string}|null>(null),contentBusy=ref(false)
let epoch=0
async function load(){const serial=++epoch;busy.value=true;error.value='';try{const out=await request<{items:Asset[];total:number}>(`/library/outputs?page=${page.value}&pageSize=25&q=${encodeURIComponent(query.value)}`);if(serial===epoch){items.value=out.items;total.value=out.total}}catch(e){if(serial===epoch)error.value=e instanceof Error?e.message:'托管输出读取失败'}finally{if(serial===epoch)busy.value=false}}
function inspect(item:Asset){detail.value=item;content.value=null;dialog.value=true}
async function readContent(path:string){if(!detail.value)return;contentBusy.value=true;error.value='';try{content.value=await request(`/library/outputs/${detail.value.id}/content?path=${encodeURIComponent(path)}`)}catch(e){error.value=e instanceof Error?e.message:'输出内容读取失败'}finally{contentBusy.value=false}}
async function refreshEmby(item:Asset){refreshing.value=item.id;error.value='';try{await post(`/library/outputs/${item.id}/emby`,{});error.value='';await load()}catch(e){error.value=e instanceof Error?e.message:'Emby刷新排队失败'}finally{refreshing.value=''}}
onMounted(load)
</script>
<template>
  <section class="panel output-workbench"><div class="panel-title"><h2>托管 STRM / NFO 集合</h2><el-button :loading="busy" @click="load">刷新输出</el-button></div>
    <el-alert v-if="error" :title="error" type="error" :closable="false"/>
    <div class="output-filter"><el-input v-model="query" aria-label="托管输出关键词" placeholder="媒体标题或原文件路径" clearable @keyup.enter="page=1;load()"/><el-button @click="page=1;load()">筛选</el-button></div>
    <p class="field-hint">仅列出本程序拥有的输出。修复 / 布局迁移 / 归档使用原有批准流程；不会覆盖手工输出，也不会更换原固定播放链接。</p>
    <article v-for="item in items" :key="item.id" class="output-card">
      <div class="output-head"><strong>{{item.title||'未确认媒体'}} {{item.year?`(${item.year})`:''}}</strong><StatusBadge :state="item.artifactsState" :label="item.artifactsState==='ready'?'托管输出就绪':'输出待恢复'"/><StatusBadge :state="item.embyState" :label="item.embyState==='visible'?'逐路径可见':'Emby尚待确认'"/></div>
      <code>{{item.source}}</code><p class="field-hint">{{item.mediaType==='series'?'剧集':'电影'}} · {{item.outputs.length}} 个归属输出 · 云端 {{item.cloudState}} · 文件级凭据{{item.revoked?'已撤销':'保持原值'}}</p>
      <div class="output-actions"><el-button @click="inspect(item)">查看输出归属</el-button><RouterLink :to="`/cloudStorage?assetId=${item.id}`">修正身份 / 输出修复</RouterLink><el-button :loading="refreshing===item.id" :disabled="item.artifactsState!=='ready'||item.cloudState!=='present'" @click="refreshEmby(item)">定向刷新并确认Emby</el-button></div>
    </article>
    <el-empty v-if="!busy&&!items.length" description="暂无此范围托管输出"/><el-pagination v-model:current-page="page" :page-size="25" :total="total" layout="prev,pager,next,total" @current-change="load"/>
  </section>
  <el-dialog v-model="dialog" title="文件级输出归属" width="min(800px,95vw)"><p class="field-hint">输出路径不是临时CDN URL；播放仍由原STRM固定入口按播放器UA请求115临时直链，302直达CDN。</p><div v-for="output in detail?.outputs??[]" :key="output.path" class="output-proof"><el-tag>{{output.kind.toUpperCase()}}</el-tag><code>{{output.path}}</code><small>{{output.owner}}</small><el-button v-if="['strm','nfo'].includes(output.kind)" :loading="contentBusy" @click="readContent(output.path)">查看原内容</el-button></div><el-alert v-if="error" :title="error" type="error" :closable="false"/><div v-if="content" class="output-body"><strong>{{content.path}}</strong><el-input :model-value="content.content" type="textarea" :rows="6" readonly aria-label="托管输出原始内容"/><p class="field-hint">所有权与实际SHA256均核对一致 · {{content.digest}}；只展示原正文，不重新生成链接。</p></div><h3>Emby逐路径确认</h3><div v-for="path in detail?.embyPaths??[]" :key="path.path" class="output-proof"><code>{{path.path}}</code><span>{{path.visible?'可见':'尚未可见'}} · {{path.reason}}</span></div><p class="field-hint">刷新请求成功不等同入库；可见性也不等同浏览器 / 原生客户端的完整音视频解码验收。</p></el-dialog>
</template>
<style scoped>.output-body{margin-top:16px}.output-body strong{display:block;overflow-wrap:anywhere;margin-bottom:12px}
.output-workbench{margin-bottom:24px}.output-filter{display:flex;gap:12px;margin-bottom:16px}.output-card{padding:20px 0;border-bottom:1px solid var(--border)}.output-head,.output-actions,.output-proof{display:flex;flex-wrap:wrap;align-items:center;gap:12px}.output-head{margin-bottom:12px}.output-proof{padding:14px 0;border-bottom:1px solid var(--border)}.output-proof code{flex:1;min-width:0}.output-proof small{flex-basis:100%;color:var(--muted)}code{overflow-wrap:anywhere;white-space:normal;line-height:1.7}.output-actions{margin-top:16px}.output-actions .el-button,.output-actions a{min-height:44px}.output-actions a{display:inline-flex;align-items:center;color:var(--el-color-primary)}.output-actions .el-button{margin-left:0}.el-pagination{margin-top:20px;overflow:auto}@media(max-width:680px){.output-head strong{flex-basis:100%}.output-proof code{flex-basis:100%}}
</style>
