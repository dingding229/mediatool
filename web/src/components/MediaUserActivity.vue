<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElTabs, ElTabPane, ElMessage, ElMessageBox } from 'element-plus'
import { listMediaDevices, listMediaHistory, revokeMediaDevice, type MediaDevice, type MediaHistory, type ManagementPage } from '../api/mediaServer'
const props = defineProps<{ userId: string; userName: string }>()
const tab = ref('devices'), page = ref(1), busy = ref(false), acting = ref(false), error = ref('')
const devices = ref<ManagementPage<MediaDevice>>({ items: [], total: 0, startIndex: 0, limit: 50 })
const history = ref<ManagementPage<MediaHistory>>({ items: [], total: 0, startIndex: 0, limit: 50 })
let generation = 0
const date = (value: string) => new Date(value).toLocaleString()
const position = (ticks: number) => `${Math.floor(ticks / 600000000)} 分钟`
async function load() {
 const current = ++generation; busy.value = true; error.value = ''
 try {
  if (tab.value === 'devices') { const out = await listMediaDevices(props.userId, (page.value - 1) * 50); if (current === generation) devices.value = out }
  else { const out = await listMediaHistory(props.userId, (page.value - 1) * 50); if (current === generation) history.value = out }
 } catch (e) { if (current === generation) error.value = e instanceof Error ? e.message : '活动记录读取失败' }
 finally { if (current === generation) busy.value = false }
}
async function revoke(device: MediaDevice) {
 const userId=props.userId
 try {
  await ElMessageBox.confirm(`撤销「${device.name || '未命名设备'}」的登录？该设备令牌立即失效，需重新输入密码登录。其他设备不受影响。`, '撤销设备', { type: 'warning', confirmButtonText: '确认撤销', cancelButtonText: '取消' })
  acting.value = true
  await revokeMediaDevice(userId, device.id); ElMessage.success('设备登录已撤销'); await load()
 } catch (e) { if (e !== 'cancel' && e !== 'close') error.value = e instanceof Error ? e.message : '撤销失败' }
 finally { acting.value = false }
}
function changeTab() { page.value = 1; devices.value.items = []; history.value.items = []; load() }
watch(() => props.userId, changeTab)
onMounted(load)
</script>
<template>
 <section v-loading="busy" :aria-label="userName + '的设备与历史'">
  <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  <el-tabs v-model="tab" @tab-change="changeTab"><el-tab-pane label="登录设备" name="devices"/><el-tab-pane label="观看历史" name="history"/></el-tabs>
  <p class="hint">{{tab === 'devices' ? '保留设备记录；撤销只影响该设备，不显示令牌、UA 或网络地址。' : '每条目最近观看记录，包含当前已移除或不再授权的历史条目；不是逐次播放事件日志。'}}</p>
  <div class="activity-table" v-if="tab === 'devices'"><el-table :data="devices.items" empty-text="没有登录设备">
   <el-table-column prop="name" label="设备" min-width="140"/><el-table-column prop="client" label="客户端" min-width="120"/><el-table-column prop="appVersion" label="版本" width="90"/>
   <el-table-column label="登录状态" width="110"><template #default="{row}">{{row.activeTokens ? '有效登录' : '已失效'}}</template></el-table-column>
   <el-table-column label="最近访问" min-width="170"><template #default="{row}">{{date(row.lastSeenAt)}}</template></el-table-column>
   <el-table-column label="操作" width="110"><template #default="{row}"><el-button :disabled="acting || busy || !row.activeTokens" @click="revoke(row)">撤销设备</el-button></template></el-table-column>
  </el-table></div>
  <div class="activity-table" v-else><el-table :data="history.items" empty-text="没有观看历史">
   <el-table-column prop="name" label="条目 / 单集" min-width="180"/><el-table-column label="观看位置" width="110"><template #default="{row}">{{position(row.positionTicks)}}</template></el-table-column>
   <el-table-column prop="playCount" label="已看次数" width="100"/><el-table-column label="已看" width="80"><template #default="{row}">{{row.played ? '是' : '否'}}</template></el-table-column>
   <el-table-column label="最近观看" min-width="170"><template #default="{row}">{{date(row.lastPlayedAt)}}</template></el-table-column>
  </el-table></div>
  <el-pagination v-model:current-page="page" :total="tab === 'devices' ? devices.total : history.total" :page-size="50" layout="prev, pager, next, total" :disabled="busy || acting" @current-change="load"/>
 </section>
</template>
<style scoped>
.hint{color:var(--muted);line-height:1.7}.activity-table{overflow:auto;margin:16px 0}.activity-table .el-table{min-width:650px}.el-pagination{max-width:100%;overflow:auto}.el-button{min-height:40px}@media(max-width:600px){.el-button{min-height:44px}}
</style>
