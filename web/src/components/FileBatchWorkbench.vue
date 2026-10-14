<script setup lang="ts">
import { computed, ref } from 'vue'
import { request, post } from '../api/client'
import type { FileBatch, FileEntry, FileRoot } from '../files/types'
import { canApprove } from '../files/workbench'
import StatusBadge from './StatusBadge.vue'
const props = defineProps<{ root: FileRoot; files: FileEntry[] }>()
const emit = defineEmits<{ completed: [] }>()
interface Row {source:string;title:string;year:string;mediaType:string;season:number;episode:number;resolution:string;name:string;warnings:string[]}
const open = ref(false), rows = ref<Row[]>([]), operation = ref('organize'), destination = ref('.'), nameMode = ref('default'), busy = ref(false), error = ref(''), batch = ref<FileBatch | null>(null)
const approved = computed(() => canApprove(batch.value))
async function start() {
  open.value = true; batch.value = null; error.value = ''; rows.value = []; busy.value = true
  try {
    if (props.files.length < 1 || props.files.length > 20) throw new Error('每批请选择1–20个普通媒体文件')
    for (const file of props.files) {
      const {hint} = await post<{hint:Omit<Row,'source'|'name'>}>('/media/recognize-name',{name:file.name})
      rows.value.push({source:file.path,title:hint.title,year:hint.year,mediaType:hint.mediaType,season:hint.season,episode:hint.episode,resolution:hint.resolution,name:'',warnings:hint.warnings ?? []})
    }
  } catch(e) { error.value = e instanceof Error ? e.message : '识别失败' } finally { busy.value = false }
}
async function preview() {
  busy.value = true; error.value = ''
  try {
    batch.value = await post<FileBatch>('/files/batches/preview',{items:rows.value.map(({warnings,...row}) => ({...row,connectionId:props.root.id,operation:operation.value,destination:destination.value,includeSidecars:true,name:nameMode.value==='direct'?row.name:''}))})
  } catch(e) { error.value = e instanceof Error ? e.message : '批次预览失败' } finally { busy.value = false }
}
async function approve() {
  if (!approved.value || !batch.value) return
  busy.value = true; error.value = ''
  try { batch.value = await post<FileBatch>(`/files/batches/${batch.value.id}/approve`,{digest:batch.value.digest,confirm:true}) } catch(e) {error.value=e instanceof Error?e.message:'批准失败'} finally {busy.value=false}
}
async function refresh() {
  if(!batch.value) return; busy.value=true;error.value=''
  try {batch.value=await request<FileBatch>(`/files/batches/${batch.value.id}`);if(batch.value.state!=='running')emit('completed')}catch(e){error.value=e instanceof Error?e.message:'批次读取失败'}finally{busy.value=false}
}
async function reconcile() {
  if(!batch.value) return;busy.value=true;error.value=''
  try{batch.value=await post<FileBatch>(`/files/batches/${batch.value.id}/reconcile`,{});emit('completed')}catch(e){error.value=e instanceof Error?e.message:'核对失败'}finally{busy.value=false}
}
</script>
<template>
  <el-button :disabled="!files.length || files.length>20 || files.some(f=>f.directory||f.capabilities?.write===false)" @click="start">批量整理预览（{{files.length}}）</el-button>
  <el-dialog v-model="open" title="批次整理工作台" width="min(980px,95vw)" top="5vh" :close-on-click-modal="false" class="batch-workbench">
    <el-alert v-if="error" :title="error" type="error" :closable="false"/>
    <p class="field-hint">预览不会移动文件。请逐项核对身份、季集和旧→新路径；全部无冲突后才能批准。网络结果未知只读回核对，不重放写入。</p>
    <template v-if="!batch">
      <el-form label-position="top" class="batch-options">
        <el-form-item label="动作"><el-select v-model="operation" aria-label="批次动作"><el-option label="分类整理" value="organize"/><el-option label="重命名" value="rename"/><el-option label="移动（保留原名）" value="move"/></el-select></el-form-item>
        <el-form-item label="目标相对目录"><el-input v-model="destination" aria-label="批次目标目录"/><span class="field-hint">分类整理的 . 使用默认电影 / 剧集目录树</span></el-form-item>
        <el-form-item label="命名方式"><el-select v-model="nameMode" aria-label="批次命名方式"><el-option label="系统电影 / 剧集模板" value="default"/><el-option label="逐项自定义名称" value="direct"/></el-select></el-form-item>
      </el-form>
      <article v-for="row in rows" :key="row.source" class="batch-row">
        <code>{{row.source}}</code><p v-if="row.warnings.length" class="field-hint">{{row.warnings.join('；')}}；这是文件名提示，不是已确认身份。</p>
        <el-form label-position="top" class="batch-fields">
          <el-form-item label="标题"><el-input v-model="row.title" aria-label="批次文件标题"/></el-form-item>
          <el-form-item label="类型"><el-select v-model="row.mediaType" aria-label="批次媒体类型"><el-option label="未识别（需纠正）" value="unknown"/><el-option label="电影" value="movie"/><el-option label="剧集" value="series"/></el-select></el-form-item>
          <el-form-item label="年份"><el-input v-model="row.year" aria-label="批次年份"/></el-form-item>
          <el-form-item v-if="row.mediaType==='series'" label="季"><el-input-number v-model="row.season" :min="0" :max="999" aria-label="批次季号"/></el-form-item>
          <el-form-item v-if="row.mediaType==='series'" label="集"><el-input-number v-model="row.episode" :min="1" :max="9999" aria-label="批次集号"/></el-form-item>
          <el-form-item v-if="nameMode==='direct'" label="完整文件名"><el-input v-model="row.name" aria-label="批次自定义文件名"/></el-form-item>
        </el-form>
      </article>
    </template>
    <template v-else>
      <div class="batch-status"><StatusBadge :state="batch.state"/><span>预览截止 {{new Date(batch.expiresAt).toLocaleString()}}</span><RouterLink :to="`/organize-history?batchId=${batch.id}`">查看持久化批次</RouterLink></div>
      <article v-for="item in batch.items" :key="item.ordinal" class="batch-row">
        <StatusBadge :state="item.state"/><p v-if="item.message" class="field-hint">{{item.message}}</p>
        <div v-for="entry in item.result?.plan.items??[]" :key="entry.target" class="path-comparison"><code>{{entry.source.path}}</code><span>→</span><code>{{entry.target}}</code></div>
        <code v-if="!item.result">{{item.source}}</code>
        <p v-if="item.result" class="field-hint">{{item.result.completedSteps}} / {{item.result.plan.steps.length}} 个步骤核对完成 · 字幕语言后缀保留</p>
      </article>
    </template>
    <template #footer><el-button :disabled="busy" @click="open=false">{{batch?.state==='running'?'关闭（后台继续）':'取消 / 关闭'}}</el-button><el-button v-if="!batch" :loading="busy" :disabled="!rows.length" @click="preview">预览全部变更</el-button><el-button v-if="batch?.state==='preview'" :disabled="!approved" type="primary" :loading="busy" @click="approve">明确批准整个批次</el-button><el-button v-if="batch?.state==='running'" :loading="busy" @click="refresh">刷新执行进度</el-button><el-button v-if="batch?.state==='needs_reconcile'" :loading="busy" @click="reconcile">核对结果（不重放）</el-button></template>
  </el-dialog>
</template>
<style scoped>
.batch-options{display:grid;grid-template-columns:1fr 2fr 1fr;gap:16px}.batch-row{border-bottom:1px solid var(--border);padding:18px 0}.batch-row code{overflow-wrap:anywhere;white-space:normal;line-height:1.7}.batch-fields{display:grid;grid-template-columns:2fr 1fr 1fr;gap:12px;margin-top:12px}.path-comparison{display:grid;grid-template-columns:1fr 20px 1fr;gap:12px;margin:12px 0}.batch-status{display:flex;flex-wrap:wrap;gap:16px;margin:20px 0}:global(.batch-workbench){max-height:90dvh;display:flex;flex-direction:column}:global(.batch-workbench .el-dialog__body){overflow:auto;min-height:0}@media(max-width:680px){.batch-options,.batch-fields,.path-comparison{grid-template-columns:1fr}.path-comparison>span{display:none}}
</style>
