<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listNativeSessions, type ManagementPage, type NativeSession } from '../api/mediaServer'
const page = ref(1), busy = ref(false), error = ref('')
const data = ref<ManagementPage<NativeSession>>({ items: [], total: 0, startIndex: 0, limit: 50 })
async function load() { busy.value = true; error.value = ''; try { data.value = await listNativeSessions((page.value - 1) * 50) } catch (e) { error.value = e instanceof Error ? e.message : '会话列表读取失败' } finally { busy.value = false } }
onMounted(load)
</script>
<template>
 <section class="panel sessions" v-loading="busy" aria-label="在线播放会话"><div class="heading"><div><h2>在线会话 / 正在播放</h2><p>最近 5 分钟内上报的播放与暂停会话；不含仅获取 PlaybackInfo 的预备记录。</p></div><el-button :loading="busy" @click="load">刷新播放会话</el-button></div><el-alert v-if="error" :title="error" type="error" :closable="false"/>
 <div class="session-table"><el-table :data="data.items" empty-text="当前没有活跃播放会话"><el-table-column prop="userName" label="媒体用户" min-width="140"/><el-table-column prop="deviceName" label="设备" min-width="140"/><el-table-column prop="itemName" label="条目" min-width="180"/><el-table-column label="状态" width="90"><template #default="{row}">{{row.state==='paused'?'暂停':'播放中'}}</template></el-table-column><el-table-column label="位置" width="110"><template #default="{row}">{{Math.floor(row.positionTicks/600000000)}} 分钟</template></el-table-column><el-table-column label="最近上报" min-width="180"><template #default="{row}">{{new Date(row.lastProgressAt).toLocaleString()}}</template></el-table-column></el-table></div>
 <el-pagination v-model:current-page="page" :total="data.total" :page-size="50" layout="prev, pager, next, total" :disabled="busy" @current-change="load"/></section>
</template>
<style scoped>
.sessions{margin-bottom:24px}.heading{display:flex;justify-content:space-between;align-items:center;gap:16px}.sessions p{color:var(--muted);line-height:1.7}.session-table{overflow:auto;margin:16px 0}.session-table .el-table{min-width:800px}.el-pagination{max-width:100%;overflow:auto}.el-button{min-height:40px}@media(max-width:700px){.heading{align-items:flex-start;flex-direction:column}.el-button{min-height:44px}}
</style>
