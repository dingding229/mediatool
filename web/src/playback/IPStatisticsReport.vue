<script setup lang="ts">
import {computed,ref} from 'vue';
import type {IPStats,IPOverview,IPDistribution} from '../api/playback-statistics';
import StateView from '../ui/StateView.vue';
import {date} from '../workflows/readiness';
defineEmits<{block:[ip:string]}>();
const props=defineProps<{rows:IPStats[];summary?:IPOverview;userScoped?:boolean}>();
const rank=ref('province');
const regions=computed(()=>rank.value==='city'?props.summary?.cities||[]:props.summary?.provinces||[]);
const operators=computed(()=>props.summary?.operators||[]);
function width(row:IPDistribution,rows:IPDistribution[]){return Math.max(1,Math.round(row.count/Math.max(1,...rows.map(r=>r.count))*100))+'%'}
</script>
<template>
 <div class="ip-report">
  <dl class="f7-metrics ip-kpis" aria-label="IP 统计概览"><div><dt>独立 IP 数</dt><dd>{{summary?.uniqueIPs??'—'}}</dd></div><div><dt title="按国家、省、市组合去重，不含未知归属地">地区数</dt><dd>{{summary?.regions??'—'}}</dd></div><div><dt>活跃用户数</dt><dd>{{summary?.activeUsers??'—'}}</dd></div></dl>
  <section v-if="summary?.uniqueIPs===0" class="ip-details ip-empty" aria-label="IP 明细"><h3>IP 明细</h3><StateView compact state="empty" title="暂无匹配的公网 IP" description="可调整时间范围或清空搜索；局域网与保留地址不纳入统计。"/></section>
  <template v-else>
  <div class="ip-distributions">
   <section class="ip-card"><header><h3>地区排行</h3><nav class="mt-segments" aria-label="地区排行层级"><button type="button" :aria-pressed="rank==='province'" @click="rank='province'">省份</button><button type="button" :aria-pressed="rank==='city'" @click="rank='city'">城市</button></nav></header>
    <ol v-if="regions.length" class="ip-ranking"><li v-for="row in regions.slice(0,10)" :key="row.name"><div><span>{{row.name}}</span><strong>{{row.count}} 次</strong></div><span class="ip-track" aria-hidden="true"><span :style="{width:width(row,regions)}"/></span></li></ol><StateView v-else compact state="empty" title="暂无地区记录" description="仅展示所选范围内可定位的公网 IP。"/>
   </section>
   <section class="ip-card"><header><h3>运营商分布</h3></header><ol v-if="operators.length" class="ip-ranking"><li v-for="row in operators" :key="row.name"><div><span>{{row.name}}</span><strong>{{row.count}} 次</strong></div><span class="ip-track" aria-hidden="true"><span :style="{width:width(row,operators)}"/></span></li></ol><StateView v-else compact state="empty" title="暂无运营商记录" description="有可核对的公网访问后显示分布。"/></section>
  </div>
  <p class="ip-help">地区排行显示前 10 项；分布按播放会话次数统计，同一会话涉及多个 IP 或地区时分别计入对应项，不相加当作总播放数。<template v-if="userScoped">“新 IP”表示保留记录内首次出现距今不超过 7 天；“异地”表示与本期常用地区不同，仅作访问记录提示，不代表账号异常。</template></p>
  <section class="ip-details" aria-label="IP 明细"><h3>IP 明细</h3>
   <StateView v-if="!rows.length" compact state="empty" title="暂无匹配的公网 IP" description="可调整时间范围或清空搜索；局域网与保留地址不纳入统计。"/>
   <table v-else><thead><tr><th scope="col">IP</th><th scope="col">归属地</th><th scope="col">运营商</th><th v-if="!userScoped" scope="col">关联用户</th><th v-else scope="col">设备 / 客户端</th><th scope="col">次数</th><th v-if="userScoped" scope="col">首次出现</th><th scope="col">最后出现</th><th scope="col">操作</th></tr></thead>
    <tbody><tr v-for="row in rows" :key="row.ip"><td data-label="IP"><strong>{{row.ip}}</strong><div v-if="userScoped" class="ip-tags"><el-tag v-if="row.newIP" size="small" type="info">新 IP</el-tag><el-tag v-if="row.differentRegion" size="small" type="warning">异地</el-tag></div></td><td data-label="归属地">{{row.location||'未知归属地'}}</td><td data-label="运营商">{{row.isp||'未知运营商'}}</td><td v-if="!userScoped" data-label="关联用户"><span v-for="account in row.accounts||[]" :key="account.id" class="ip-person">{{account.name}}</span><span v-if="!row.accounts?.length">{{row.users}} 用户</span></td><td v-else data-label="设备 / 客户端"><span v-for="client in row.clients||[]" :key="client.id" class="ip-person">{{client.name||'未命名设备'}} · {{client.client||'未知客户端'}}</span><span v-if="!row.clients?.length">{{row.devices}} 设备</span></td><td data-label="次数">{{row.count}} 次播放</td><td v-if="userScoped" data-label="首次出现">{{date(row.firstSeenAt)}}</td><td data-label="最后出现">{{date(row.lastSeenAt)}}</td><td data-label="操作"><el-button @click="$emit('block',row.ip)">加入 IP 黑名单</el-button></td></tr></tbody>
   </table>
  </section>
  </template>
 </div>
</template>
<style scoped>
.ip-details :deep(.el-button){min-height:44px;margin:0}

.ip-report{container-type:inline-size;min-width:0}.ip-kpis{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin:16px 0}.ip-kpis>div,.ip-card{padding:16px;border:1px solid var(--mt-border-color);border-radius:8px;background:var(--mt-bg-surface);min-width:0}.ip-kpis dt{font-size:13px;color:var(--mt-text-muted)}.ip-kpis dd{margin:8px 0 0;font-size:24px;font-weight:600;font-variant-numeric:tabular-nums}
.ip-distributions{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.ip-card{min-height:240px}.ip-card header{display:flex;align-items:center;justify-content:space-between;gap:12px;min-height:52px;flex-wrap:wrap}.ip-card h3,.ip-details h3{font-size:16px;margin:0}.ip-card .mt-segments{margin:0}.ip-ranking{list-style:none;padding:0;margin:12px 0 0;display:grid;gap:16px;max-height:360px;overflow:auto}.ip-ranking li>div{display:flex;align-items:baseline;justify-content:space-between;gap:12px;font-size:13px}.ip-ranking li>div>span{overflow-wrap:anywhere;min-width:0}.ip-ranking strong{white-space:nowrap;font-variant-numeric:tabular-nums}.ip-track{display:block;margin-top:8px;height:6px;border-radius:6px;background:var(--mt-bg-canvas)}.ip-track>span{display:block;height:100%;border-radius:inherit;background:var(--mt-color-primary)}
.ip-help{font-size:13px;color:var(--mt-text-muted);line-height:1.7}.ip-details{min-width:0}.ip-details table{width:100%;border-collapse:collapse;table-layout:fixed;margin-top:12px;font-size:13px}.ip-details th{text-align:left;color:var(--mt-text-muted);font-weight:500}.ip-details td,.ip-details th{padding:12px 8px;border-bottom:1px solid var(--mt-border-color);vertical-align:top;overflow-wrap:anywhere;line-height:1.7}.ip-details td:first-child,.ip-details th:first-child{padding-left:0}.ip-person{display:block}.ip-tags{display:flex;flex-wrap:wrap;gap:4px;margin-top:6px}
@container(max-width:700px){.ip-distributions{grid-template-columns:1fr}.ip-details table,.ip-details tbody,.ip-details tr,.ip-details td{display:block;width:auto}.ip-details thead{display:none}.ip-details tr{padding:12px;border:1px solid var(--mt-border-color);border-radius:8px;margin-top:12px}.ip-details td{display:grid;grid-template-columns:90px minmax(0,1fr);gap:8px;padding:8px 0;border:0}.ip-details td:before{content:attr(data-label);color:var(--mt-text-muted)}.ip-tags{grid-column:2}.ip-details td .ip-person{grid-column:2}}
@media(max-width:600px){.ip-kpis{gap:8px}.ip-kpis>div{padding:12px 8px}.ip-kpis dt{font-size:12px}.ip-kpis dd{font-size:22px}}
</style>
