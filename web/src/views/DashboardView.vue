<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { request } from '../api/client'
import PluginDashboards from '../components/PluginDashboards.vue'
import StatusBadge from '../components/StatusBadge.vue'
interface Runtime { server:{loadAverage?:number[];memoryTotal?:number;memoryUsed?:number;diskTotal?:number;diskAvailable?:number;uptimeSeconds?:number;sampledAt:string}; daily:Record<string,number> }
const snapshot=ref<Runtime|null>(null),recent=ref<Array<{id:string;keyword:string;state:string;total:number}>>([]),busy=ref(false),error=ref('')
const daily=[{key:'sites',label:'已启用站点',to:'/sites'},{key:'subscriptions',label:'进行中的订阅',to:'/subscriptions'},{key:'downloads',label:'下载与做种',to:'/downloads'},{key:'media',label:'媒体资产',to:'/mediaLibrary'},{key:'todaySearches',label:'今日搜索',to:'/search'},{key:'failedJobs',label:'待检查任务',to:'/jobs'}]
const size=(n?:number)=>n===undefined?'不可用':(n/1073741824).toFixed(1)+' GiB'
async function refresh(){busy.value=true;error.value='';try{snapshot.value=await request<Runtime>('/system/runtime');recent.value=(await request<typeof recent.value>('/search')).slice(0,5)}catch(e){error.value=String(e)}finally{busy.value=false}}
onMounted(refresh)
</script>
<template>
<section class="page-heading"><div><span class="page-kicker">WORKSPACE / 总览</span><h1>运行总览</h1><p>服务器状态与日常媒体管理。</p></div><el-button :icon="Refresh" :loading="busy" @click="refresh">刷新状态</el-button></section>
<el-alert v-if="error" :title="error" type="error" :closable="false"/>
<section class="runtime-grid" aria-label="服务器状态">
<article class="panel"><span>服务器负载</span><strong>{{snapshot?.server.loadAverage?.[0]?.toFixed(2) ?? '不可用'}}</strong><p class="field-hint">1 / 5 / 15 分钟：{{snapshot?.server.loadAverage?.map(n=>n.toFixed(2)).join(' / ') || '未挂载宿主机 proc'}}</p></article>
<article class="panel"><span>内存占用</span><strong>{{size(snapshot?.server.memoryUsed)}}</strong><el-progress v-if="snapshot?.server.memoryTotal" :percentage="Math.round((snapshot.server.memoryUsed || 0)/snapshot.server.memoryTotal*100)"/><p class="field-hint">宿主机总内存 {{size(snapshot?.server.memoryTotal)}}</p></article>
<article class="panel"><span>剩余存储空间</span><strong>{{size(snapshot?.server.diskAvailable)}}</strong><p class="field-hint">数据目录所在文件系统 · 共 {{size(snapshot?.server.diskTotal)}}</p></article>
<article class="panel"><span>服务器运行时间</span><strong>{{snapshot?.server.uptimeSeconds!==undefined ? Math.floor(snapshot.server.uptimeSeconds/86400)+' 天' : '不可用'}}</strong><p class="field-hint">数据采样 {{snapshot ? new Date(snapshot.server.sampledAt).toLocaleTimeString('zh-CN') : '—'}}</p></article>
</section>
<section class="daily-grid"><RouterLink v-for="item in daily" :key="item.key" :to="item.to" class="panel"><span>{{item.label}}</span><strong>{{snapshot?.daily[item.key] ?? '—'}}</strong></RouterLink></section>
<section class="panel"><div class="panel-title"><h2>最近搜索</h2><RouterLink to="/search-cache">查看搜索记录</RouterLink></div><div v-for="item in recent" :key="item.id" class="daily-search"><RouterLink :to="{path:`/search/results/${item.id}`}">{{item.keyword}}</RouterLink><StatusBadge :state="item.state"/><span>{{item.total}} 个资源</span></div><p v-if="!recent.length" class="field-hint">暂无搜索记录，前往资源搜索发现作品。</p></section>
<PluginDashboards/>
</template>
<style scoped>
.runtime-grid,.daily-grid{display:grid;gap:16px;margin-bottom:24px}.runtime-grid{grid-template-columns:repeat(4,minmax(0,1fr))}.daily-grid{grid-template-columns:repeat(6,minmax(0,1fr))}article span,.daily-grid span{color:var(--muted);font-size:13px}strong{display:block;font-size:28px;margin:14px 0;letter-spacing:-.5px}.daily-grid a{color:var(--text);text-decoration:none}.daily-grid a:hover{border-color:var(--primary)}.daily-search{display:grid;grid-template-columns:1fr 110px 100px;align-items:center;gap:16px;padding:16px 0;border-bottom:1px solid var(--border)}.daily-search a{overflow-wrap:anywhere}.daily-search:last-child{border-bottom:0}@media(max-width:1100px){.runtime-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.daily-grid{grid-template-columns:repeat(3,minmax(0,1fr))}}@media(max-width:600px){.runtime-grid,.daily-grid{grid-template-columns:repeat(2,minmax(0,1fr))}strong{font-size:23px}.runtime-grid article,.daily-grid a{padding:16px}.daily-search{grid-template-columns:1fr 90px}.daily-search>span:last-child{display:none}}
</style>
