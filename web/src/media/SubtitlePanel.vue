<script setup lang="ts">
import {ref,reactive,watch} from 'vue';
import {requestUUID} from '../utils/request-uuid';
import {listSubtitles,uploadSubtitle,saveSubtitle,deleteSubtitle,subtitleDownload,type Subtitle,type SubtitleMetadata} from '../api/subtitles';
import {useResource} from '../workflows/useResource';
import {useOperation} from '../workflows/useOperation';
import {errorText} from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
const props=defineProps<{assetId:string}>(),asset=ref(props.assetId),data=reactive(useResource(()=>listSubtitles(asset.value),{assetId:'',versions:[],items:[]})),op=reactive(useOperation('subtitles')),file=ref<File>(),uploadId=ref(''),encoding=ref('auto'),input=ref<HTMLInputElement>(),notice=ref(''),editing=ref<Subtitle>(),form=reactive<SubtitleMetadata>({language:'',title:'',default:false,forced:false,hidden:false,revision:1}),remove=ref<Subtitle>(),confirm=ref(false);
const origin=(s:string)=>({automatic:'自动识别',embedded:'内嵌',uploaded:'已上传'}[s]||'待核对');
async function load(){await data.load();if(op.pending&&!data.error&&data.data.assetId===asset.value){let p;try{p=JSON.parse(op.pending.key)}catch{op.clear();return}if(p.asset!==asset.value)return;const row=data.data.items.find(s=>s.id===p.id);if(p.kind==='delete'&&(!row||row.deleting)||p.kind==='upload'&&!!row||p.kind==='save'&&row&&row.revision!==p.revision)op.clear()}}
watch(()=>props.assetId,v=>{asset.value=v});watch(asset,()=>{editing.value=undefined;file.value=undefined;notice.value='';if(input.value)input.value.value='';void load()},{immediate:true});
function cancelPending(){op.clear();file.value=undefined;if(input.value)input.value.value='';notice.value='已核对列表，请重新选择文件或操作。'}
function choose(event:Event){file.value=(event.target as HTMLInputElement).files?.[0];notice.value='';uploadId.value=requestUUID();if(file.value&&(!/\.(srt|ass|ssa|vtt)$/i.test(file.value.name)||file.value.size>5*1024*1024)){notice.value='仅支持 srt/ass/ssa/vtt，最大 5MiB。';file.value=undefined;if(input.value)input.value.value=''}}
async function upload(){if(!file.value||op.pending)return;const selected=file.value,target=asset.value,id=uploadId.value;const out=await op.run(JSON.stringify({kind:'upload',asset:target,id,name:selected.name}),()=>uploadSubtitle(target,selected,encoding.value,id));if(asset.value!==target)return;if(out){notice.value=`已上传并转为 UTF-8，识别编码：${out.encoding||'UTF-8'}。`;file.value=undefined;if(input.value)input.value.value='';await load()}else if(op.pending){await load();notice.value='上传结果待核对，请查看字幕列表；不要重复上传。'}}
function edit(row:Subtitle){editing.value=row;Object.assign(form,{language:row.language,title:row.title,default:row.default,forced:row.forced,hidden:row.hidden,revision:row.revision});op.error=null}
async function save(row=editing.value,value:SubtitleMetadata=form){if(!row||op.pending)return;const target=asset.value;const out=await op.run(JSON.stringify({kind:'save',asset:target,id:row.id,revision:row.revision}),()=>saveSubtitle(target,row.id,{...value,revision:row.revision}));if(asset.value!==target)return;await load();if(out)editing.value=undefined}
async function applyDelete(){if(!remove.value)return;const row=remove.value,target=asset.value;await op.run(JSON.stringify({kind:'delete',asset:target,id:row.id,revision:row.revision}),()=>deleteSubtitle(target,row.id,row.revision));if(asset.value!==target)return;confirm.value=false;await load()}
</script>
<template>
<section class="subtitle-panel" aria-label="字幕管理">
 <h3>字幕</h3><p>按物理版本管理；自动字幕走直链，上传副本仅存本程序，不改媒体或 STRM。</p>
 <label class="subtitle-field">物理版本<select v-model="asset" aria-label="字幕物理版本" :disabled="!!op.pending"><option v-for="v in data.data.versions" :key="v.id" :value="v.id">{{v.name}}</option></select></label>
 <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" @reload="load"/>
 <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
 <AsyncResultBanner v-if="op.phase" :status="op.phase" message="正在核对上次操作，不重复提交。" @refresh="load"/>
 <el-button v-if="op.pending&&data.loaded&&!data.error" @click="cancelPending">已核对，取消待定操作</el-button>
 <div class="subtitle-upload">
  <div class="subtitle-field"><span>上传字幕</span><div class="subtitle-file"><el-button :disabled="!!op.pending||!!data.error" @click="input?.click()">选择字幕文件</el-button><span :title="file?.name">{{file?.name||'未选择文件'}}</span><input ref="input" type="file" accept=".srt,.ass,.ssa,.vtt" aria-label="上传字幕文件" hidden :disabled="!!op.pending||!!data.error" @change="choose"/></div></div>
  <label class="subtitle-field">原文件编码<select v-model="encoding" aria-label="字幕编码" :disabled="!!op.pending"><option value="auto">自动检测</option><option v-for="v in ['utf-8','utf-16le','utf-16be','gb18030','big5','shift-jis','windows-1252']" :key="v" :value="v">{{v.toUpperCase()}}</option></select></label>
  <p>支持 srt / ass / ssa / vtt，最大 5MiB。检测 Unicode、UTF-8，并按文件名尝试旧编码；疑似乱码请指定原编码重新上传。</p>
  <el-button :disabled="!file||!!op.pending||!!data.error" :loading="op.busy" @click="upload">上传字幕</el-button>
 </div>
 <p v-if="notice" role="status">{{notice}}</p>
 <p v-if="data.loaded&&!data.error&&!data.data.items.length" class="subtitle-empty">此版本暂无字幕，可上传自己的字幕文件。</p>
 <p v-if="data.data.items.some(row=>row.source==='embedded')" class="subtitle-embedded-note">内嵌字幕随视频播放，仅编辑显示信息，不提供提取或删除。</p>
 <article v-for="row in data.data.items" :key="row.id" class="subtitle-row">
  <header><strong :title="row.filename">{{row.title||row.filename}}</strong><el-tag>{{origin(row.source)}}</el-tag></header>
  <p>{{row.language||'语言未标注'}} · {{row.extension.toUpperCase()}}<span v-if="row.default"> · 默认</span><span v-if="row.forced"> · 强制</span><span v-if="row.hidden"> · 已隐藏</span><span v-if="row.deleting"> · 副本清理待重试</span></p>
  <div v-if="editing?.id!==row.id" class="subtitle-actions">
   <el-button :disabled="!!op.pending||row.deleting" @click="edit(row)">编辑字幕信息</el-button>
   <a v-if="row.source!=='embedded'&&!row.hidden&&!row.deleting" class="el-button" :href="subtitleDownload(asset,row.id)" target="_blank" rel="noopener noreferrer">下载字幕</a>
   <el-button v-if="row.source==='automatic'" :disabled="!!op.pending" @click="save(row,{...row,hidden:!row.hidden})">{{row.hidden?'恢复显示':'隐藏字幕'}}</el-button>
   <el-button v-if="row.source==='uploaded'" :disabled="!!op.pending" @click="remove=row;confirm=true">{{row.deleting?'重试清理':'删除上传副本'}}</el-button>
  </div>
  <form v-else class="subtitle-editor" @submit.prevent="save()">
   <label class="subtitle-field">语言<input v-model="form.language" aria-label="字幕语言" maxlength="16" placeholder="例如 chs / cht / eng / jpn"/></label>
   <label class="subtitle-field">标题<input v-model="form.title" aria-label="字幕标题" maxlength="200"/></label>
   <div class="subtitle-checks"><label><input v-model="form.default" type="checkbox"/>默认字幕</label><label><input v-model="form.forced" type="checkbox"/>强制字幕</label></div>
   <div class="subtitle-actions"><el-button :disabled="!!op.pending" @click="editing=undefined">取消编辑</el-button><el-button native-type="submit" :disabled="!!op.pending" :loading="op.busy">保存字幕信息</el-button></div>
  </form>
 </article>
 <ConfirmAction v-model:visible="confirm" title="删除上传字幕副本" :description="`确认删除 ${remove?.title||remove?.filename||'此字幕'}？`" impact-scope="仅删除本程序登记和上传副本；不会删除或修改网盘文件、媒体及 STRM。" confirm-text="删除副本" :loading="op.busy" @confirm="applyDelete"/>
</section>
</template>
<style scoped>
.subtitle-panel{border-top:1px solid var(--mt-border-color);padding-top:20px;margin-top:20px;min-width:0}.subtitle-panel p{margin:10px 0;color:var(--mt-text-muted);font-size:13px;line-height:1.7;overflow-wrap:anywhere}.subtitle-field{display:grid;gap:8px;font-size:13px;min-width:0}.subtitle-field input,.subtitle-field select{min-height:44px;max-width:100%;width:100%;padding:var(--mt-field-padding);border:1px solid var(--mt-border-color);border-radius:var(--el-border-radius-base);background:var(--mt-bg-surface);color:var(--mt-text-primary);font:inherit}.subtitle-upload{display:grid;gap:12px;margin:20px 0;padding:16px;border:1px solid var(--mt-border-color);border-radius:8px}.subtitle-row{border-top:1px solid var(--mt-border-color);padding:16px 0;min-width:0}.subtitle-row header,.subtitle-actions,.subtitle-checks{display:flex;align-items:center;gap:12px;flex-wrap:wrap}.subtitle-row strong{min-width:0;flex:1;overflow-wrap:anywhere}.subtitle-actions :deep(.el-button){min-height:44px;margin:0}.subtitle-actions a{color:var(--el-button-text-color);text-decoration:none}.subtitle-editor{display:grid;gap:12px}.subtitle-checks label{display:flex;gap:8px;align-items:center;min-height:44px}.subtitle-checks input{width:20px;height:20px}.subtitle-empty{padding:16px;border:1px dashed var(--mt-border-color);border-radius:8px}
.subtitle-file{display:flex;align-items:center;gap:12px;min-width:0}.subtitle-file span{flex:1;min-width:0;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}.subtitle-file input[hidden]{display:none}
</style>
