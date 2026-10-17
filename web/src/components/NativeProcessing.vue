<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getProcessing, saveProcessing, listProcessing, retryProcessing, collectProcessing, type ProcessingKind, type ProcessingSummary, type ProcessingTask, type ProcessingPage } from '../api/mediaServer'
const emit=defineEmits<{ settingsSaved: [] }>()
const summary = ref<ProcessingSummary>()
const form = reactive({ enabled: false, probeEnabled: true, stillsEnabled: true, revision: 0 })
const filter = reactive({ kind: 'probe' as ProcessingKind, state: 'failed', page: 1, pageSize: 50 })
const tasks = ref<ProcessingPage>({ items: [], total: 0, page: 1, pageSize: 50 })
const loading = ref(false), acting = ref(false), error = ref(''), notice = ref('')
const noticeType = ref<'info' | 'warning'>('info')
const states: Record<string, string> = { pending: '等待中', probing: '探测中', grabbing: '截图中', ready: '已就绪', failed: '失败', needs_action: '需人工处理', skipped: '已跳过' }
const reasons: Record<string, string> = {
 not_exposable: '资产尚不可暴露', file_missing: '文件不存在', strm_mismatch: 'STRM 内容与记录不一致', authorization: '115 授权失效，请重新授权', rate_limited: '请求限流，稍后重试', timeout: '执行超时', unavailable: '探测工具不可用', unsafe_target: '解析目标不安全', ua_mismatch: '取链 UA 不一致', ticket_unavailable: '一次性票据不可用', lease_expired: '执行租约已过期', process_failed: '媒体工具执行失败', invalid_result: '探测结果无效', invalid_image: '截图图像无效', tmdb_still: '已有 TMDB 单集图片，无需截图', source_unavailable: '没有可用截图来源', storage_unavailable: '截图缓存暂不可用', storage_full: '截图缓存空间不足', cache_evicted: '截图已被容量回收（请求满 24 小时可恢复）', schema_outdated: '快照版本需要更新', link_timeout: '解析超时', probe_failed: '探测失败',
}
const currentStates = computed(() => filter.kind === 'probe' ? summary.value?.probe || [] : summary.value?.still || [])
const capacityPercent = computed(() => { const c = summary.value?.capacity; return c?.limitBytes ? Math.min(100, Math.round(100 * (c.usedBytes + c.reservedBytes) / c.limitBytes)) : 0 })
function bytes(n: number) { return n < 1024 ** 2 ? `${(n / 1024).toFixed(1)} KiB` : `${(n / 1024 ** 2).toFixed(1)} MiB` }
function date(value: string | null) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
function reason(code: string) { return code ? reasons[code] || `未知原因（${code}）` : '—' }
function failed(e: unknown) { error.value = e instanceof Error ? e.message : '预处理操作失败' }
let generation = 0
async function load() {
 const current = ++generation
 loading.value = true; error.value = ''
 try {
  const [status, page] = await Promise.all([getProcessing(), listProcessing(filter.kind, filter.state, filter.page, filter.pageSize)])
  if (current !== generation) return
  summary.value = status; Object.assign(form, status.settings); tasks.value = page
 } catch (e) { if (current === generation) { tasks.value = { items: [], total: 0, page: filter.page, pageSize: filter.pageSize }; failed(e) } } finally { if (current === generation) loading.value = false }
}
async function changeFilter() { filter.page = 1; await load() }
async function save() {
 if (acting.value) return
 acting.value = true; error.value = ''
 try { Object.assign(form, await saveProcessing({ ...form })); ElMessage.success('预处理开关已保存'); await load(); emit('settingsSaved') } catch (e) { failed(e) } finally { acting.value = false }
}
async function retry(task?: ProcessingTask) {
 if (acting.value || loading.value) return
 // Snapshot the filter before opening the dialog: changes while it is open
 // cannot silently broaden the confirmed request.
 const selected = { kind: filter.kind, state: filter.state }
 if (!task) {
  try { await ElMessageBox.confirm(`将重试“${selected.kind === 'probe' ? '探测' : '截图'} / ${states[selected.state] || '全部状态'}”当前筛选结果，单次最多 500 条；会重置退避及执行租约。是否继续？`, '确认批量重试', { type: 'warning', confirmButtonText: '确认重试', cancelButtonText: '取消' }) } catch { return }
 }
 if (acting.value) return
 acting.value = true; error.value = ''; notice.value = ''
 try {
  const out = await retryProcessing({ ...selected, id: task?.id || '', bulk: !task, confirmed: !task })
  noticeType.value = out.failed || out.limitReached ? 'warning' : 'info'
  notice.value = `本次选中 ${out.selected} 条，已排队 ${out.queued} 条，跳过 ${out.skipped} 条，失败 ${out.failed} 条。${out.limitReached ? '已达 500 条上限，可调整筛选后继续。' : ''}`
  await load()
 } catch (e) { failed(e) } finally { acting.value = false }
}
async function gc() {
 if (acting.value) return
 acting.value = true; error.value = ''; notice.value = ''
 try {
  const out = await collectProcessing()
  noticeType.value = out.partial ? 'warning' : 'info'
  notice.value = `本轮 GC 扫描 ${out.scanned} 项，回收 ${out.reclaimed} 个文件（${bytes(out.reclaimedBytes)}）。${out.partial ? '本轮部分完成，可稍后继续。' : out.done ? '本轮扫描已完成。' : '已保存扫描游标，可继续下一批。'}`
  await load()
 } catch (e) { failed(e) } finally { acting.value = false }
}
onMounted(load)
</script>
<template>
 <section class="panel processing" aria-labelledby="processing-title" v-loading="loading">
  <header class="processing-heading"><div><h2 id="processing-title">媒体预处理</h2><p>后台探测媒体信息，并为缺少 TMDB 单集图片的剧集补充截图。播放仍为原始流 302，不转码。</p></div><el-button :loading="loading" :disabled="acting" @click="load">刷新预处理</el-button></header>
  <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/>
  <el-alert v-if="notice" :title="notice" :type="noticeType" :closable="false" role="status"/>
  <el-form label-position="top" class="processing-switches" @submit.prevent="save">
   <el-form-item label="原生媒体服务" for="native-service-enabled"><el-switch id="native-service-enabled" v-model="form.enabled" aria-label="原生媒体服务" :disabled="!summary || acting"/></el-form-item>
   <el-form-item label="媒体探测" for="native-probe-enabled"><el-switch id="native-probe-enabled" v-model="form.probeEnabled" aria-label="媒体探测" :disabled="!summary || !form.enabled || acting"/></el-form-item>
   <el-form-item label="单集截图" for="native-stills-enabled"><el-switch id="native-stills-enabled" v-model="form.stillsEnabled" aria-label="单集截图" :disabled="!summary || !form.enabled || !form.probeEnabled || acting"/></el-form-item>
   <el-button type="primary" :loading="acting" :disabled="!summary || loading" @click="save">保存开关</el-button>
  </el-form>
  <p class="dependency-note">原生服务关闭时，探测与截图均不执行；截图还依赖探测开启及有效时长。关闭上级开关会保留下级配置，不删除已有结果。</p>
  <div v-if="summary" class="processing-summary">
   <section v-for="kind in (['probe', 'still'] as const)" :key="kind" role="region" :aria-label="kind === 'probe' ? '探测统计' : '截图统计'"><h3>{{kind === 'probe' ? '媒体探测' : '单集截图'}}</h3><dl><div v-for="entry in summary[kind]" :key="entry.state"><dt>{{states[entry.state] || entry.state}}</dt><dd>{{entry.count}}</dd></div></dl></section>
  </div>
  <section v-if="summary" class="capacity" role="region" aria-label="截图容量"><h3>截图缓存</h3><p>已用 {{bytes(summary.capacity.usedBytes)}} / {{bytes(summary.capacity.limitBytes)}}，执行中预留 {{bytes(summary.capacity.reservedBytes)}}</p><el-progress :percentage="capacityPercent" :status="capacityPercent >= 90 ? 'warning' : undefined"/><p v-if="summary.capacity.unknownSizes" class="dependency-note">{{summary.capacity.unknownSizes}} 张旧图片待校正大小，GC 会限批校正。</p><el-button :loading="acting" :disabled="!summary || loading" @click="gc">执行一轮 GC</el-button></section>
  <div class="processing-toolbar"><el-form inline label-position="top"><el-form-item label="任务类型"><el-select v-model="filter.kind" aria-label="任务类型" :disabled="acting" @change="filter.state='';changeFilter()"><el-option label="媒体探测" value="probe"/><el-option label="单集截图" value="still"/></el-select></el-form-item><el-form-item label="任务状态"><el-select v-model="filter.state" aria-label="任务状态" :disabled="acting" @change="changeFilter"><el-option label="全部状态" value=""/><el-option v-for="entry in currentStates" :key="entry.state" :label="states[entry.state] || entry.state" :value="entry.state"/></el-select></el-form-item></el-form><el-button :disabled="acting || loading || !tasks.total || !summary?.settings.enabled || !summary.settings.probeEnabled || (filter.kind==='still' && !summary.settings.stillsEnabled)" @click="retry()">重试当前筛选结果（最多 500 条）</el-button></div>
  <div class="processing-table"><el-table :data="tasks.items" empty-text="当前筛选下没有预处理任务"><el-table-column prop="name" label="资产 / 单集" min-width="190" show-overflow-tooltip/><el-table-column label="状态" width="110"><template #default="{row}">{{states[row.state] || row.state}}</template></el-table-column><el-table-column label="原因" min-width="230"><template #default="{row}"><span>{{reason(row.errorReason)}}</span><small v-if="row.errorReason">{{row.errorReason}}</small></template></el-table-column><el-table-column prop="attempts" label="尝试次数" width="95"/><el-table-column label="下次重试" min-width="180"><template #default="{row}">{{date(row.nextAttemptAt)}}</template></el-table-column><el-table-column label="操作" width="95"><template #default="{row}"><el-button :disabled="!row.id || acting || loading || !summary?.settings.enabled || !summary.settings.probeEnabled || (filter.kind==='still' && !summary.settings.stillsEnabled)" @click="retry(row)">重试</el-button></template></el-table-column></el-table></div>
  <el-pagination v-model:current-page="filter.page" :page-size="filter.pageSize" :total="tasks.total" layout="prev, pager, next, total" :disabled="acting || loading" @current-change="load"/>
 </section>
</template>
<style scoped>
.processing{margin-bottom:24px}.processing-heading,.processing-toolbar{display:flex;align-items:center;justify-content:space-between;gap:20px}.processing h2{margin:0 0 8px}.processing p,.processing dt{color:var(--muted);line-height:1.7}.processing-switches{display:flex;align-items:center;flex-wrap:wrap;gap:24px;margin-top:20px}.processing-switches .el-form-item{margin-bottom:0}.dependency-note{font-size:14px}.processing-summary{display:grid;grid-template-columns:1fr 1fr;gap:24px;margin:24px 0;border-block:1px solid var(--border)}.processing-summary dl{display:flex;flex-wrap:wrap;gap:20px}.processing-summary dd{margin:8px 0;font-weight:600;font-variant-numeric:tabular-nums}.capacity{max-width:650px;margin-bottom:24px}.processing-toolbar{align-items:flex-end;margin:24px 0 12px}.processing-toolbar .el-form-item{margin-bottom:0}.el-select{width:160px}.processing-table{overflow-x:auto;margin-bottom:20px}.processing-table .el-table{min-width:900px}.processing-table small{display:block;color:var(--muted);font-size:12px}.processing .el-alert{margin:16px 0}.processing .el-button{min-height:40px}@media(max-width:700px){.processing-heading,.processing-toolbar{align-items:flex-start;flex-direction:column}.processing-summary{grid-template-columns:1fr;gap:0}.processing .el-button{min-height:44px}.processing-toolbar .el-form{display:flex;flex-wrap:wrap;gap:12px}.el-pagination{overflow:auto;max-width:100%}}
</style>
