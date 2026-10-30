<script setup lang="ts">
import {computed,reactive,watch} from 'vue';
import {useRoute,useRouter} from 'vue-router';
import {request} from '../api/client';
import {useRefresh} from '../composables/useRefresh';
import {useResource} from '../workflows/useResource';
import {usePageRefresh} from '../ui/page-refresh';
import PageHeader from '../ui/PageHeader.vue';
import ResourceState from '../workflows/ResourceState.vue';
import TrendChart from '../operations/TrendChart.vue';
import '../operations/u6.css';
const route=useRoute(),router=useRouter(),ranges=[1,24,168,720];
const hours=computed({get:()=>ranges.includes(Number(route.query.hours))?Number(route.query.hours):24,set:(value:number)=>{void router.replace({query:{...route.query,hours:String(value)},hash:route.hash})}});
// Moved from MaintenancePanel: same endpoint, sampling gap and derived metrics.
const trends=reactive(useResource<any[]>(()=>request('/ops/trends?hours='+hours.value),[]));
const gapMS=computed(()=>Math.max(1,Math.ceil(hours.value*60/500))*60000);
const trendRows=computed(()=>trends.data.map(r=>({...r,processMiB:typeof r.processBytes==='number'?r.processBytes/1048576:null,successRate:typeof r.succeededJobs10m==='number'&&typeof r.failedJobs10m==='number'&&r.succeededJobs10m+r.failedJobs10m>0?r.succeededJobs10m/(r.succeededJobs10m+r.failedJobs10m)*100:null})));
const trendFields=[['memoryPercent','内存使用率','%'],['diskFreeGB','剩余空间','GiB'],['load1','系统负载（1 分钟平均）',''],['loadPerCPU','每CPU负载',''],['jobSeconds','任务执行耗时','秒'],['pendingJobs','待处理任务','条'],['processMiB','应用堆内存','MiB'],['failedJobs10m','十分钟失败任务','条'],['successRate','十分钟任务成功率','%']];
watch(hours,()=>void trends.load());
useRefresh(trends.load,{interval:30000});
usePageRefresh(trends.load);
</script>
<template>
<section id="trends" class="runtime-trends u6-workspace" aria-label="运行趋势">
 <PageHeader title="运行趋势" description="查看系统采样与任务执行趋势，缺失采样不补零。">
  <template #actions>
   <el-select v-model="hours" aria-label="趋势时间范围" class="trend-range">
    <el-option v-for="h in ranges" :key="h" :label="`最近 ${h} 小时`" :value="h"/>
   </el-select>
  </template>
 </PageHeader>
 <ResourceState :stale="trends.loaded" :loading="trends.loading" :error="trends.error" @reload="trends.load"/>
 <template v-if="trends.loaded">
 <div class="trend-list">
  <TrendChart v-for="[metric,title,unit] in trendFields" :key="metric" :title="title!" :unit="unit!" :metric="metric!" :rows="trendRows" :gap-ms="gapMS" stretch/>
 </div>
 </template>
</section>
</template>
<style scoped>
.runtime-trends{min-width:0}.runtime-trends :deep(.page-heading-main){min-width:180px}.trend-list{display:grid;grid-template-columns:minmax(0,1fr);gap:16px}.trend-list :deep(.u6-trend){background:var(--mt-bg-surface)}.trend-list :deep(.u6-trend p){font-size:13px;line-height:1.6;color:var(--mt-text-muted);margin:8px 0}.trend-range{width:180px}
</style>
