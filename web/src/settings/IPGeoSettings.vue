<script setup lang="ts">
import {onMounted,reactive} from 'vue';
import {getGeoStatus} from '../api/ip-geolocation';
import {useResource} from '../workflows/useResource';
import ResourceState from '../workflows/ResourceState.vue';
import {date} from '../workflows/readiness';
import {usePageRefresh} from '../ui/page-refresh';
const state=reactive(useResource(getGeoStatus));
onMounted(state.load);usePageRefresh(state.load,'general');
</script>
<template>
 <section class="settings-card f7-geo"><h2>IP 归属地</h2>
  <p>数据来源 ip2region · 镜像内置离线数据，无需密钥。只查询公网 IP，局域网与保留地址不查询。</p>
  <ResourceState :stale="state.loaded" :loading="state.loading" :error="state.error" @reload="state.load"/>
  <dl v-if="state.data" class="ip-geo-status"><div><dt>数据版本</dt><dd>{{state.data.version}}</dd></div><div><dt>数据日期（北京时间）</dt><dd>{{date(state.data.dataDate||undefined)}}</dd></div><div><dt>IPv4</dt><dd>{{state.data.ipv4?'可用':'不可用'}} · {{date(state.data.ipv4Date||undefined)}}</dd></div><div><dt>IPv6</dt><dd>{{state.data.ipv6?'可用':'不可用'}} · {{date(state.data.ipv6Date||undefined)}}</dd></div></dl>
  <p>全量内存缓存约 {{((state.data?.memoryBytes||0)/1048576).toFixed(1)}} MiB / 应用进程（不含运行时开销）。数据库随应用镜像更新，运行时不会联网下载。</p>
  <p v-if="state.data?.reason" role="status">{{state.data.reason}}</p>
 </section>
</template>
<style scoped>.f7-geo{min-height:0}.ip-geo-status{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.ip-geo-status>div{min-width:0}.ip-geo-status dt{font-size:13px;color:var(--mt-text-muted)}.ip-geo-status dd{margin:6px 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}@media(max-width:600px){.ip-geo-status{grid-template-columns:1fr}}</style>
