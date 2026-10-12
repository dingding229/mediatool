<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessageBox, ElMessage, ElDatePicker } from 'element-plus'
import { Refresh, Delete, CopyDocument, Download, Setting, Search, ArrowDown } from '@element-plus/icons-vue'
import { request, post } from '../api/client'
import { formatLogTime, logLevel, logSource, logFields, inlineLogFields, logText } from '../logs/format'
import type { LogRecord } from '../logs/format'
interface Summary { totalBytes:number; limitBytes:number; level:string; files:Array<{name:string;size:number;modified:string}> }
interface LogPage { items:LogRecord[]; total:number; page:number; pageSize:number; modules:string[]; before:string; truncated:boolean; maxFileBytes:number; maxRecords:number }
const linkOpen=ref(false),linkRow=ref<LogRecord|null>(null)
const summary=ref<Summary|null>(null),rows=ref<LogRecord[]>([]),level=ref(''),module=ref(''),keyword=ref(''),range=ref<Date[]|null>(null)
const busy=ref(false),summaryBusy=ref(false),error=ref(''),clean=ref(false),settingsOpen=ref(false),copyOpen=ref(false),copyContent=ref('')
const page=ref(1),pageSize=ref(200),total=ref(0),modules=ref<string[]>([]),before=ref(''),truncated=ref(false),expanded=ref(new Set<number>())
let controller:AbortController|undefined,epoch=0,timer:ReturnType<typeof setTimeout>|undefined
const bytes=(n:number)=>n<1024?n+' B':n<1048576?(n/1024).toFixed(1)+' KiB':(n/1048576).toFixed(2)+' MiB'
async function loadSummary(){summaryBusy.value=true;try{summary.value=await request<Summary>('/logs/summary')}catch(e){ElMessage.error(e instanceof Error?e.message:'日志设置读取失败')}finally{summaryBusy.value=false}}
async function load(){
 const current=++epoch;controller?.abort();controller=new AbortController();busy.value=true;error.value='';expanded.value.clear()
 const params=new URLSearchParams({page:String(page.value),pageSize:String(pageSize.value),level:level.value,module:module.value,keyword:keyword.value})
 if(before.value)params.set('before',before.value)
 if(range.value?.length===2){params.set('from',range.value[0].toISOString());params.set('to',range.value[1].toISOString())}
 try{const out=await request<LogPage>('/logs/query?'+params,{signal:controller.signal});if(current!==epoch)return;rows.value=out.items;total.value=out.total;modules.value=out.modules;before.value=out.before;truncated.value=out.truncated}
 catch(e){if(current!==epoch||e instanceof DOMException&&e.name==='AbortError')return;rows.value=[];total.value=0;error.value=e instanceof Error?e.message:'日志读取失败'}
 finally{if(current===epoch)busy.value=false}
}
function refresh(){if(timer)clearTimeout(timer);before.value='';page.value=1;void load()}
watch(keyword,()=>{if(timer)clearTimeout(timer);timer=setTimeout(refresh,350)})
async function clear(){
 try{await ElMessageBox.confirm('清理全部程序文件日志？数据库审计、Docker启动控制台日志不会删除。','清理日志',{type:'warning',confirmButtonText:'确认清理',cancelButtonText:'取消'})}catch{return}
 try{await post('/logs/clear',{confirm:true});refresh();await loadSummary();ElMessage.success('程序文件日志已清理')}catch(e){error.value=e instanceof Error?e.message:'清理失败'}
}
async function change(value:string){try{await post('/logs/level',{level:value});await loadSummary();refresh()}catch(e){ElMessage.error(e instanceof Error?e.message:'设置失败')}}
function toggle(index:number){if(expanded.value.has(index))expanded.value.delete(index);else expanded.value.add(index)}
async function copy(){
 const text=logText(rows.value,clean.value)
 try{if(!navigator.clipboard?.writeText)throw new Error('unavailable');await navigator.clipboard.writeText(text);ElMessage.success('当前页原始日志已复制')}
 catch{copyContent.value=text;copyOpen.value=true}
}
function download(){
 const blob=new Blob([logText(rows.value,clean.value)],{type:'text/plain;charset=utf-8'});const url=URL.createObjectURL(blob),link=document.createElement('a');link.href=url;link.download='mediatool-logs-'+new Date().toISOString().slice(0,10)+'.txt';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)
}
onMounted(()=>{void loadSummary();void load()})
onUnmounted(()=>{++epoch;controller?.abort();if(timer)clearTimeout(timer)})
</script>
<template>
 <section class="log-workspace" aria-label="系统日志控制台">
  <header class="log-toolbar">
   <div class="log-title"><h1>程序日志</h1><span>系统日志控制台</span></div>
   <div class="log-actions">
    <div class="clean-toggle"><el-switch id="log-clean-mode" v-model="clean" aria-label="简约模式"/><label for="log-clean-mode">简约模式</label></div>
    <el-button class="log-icon" :icon="Refresh" :loading="busy" aria-label="刷新日志" title="刷新日志" @click="refresh"/>
    <el-button class="log-icon" :icon="Delete" type="danger" plain aria-label="清理日志" title="清理日志" :disabled="busy" @click="clear"/>
    <el-button class="log-icon" :icon="CopyDocument" aria-label="复制当前页日志" title="复制当前页原始日志" :disabled="!rows.length||busy" @click="copy"/>
    <el-button class="log-icon" :icon="Download" aria-label="下载当前页日志" title="下载当前页原始日志" :disabled="!rows.length||busy" @click="download"/>
    <el-button class="log-icon" :icon="Setting" aria-label="日志设置" title="大小、文件与运行级别" @click="settingsOpen=true;loadSummary()"/>
   </div>
  </header>
  <form class="log-filters" @submit.prevent="refresh">
   <div class="log-filter"><label for="log-keyword">关键词</label><el-input id="log-keyword" v-model="keyword" :maxlength="200" clearable :prefix-icon="Search" placeholder="搜索消息或结构化字段" aria-label="日志关键词"/></div>
   <div class="log-filter"><label for="log-level">级别</label><el-select class="log-level-select" placeholder="全部" id="log-level" v-model="level" aria-label="日志级别筛选" @change="refresh"><el-option label="全部" value=""/><el-option v-for="x in ['debug','info','warn','error']" :key="x" :label="x.toUpperCase()" :value="x"/></el-select></div>
   <div class="log-filter"><label for="log-module">模块</label><el-select class="log-module-select" placeholder="全部" id="log-module" v-model="module" aria-label="日志模块筛选" @change="refresh"><el-option label="全部" value=""/><el-option v-for="x in modules" :key="x" :label="x==='app'?'app / 未标注来源':x" :value="x"/></el-select></div>
   <div class="log-filter"><label for="log-time-start">时间范围</label><el-date-picker :id="['log-time-start','log-time-end']" v-model="range" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" range-separator="至" aria-label="日志时间范围" @change="refresh"/></div>
  </form>
  <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon/>
  <div class="log-window-note"><span>仅已登录管理员可见原始日志（含完整链接），请勿公开分享 · 本地时区 · 最新写入在前</span><span v-if="summary">{{bytes(summary.totalBytes)}} / {{bytes(summary.limitBytes)}}</span></div>
  <div class="log-console" :class="{'is-clean':clean}" tabindex="0" aria-label="日志内容" :aria-busy="busy">
   <div v-if="busy" class="console-state" role="status">正在读取日志…</div>
   <div v-else-if="error" class="console-state">读取失败，请使用上方刷新按钮重试。</div>
   <div v-else-if="!rows.length" class="console-state">没有符合筛选条件的日志</div>
   <template v-else><article v-for="(row,index) in rows" :key="index" class="log-entry log-row">
    <el-button v-if="row.url||row.location" size="small" text @click.stop="linkRow=row;linkOpen=true">查看请求与跳转链接</el-button><div class="log-line"><time :datetime="row.time">{{formatLogTime(row.time)}}</time><span class="severity" :class="logLevel(row.level)">{{logLevel(row.level)}}</span><div class="log-message"><span>{{row.msg}}</span><span v-if="!clean" class="log-inline-fields"> {{inlineLogFields(row)}}</span><span v-if="!clean&&logSource(row)" class="log-source"> {{logSource(row)}}</span></div>
     <button v-if="!clean" class="log-detail-toggle" :aria-label="`查看第${index+1}条日志详情`" :aria-expanded="expanded.has(index)" @click="toggle(index)"><ArrowDown :class="{'is-open':expanded.has(index)}" aria-hidden="true"/></button>
    </div>
    <div v-if="!clean&&expanded.has(index)" class="log-details"><span class="log-detail-label">模块 {{row.module||'app'}} · 来源 {{logSource(row)||'旧日志未记录来源'}}</span><pre>{{JSON.stringify(logFields(row),null,2)}}</pre></div>
   </article></template>
  </div>
  <footer class="log-pagination"><div class="log-window-scope"><strong>当前检索窗口共 {{total}} 条</strong><span v-if="truncated">仅检索每文件末尾 2 MiB、最多 20000 条；不代表全部保留日志。</span><span v-else>分页复用查询截止时间；大量写入、轮转或清理后可刷新重读。</span></div><div class="log-pager-controls"><div class="page-size-control"><label for="log-page-size">每页行数</label><el-select id="log-page-size" v-model="pageSize" aria-label="日志每页行数" @change="refresh"><el-option v-for="n in [50,100,200]" :key="n" :label="String(n)" :value="n"/></el-select></div><el-pagination v-model:current-page="page" :page-size="pageSize" :total="total" :pager-count="5" layout="prev,pager,next" :disabled="busy" @current-change="load"/></div></footer>
 </section>
 <el-drawer v-model="settingsOpen" title="日志设置" size="min(480px,95vw)" class="log-settings"><div v-loading="summaryBusy"><section class="log-size-info"><h2>日志大小</h2><strong>{{summary?bytes(summary.totalBytes):'—'}}</strong><p>上限 {{summary?bytes(summary.limitBytes):'—'}} · 自动轮转</p></section><label for="runtime-log-level" class="runtime-level-label">本次运行日志级别</label><el-select id="runtime-log-level" :model-value="summary?.level" @change="change"><el-option v-for="x in ['debug','info','warn','error']" :key="x" :label="x.toUpperCase()" :value="x"/></el-select><p class="field-hint">永久默认值在 Compose 配置。更改不回填过去的日志。恢复临时密码不进入此页面。</p><h2>保留文件</h2><div v-for="file in summary?.files" :key="file.name" class="log-retained-file"><strong>{{file.name}}</strong><span>{{bytes(file.size)}}</span><time>{{formatLogTime(file.modified)}}</time></div></div></el-drawer>
 <el-dialog v-model="copyOpen" title="复制原始日志" width="min(740px,94vw)"><p class="field-hint">当前浏览器不支持直接写入剪贴板，请选中下方文本并复制。</p><el-input v-model="copyContent" type="textarea" readonly :rows="12" aria-label="待复制日志"/><template #footer><el-button @click="copyOpen=false">关闭</el-button></template></el-dialog>
<el-dialog v-model="linkOpen" title="完整请求与302链接" width="min(900px,94vw)"><p>原始请求</p><el-input :model-value="String(linkRow?.url??'')" readonly type="textarea" :rows="3"/><p>跳转目标 / Location</p><el-input :model-value="String(linkRow?.location??'无跳转')" readonly type="textarea" :rows="3"/><p>请求ID {{linkRow?.requestId}} · 状态 {{linkRow?.status}} · 耗时 {{linkRow?.durationMs}} ms</p><p class="field-hint">链接含完整参数，仅管理员可见，请勿公开分享。查看不会发起媒体请求。</p></el-dialog></template>
<style scoped>
.log-workspace{background:var(--panel-bg);border:1px solid var(--border);border-radius:12px;overflow:hidden;min-width:0}
.log-toolbar{display:flex;align-items:center;justify-content:space-between;padding:18px 20px;gap:16px}
.log-title{display:flex;align-items:baseline;gap:14px;min-width:0}.log-title h1{font-size:19px;letter-spacing:0;white-space:nowrap}.log-title>span{font-size:12px;color:var(--muted)}
.log-actions{display:flex;align-items:center;gap:6px;flex-wrap:wrap}.log-actions .el-button+.el-button{margin-left:0}.log-icon{width:44px;padding:0;background:none;border-color:transparent}.log-icon:hover{background:var(--el-color-primary-light-9);border-color:var(--border)}.clean-toggle{display:flex;gap:8px;align-items:center;font-size:13px;margin-right:10px;white-space:nowrap}
.log-filters{display:grid;grid-template-columns:1fr .75fr .9fr 1.5fr;gap:18px;padding:8px 20px 18px;min-width:0}.log-filter{display:flex;flex-direction:column;gap:6px;min-width:0;font-size:12px;color:var(--muted)}.log-filter .el-select{width:100%}.log-filter :deep(.el-date-editor){width:100%;max-width:100%;min-width:0;height:44px}.log-filter :deep(.el-range-input){min-width:0;font-size:12px}.log-filter :deep(.el-range-separator){flex:0;min-width:26px}
.log-window-note{display:flex;justify-content:space-between;gap:12px;padding:0 20px 12px;font-size:11px;color:var(--muted)}
.log-console{background:#151b23;color:#e5e9f0;min-height:340px;height:clamp(340px,calc(100dvh - 430px),800px);overflow:auto;overscroll-behavior:contain;scrollbar-width:thin;scrollbar-color:#526170 #151b23;font-family:"SFMono-Regular",Consolas,"Liberation Mono","PingFang SC",monospace;font-size:12px;line-height:1.6;--focus:#8fc6ff}
.log-row{padding:0;border-bottom:1px solid #2b3541;display:block}.log-row:hover{background:#1d2631}.log-line{display:grid;grid-template-columns:158px 48px minmax(0,1fr) 28px;gap:10px;align-items:start;padding:3px 12px;min-width:0;min-height:27px}.log-line>time{font-variant-numeric:tabular-nums;white-space:nowrap;color:#c4ccd6;font-size:12px}.severity{font-weight:700;font-size:12px;letter-spacing:.5px}.severity.debug{color:#b6bfca}.severity.info{color:#65acff}.severity.warn{color:#ffd37a}.severity.error{color:#ff8c82}.severity.unknown{color:#c4ccd6}
.log-message{overflow-wrap:anywhere;white-space:pre-wrap;min-width:0}.log-inline-fields{color:#c4ccd6;margin-left:8px}.log-source{color:#92a7be;margin-left:16px}.log-detail-toggle{display:grid;place-items:center;background:none;color:#9fb6cd;padding:0;width:26px;height:24px;border-radius:3px;align-self:start}.log-detail-toggle:hover{background:#304152;color:#fff}.log-detail-toggle svg{width:12px;height:12px;transition:transform .16s}.log-detail-toggle svg.is-open{transform:rotate(180deg)}
.log-details{padding:10px 12px 12px 238px;background:#1a2430}.log-detail-label{font-size:11px;color:#aebed1}.log-details pre{margin:8px 0 0;padding:0;background:none;border-radius:0;white-space:pre-wrap;overflow-wrap:anywhere;color:#d6e0eb;font-size:12px;line-height:1.6}.is-clean .log-line{grid-template-columns:158px 48px minmax(0,1fr)}
.console-state{padding:36px 20px;text-align:center;color:#b9c8d8;font-family:inherit;font-size:13px}
.log-pagination{display:flex;justify-content:space-between;align-items:center;gap:18px;padding:14px 20px}.log-window-scope{display:flex;flex-direction:column;gap:4px;max-width:480px;color:var(--muted);font-size:11px}.log-window-scope strong{font-size:12px;color:var(--text);font-weight:500}.log-pager-controls{display:flex;align-items:center;gap:18px}.page-size-control{display:flex;gap:8px;align-items:center;font-size:12px;color:var(--muted);white-space:nowrap}.page-size-control .el-select{width:90px}.log-pagination .el-pagination{margin:0}
.log-size-info{padding-bottom:24px;margin-bottom:24px;border-bottom:1px solid var(--border)}.log-size-info>strong{display:block;font-size:30px;margin:12px 0}.log-size-info p{font-size:13px;color:var(--muted)}.runtime-level-label{display:block;margin-bottom:12px;font-size:14px}.log-settings .el-select{width:100%}.log-retained-file{display:grid;grid-template-columns:1fr auto;gap:8px;font-size:13px;padding:16px 0;border-bottom:1px solid var(--border)}.log-retained-file time{grid-column:1/-1;color:var(--muted);font-size:12px}
@media(max-width:1200px){.log-filters{grid-template-columns:1fr 1fr}.log-pagination{flex-wrap:wrap}.log-title>span{display:none}.log-details{padding-left:24px}}
@media(max-width:680px){.log-toolbar{padding:14px 12px;flex-wrap:wrap;gap:10px}.log-actions{width:100%;justify-content:space-between;gap:2px}.clean-toggle{margin-right:0;font-size:12px;gap:5px}.log-icon{width:40px;min-width:36px}.log-filters{padding:4px 12px 14px;gap:12px}.log-filter:first-child,.log-filter:last-child{grid-column:1/-1}.log-window-note{padding-inline:12px;flex-wrap:wrap;gap:4px}.log-console{height:450px;min-height:280px}.log-line,.is-clean .log-line{grid-template-columns:142px 42px minmax(0,1fr);gap:6px;padding:6px 10px}.log-line>time{font-size:11px}.log-message{grid-column:1/-1}.log-detail-toggle{grid-column:3;grid-row:1;justify-self:end;width:44px;height:36px;margin-top:-4px}.log-line{min-height:54px}.log-details{padding:12px}.log-pagination{padding:12px;gap:14px}.log-pager-controls{width:100%;gap:10px;flex-wrap:wrap;justify-content:space-between}.log-pagination .el-pagination{max-width:100%}}
</style>
