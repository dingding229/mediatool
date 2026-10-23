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
import IpAttribution from '../ui/IpAttribution.vue';
const route=useRoute(),router=useRouter();
const props=defineProps<{userId?:string;onlyIPs?:boolean;initialDevice?:string}>(),days=ref([0,7,30,90].includes(Number(route.query.statsDays))?Number(route.query.statsDays):30),device=ref(props.initialDevice||String(route.query.statsDevice||'')),user=ref(props.userId||String(route.query.statsUser||'')),mode=ref(props.onlyIPs?'ips':'events'),group=ref(['country','province'].includes(String(route.query.statsGroup))?String(route.query.statsGroup):'ip');
const page=usePageNumber(props.userId?'activityStatsPage':'ipStatsPage'),size=ref(25),users=ref<any[]>([]),devices=ref<any[]>([]);
const filter=()=>({userId:user.value,deviceId:device.value,days:days.value,startIndex:(page.value-1)*size.value,limit:size.value,group:group.value});
const summary=reactive(useResource(()=>getPlaybackStatistics(filter()))),events=reactive(useResource(()=>getPlaybackEvents(filter()),{items:[],total:0,startIndex:0,limit:25})),ips=reactive(useResource(()=>mode.value==='deviceIps'?getDeviceIPHistory(filter()):getPlaybackIPs(filter()),{items:[],total:0,startIndex:0,limit:25}));
const chart=computed(()=>(summary.data?.trend||[]).map(r=>({...r,at:r.date+'T00:00:00+08:00'})));
const current=computed(()=>mode.value!=='events'?ips:events);
async function load(){await Promise.all([props.onlyIPs?Promise.resolve():summary.load(),mode.value!=='events'?ips.load():events.load()])}
watch([days,user,device,mode,group,size],()=>{page.value=1;void router.replace({query:{...route.query,statsDays:String(days.value),statsUser:props.userId?undefined:user.value||undefined,statsDevice:device.value||undefined,statsGroup:group.value}});void load()});watch(page,load);
watch(user,async value=>{device.value='';if(mode.value==='deviceIps')mode.value='ips';devices.value=value?(await listMediaDevices(value,0,200)).items:[]});
onMounted(async()=>{await load();if(!props.userId)users.value=(await listMediaUsers(1)).items;else devices.value=(await listMediaDevices(props.userId,0,200)).items});
defineExpose({load});
</script>
<template>
 <section class="f7-statistics">
  <div class="media-actions">
   <label>时间范围 <el-select v-model="days" aria-label="播放统计时间范围" style="width:130px"><el-option v-for="n in [7,30,90,0]" :key="n" :value="n" :label="n?n+' 天':'全部'"/></el-select></label>
   <label v-if="!userId">媒体用户 <el-select v-model="user" clearable aria-label="IP 统计用户" style="width:150px"><el-option v-for="u in users" :key="u.id" :value="u.id" :label="u.displayName||u.username"/></el-select></label>
   <label>设备 <el-select v-model="device" clearable aria-label="统计设备" style="width:150px"><el-option v-for="d in devices" :key="d.id" :value="d.id" :label="d.name||d.client||'未命名设备'"/></el-select></label>
   <el-button v-if="!onlyIPs" :aria-pressed="mode==='events'" @click="mode='events'">最近播放</el-button><el-button v-if="!onlyIPs" :aria-pressed="mode==='ips'" @click="mode='ips'">IP 统计</el-button>
   <el-button v-if="device" :aria-pressed="mode==='deviceIps'" @click="mode='deviceIps'">设备 IP 历史</el-button>
   <label v-if="mode==='ips'">汇总 <el-select v-model="group" aria-label="IP 汇总方式" style="width:110px"><el-option value="ip" label="IP"/><el-option value="country" label="国家"/><el-option value="province" label="省份"/></el-select></label>
  </div>
  <p class="f7-scope">仅管理员可见。次数按实际开始的播放会话计，未看完也计 1 次；重复进度与 302 不增加次数。时长按有效进度增量计，暂停、跳转和无上报空档不计。统计自本功能启用起，受保留期限制。</p>
  <template v-if="!onlyIPs">
   <ResourceState :stale="summary.loaded" :loading="summary.loading" :error="summary.error" @reload="load"/>
   <dl class="f7-metrics"><div><dt>播放次数</dt><dd>{{summary.data?.count??'—'}}</dd></div><div><dt>累计时长</dt><dd>{{playbackDuration(summary.data?.seconds)}}</dd></div><div><dt>观看内容</dt><dd>{{summary.data?.movies??0}} 部电影 / {{summary.data?.series??0}} 部剧 · {{summary.data?.episodes??0}} 集</dd></div></dl>
   <div class="f7-chart"><TrendChart title="每日播放" unit="次" :rows="chart" metric="count" :gap-ms="2*86400000"/></div>
   <details class="f7-top"><summary>最常观看 · Top 10</summary><ol><li v-for="row in summary.data?.top||[]" :key="row.itemId">{{mediaTitle(row)}} · {{row.count}} 次 · {{playbackDuration(row.seconds)}}</li></ol></details>
  </template>
  <ResourceState :stale="current.loaded" :loading="current.loading" :error="current.error" :empty="!current.data?.items.length" @reload="load"/>
  <div class="f7-results" :aria-busy="current.loading">
   <template v-if="mode==='events'"><article v-for="row in events.data?.items||[]" :key="row.id" class="organize-record"><h3>{{mediaTitle(row)}}</h3><p>{{date(row.startedAt)}} · {{row.userName}} / {{row.deviceName||row.client}} · {{playbackDuration(row.durationSeconds)}}</p><p><IpAddress :ip="row.ip"/> · {{row.state==='stopped'?'已结束':row.state==='paused'?'暂停':'播放中'}} · 302 {{row.redirectCount}} 次</p></article></template>
   <template v-else><article v-for="row in ips.data?.items||[]" :key="row.location||row.ip" class="organize-record"><h3><span v-if="mode==='ips'&&group!=='ip'">{{row.location||'未配置归属地库'}}</span><IpAddress v-else :ip="row.ip"/></h3><p><template v-if="mode==='deviceIps'">{{row.count}} 次已鉴权请求</template><template v-else>{{row.users}} 用户 · {{row.devices}} 设备 · {{row.count}} 次播放</template></p><p>首次 {{date(row.firstSeenAt)}} · 最近 {{date(row.lastSeenAt)}}</p></article></template>
  </div>
  <PagePagination v-model:current-page="page" v-model:page-size="size" :total="current.data?.total||0" :page-sizes="[25,50,100]" label="播放统计分页"/>
  <IpAttribution/>
 </section>
</template>
<style scoped>.f7-statistics{min-height:480px}.f7-scope{font-size:.8rem;color:var(--mt-text-muted,#64748b)}.f7-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;min-height:75px}.f7-metrics div{padding:10px;border:1px solid var(--el-border-color);border-radius:8px}.f7-metrics dt{font-size:.8rem}.f7-metrics dd{margin:6px 0 0;font-weight:600;font-variant-numeric:tabular-nums}.f7-chart{min-height:220px}.f7-chart :deep(.u6-trend){min-height:210px}.f7-results{height:460px;overflow:auto;overscroll-behavior:contain;padding:6px;border:1px solid var(--el-border-color);border-radius:8px;box-shadow:inset 0 -8px 8px -10px #0006}.f7-top{margin:12px 0}.f7-top li{padding:4px 0}.f7-statistics .organize-record{padding:10px}.f7-statistics .organize-record h3,.f7-statistics .organize-record p{margin:4px 0}@media(max-width:600px){.f7-metrics{grid-template-columns:1fr}.f7-statistics .media-actions>label{display:flex;align-items:center;gap:8px}}</style>
