<script setup lang="ts">
import {computed,onMounted,reactive,ref,watch} from 'vue';
import {useRoute,useRouter} from 'vue-router';
import {getPlaybackStatistics,getPlaybackEvents,getPlaybackIPs,getDeviceIPHistory} from '../api/playback-statistics';
import {listMediaUsers,listMediaDevices} from '../api/mediaServer';
import {useResource} from '../workflows/useResource';
import {usePageNumber} from '../ui/page-query';
import {mediaTitle,playbackDuration} from '../utils/media-title';
import {date} from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import TrendChart from '../operations/TrendChart.vue';
import IpAddress from '../ui/IpAddress.vue';
import IPBlockEditor from './IPBlockEditor.vue';
import IPStatisticsReport from './IPStatisticsReport.vue';
import type {IPStatsPage} from '../api/playback-statistics';
const blockEditor=ref<InstanceType<typeof IPBlockEditor>>();
const route=useRoute(),router=useRouter();
const props=defineProps<{userId?:string;onlyIPs?:boolean;initialDevice?:string}>(),days=ref([0,7,30,90].includes(Number(route.query.statsDays))?Number(route.query.statsDays):30),device=ref(props.initialDevice||String(route.query.statsDevice||'')),user=ref(props.userId||String(route.query.statsUser||'')),mode=ref(props.onlyIPs?'ips':'events'),query=ref(String(route.query.ipQuery||'')),searchInput=ref(query.value);
const page=usePageNumber(props.userId?'activityStatsPage':'ipStatsPage'),size=ref(25),users=ref<any[]>([]),devices=ref<any[]>([]);
const filter=()=>({userId:user.value,deviceId:device.value,days:days.value,startIndex:(page.value-1)*size.value,limit:size.value,q:query.value});
const summary=reactive(useResource(()=>getPlaybackStatistics(filter()))),events=reactive(useResource(()=>getPlaybackEvents(filter()),{items:[],total:0,startIndex:0,limit:25})),ips=reactive(useResource<IPStatsPage>(()=>mode.value==='deviceIps'?getDeviceIPHistory(filter()):getPlaybackIPs(filter()),{items:[],total:0,startIndex:0,limit:25}));
const chart=computed(()=>(summary.data?.trend||[]).map(r=>({...r,at:r.date+'T00:00:00+08:00'})));
const current=computed(()=>mode.value!=='events'?ips:events);
async function load(){await Promise.all([props.onlyIPs||mode.value!=='events'?Promise.resolve():summary.load(),mode.value!=='events'?ips.load():events.load()])}
watch([days,user,device,mode,query,size],()=>{page.value=1;void router.replace({query:{...route.query,statsDays:String(days.value),statsUser:props.userId?undefined:user.value||undefined,statsDevice:device.value||undefined,statsGroup:undefined,ipQuery:query.value||undefined}});void load()});watch(page,load);
watch(user,async value=>{device.value='';if(mode.value==='deviceIps')mode.value='ips';devices.value=value?(await listMediaDevices(value,0,200)).items:[]});
function search(){const value=searchInput.value.trim();if(value===query.value){page.value=1;void load()}else query.value=value}
onMounted(async()=>{await load();if(!props.userId)users.value=(await listMediaUsers(1)).items;else devices.value=(await listMediaDevices(props.userId,0,200)).items});
defineExpose({load});
</script>
<template>
 <section class="f7-statistics">
  <div class="media-actions">
   <label>时间范围 <el-select v-model="days" aria-label="播放统计时间范围" style="width:130px"><el-option v-for="n in [7,30,90,0]" :key="n" :value="n" :label="n?n+' 天':'全部'"/></el-select></label>
   <label v-if="!userId">媒体用户 <el-select v-model="user" clearable aria-label="IP 统计用户" style="width:150px"><el-option v-for="u in users" :key="u.id" :value="u.id" :label="u.displayName||u.username"/></el-select></label>
   <label>设备 <el-select v-model="device" clearable aria-label="统计设备" style="width:150px"><el-option v-for="d in devices" :key="d.id" :value="d.id" :label="d.name||d.client||'未命名设备'"/></el-select></label>
   <el-button class="ip-refresh" :loading="current.loading" @click="load">刷新统计</el-button>
   <el-button v-if="!onlyIPs" :aria-pressed="mode==='events'" @click="mode='events'">最近播放</el-button><el-button v-if="!onlyIPs" :aria-pressed="mode==='ips'" @click="mode='ips'">IP 统计</el-button>
   <el-button v-if="device" :aria-pressed="mode==='deviceIps'" @click="mode='deviceIps'">设备 IP 历史</el-button>

  </div>
  <p v-if="mode==='events'" class="f7-scope">仅管理员可见。次数按实际开始的播放会话计，未看完也计 1 次；重复进度与 302 不增加次数。时长按有效进度增量计，暂停、跳转和无上报空档不计。统计自本功能启用起，受保留期限制。</p>
  <p v-else class="f7-scope">仅管理员可见，仅统计通过可信来源核对的公网 IP；同一会话的重复进度和 302 不增加播放次数，历史受保留期限制。</p>
  <template v-if="!onlyIPs&&mode==='events'">
   <ResourceState :stale="summary.loaded" :loading="summary.loading" :error="summary.error" @reload="load"/>
   <dl class="f7-metrics"><div><dt>播放次数</dt><dd>{{summary.data?.count??'—'}}</dd></div><div><dt>累计时长</dt><dd>{{playbackDuration(summary.data?.seconds)}}</dd></div><div><dt>观看内容</dt><dd>{{summary.data?.movies??0}} 部电影 / {{summary.data?.series??0}} 部剧 · {{summary.data?.episodes??0}} 集</dd></div></dl>
   <div class="f7-chart"><TrendChart title="每日播放" unit="次" :rows="chart" metric="count" :gap-ms="2*86400000"/></div>
   <details class="f7-top"><summary>最常观看 · Top 10</summary><ol><li v-for="row in summary.data?.top||[]" :key="row.itemId">{{mediaTitle(row)}} · {{row.count}} 次 · {{playbackDuration(row.seconds)}}</li></ol></details>
  </template>
  <ResourceState v-if="mode!=='ips'" :stale="current.loaded" :loading="current.loading" :error="current.error" :empty="mode!=='ips'&&!current.data?.items.length" @reload="load"/>
  <form v-if="mode==='ips'" class="media-actions ip-search" @submit.prevent="search"><el-input v-model="searchInput" aria-label="搜索 IP 统计" placeholder="搜索 IP、地区、运营商或用户" :maxlength="100" clearable/><el-button native-type="submit">搜索 IP</el-button><el-button :disabled="!query&&!searchInput" @click="searchInput='';query=''">清空</el-button></form>
  <div v-if="mode==='ips'" class="ip-report-shell" :aria-busy="ips.loading"><ResourceState overlay :stale="ips.loaded" :loading="ips.loading" :error="ips.error" @reload="load"/><IPStatisticsReport @block="blockEditor?.open($event)" v-show="!ips.error||ips.loaded" :rows="ips.data?.items||[]" :summary="ips.data?.summary" :user-scoped="Boolean(user)"/></div>
  <div v-else class="f7-results" :aria-busy="current.loading">
   <template v-if="mode==='events'"><article v-for="row in events.data?.items||[]" :key="row.id" class="organize-record"><h3>{{mediaTitle(row)}}</h3><p>{{date(row.startedAt)}} · {{row.userName}} / {{row.deviceName||row.client}} · {{playbackDuration(row.durationSeconds)}}</p><p><IpAddress :ip="row.ip"/> · {{row.state==='stopped'?'已结束':row.state==='paused'?'暂停':'播放中'}} · 302 {{row.redirectCount}} 次</p></article></template>
   <template v-else><article v-for="row in ips.data?.items||[]" :key="row.ip" class="organize-record"><h3><IpAddress :ip="row.ip"/></h3><p>{{row.count}} 次已鉴权请求</p><el-button class="ip-block-quick-button" @click="blockEditor?.open(row.ip)">拉黑此 IP</el-button><p>首次 {{date(row.firstSeenAt)}} · 最近 {{date(row.lastSeenAt)}}</p></article></template>
  </div>
  <PagePagination v-model:current-page="page" v-model:page-size="size" :total="current.data?.total||0" :page-sizes="[25,50,100]" label="播放统计分页"/>

 </section>
<IPBlockEditor ref="blockEditor"/>
</template>
<style scoped>
.ip-block-quick-button{min-height:44px}
.f7-statistics{min-height:480px}.f7-statistics>.media-actions{align-items:flex-end}.ip-refresh{min-width:108px}.ip-report-shell{position:relative;min-height:650px}.ip-search{margin:16px 0}.ip-search .el-input{flex:1;min-width:120px;max-width:480px}.f7-scope{font-size:.8rem;color:var(--mt-text-muted,#64748b)}.f7-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;min-height:75px}.f7-metrics div{padding:10px;border:1px solid var(--el-border-color);border-radius:8px}.f7-metrics dt{font-size:.8rem}.f7-metrics dd{margin:6px 0 0;font-weight:600;font-variant-numeric:tabular-nums}.f7-chart{min-height:220px}.f7-chart :deep(.u6-trend){min-height:210px}.f7-results{height:460px;overflow:auto;overscroll-behavior:contain;padding:6px;border:1px solid var(--el-border-color);border-radius:8px;box-shadow:inset 0 -8px 8px -10px #0006}.f7-top{margin:12px 0}.f7-top li{padding:4px 0}.f7-statistics .organize-record{padding:10px}.f7-statistics .organize-record h3,.f7-statistics .organize-record p{margin:4px 0}@media(max-width:600px){.f7-metrics{grid-template-columns:1fr}.f7-statistics .media-actions>label{display:flex;align-items:center;gap:8px}}</style>
