<script setup lang="ts">
import {fileDataURL} from '../utils/file-data-url';
import FilePicker from '../ui/FilePicker.vue';
import {computed,ref,watch,onBeforeUnmount} from 'vue';
import {APIError} from '../api/client';
import './editor.css';
import {readMediaImages,readImageCandidates,uploadMediaImage,changeMediaImage,imageKinds,type ImageKind,type ImageRecord,type ImageCandidates} from '../api/mediaImages';
import {metadataTargetID,type MetadataTarget} from '../api/mediaMetadata';
import {errorText} from '../workflows/readiness';
const props=withDefaults(defineProps<{itemId:string;hideTrigger?:boolean;externalControls?:boolean;deferred?:boolean}>(),{hideTrigger:false}),emit=defineEmits<{saved:[]}>();
const open=ref(false),record=ref<ImageRecord>(),target=ref(''),busy=ref(false),error=ref(''),notice=ref(''),blocked=ref(false),picker=ref<ImageKind>(),candidates=ref<ImageCandidates>(),broken=ref(new Set<string>());
type DraftImage={file?:File;path?:string;url?:string};
const draftImages=ref(new Map<string,{base:ImageRecord;changes:Partial<Record<ImageKind,DraftImage>>}>());
const dirty=computed(()=>[...draftImages.value.values()].some(v=>Object.keys(v.changes).length>0));
const hasSaved=ref(false);
function revoke(change?:DraftImage){if(change?.file&&change.url?.startsWith('blob:'))URL.revokeObjectURL(change.url)}
function discard(){for(const [id,draft]of draftImages.value){for(const change of Object.values(draft.changes))revoke(change);if(id===selectedId.value)record.value=JSON.parse(JSON.stringify(draft.base))}draftImages.value.clear();picker.value=undefined;notice.value='已撤回未保存修改';}
onBeforeUnmount(()=>{generation++;for(const draft of draftImages.value.values())for(const change of Object.values(draft.changes))revoke(change)});
function staged(kind:ImageKind){return draftImages.value.get(selectedId.value)?.changes[kind]}
function stage(kind:ImageKind,change:DraftImage){if(!record.value)return;let draft=draftImages.value.get(selectedId.value);if(!draft){draft={base:JSON.parse(JSON.stringify(record.value)),changes:{}};draftImages.value.set(selectedId.value,draft)}revoke(draft.changes[kind]);draft.changes[kind]=change;record.value={...record.value,effective:{...record.value.effective,[kind]:change.file||change.path?{url:change.url||''}:record.value.base[kind]},overrides:{...record.value.overrides,[kind]:'draft'}};picker.value=undefined;notice.value='图片修改尚未保存';}
async function restoreImage(kind:ImageKind){if(props.deferred){stage(kind,{});return}await save(()=>changeMediaImage(selectedId.value,change(kind)))}
async function selectCandidate(kind:ImageKind,candidate:{path:string;url:string}){if(props.deferred){stage(kind,{path:candidate.path,url:candidate.url});return}await save(()=>changeMediaImage(selectedId.value,change(kind),candidate.path))}
async function saveDraft(){
 if(!props.deferred||!dirty.value)return true;if(blocked.value||busy.value)return false;
 const version=++generation;busy.value=true;error.value='';
 try{for(const [id,draft]of draftImages.value){for(const kind of imageKinds){const pending=draft.changes[kind];if(!pending)continue;const request={kind,targetKey:draft.base.targetKey,revision:draft.base.revision};const out=pending.file?await uploadMediaImage(id,request,pending.file):await changeMediaImage(id,request,pending.path||'');if(version!==generation)return false;draft.base=JSON.parse(JSON.stringify(out));hasSaved.value=true;delete draft.changes[kind];revoke(pending);if(id===selectedId.value){record.value=JSON.parse(JSON.stringify(out));for(const [pendingKind,value]of Object.entries(draft.changes)){record.value!.effective[pendingKind as ImageKind]=value.file||value.path?{url:value.url||''}:out.base[pendingKind as ImageKind];record.value!.overrides[pendingKind as ImageKind]='draft'}}}if(!Object.keys(draft.changes).length)draftImages.value.delete(id)}notice.value='图片已保存';return true;}catch(e){if(version===generation){failed(e);if(!(e instanceof APIError)||e.status>=500)blocked.value=true;}return false;}finally{if(version===generation)busy.value=false;}
}
let generation=0;
const selectedId=computed(()=>target.value||props.itemId);
const label=(kind:ImageKind)=>kind==='Primary'?(record.value?.target.kind==='episode'?'单集缩略图':'海报'):kind==='Backdrop'?'背景图':'Logo';
const scopeLabel=(t:MetadataTarget)=>t.kind==='movie'?'电影':t.kind==='series'?'剧集':t.kind==='season'?`第 ${t.season} 季`:`第 ${t.season} 季 · 第 ${t.episode} 集`;
const scopes=computed(()=>{if(!record.value)return [];const t=record.value.target,list=[t,...record.value.choices];if(t.kind==='episode')list.push({...t,kind:'season',episode:-1});if(t.kind==='episode'||t.kind==='season')list.push({...t,kind:'series',season:-1,episode:-1});return [...new Map(list.map(x=>[metadataTargetID(x),x])).values()]});
watch(()=>props.itemId,()=>{discard();hasSaved.value=false;generation++;busy.value=false;open.value=false;record.value=undefined;target.value='';error.value='';notice.value='';picker.value=undefined;candidates.value=undefined;blocked.value=false});
function failed(e:unknown){error.value=errorText(e);if(e instanceof APIError&&e.status===409)blocked.value=true;}
async function load(){const version=++generation,id=selectedId.value;busy.value=true;error.value='';picker.value=undefined;candidates.value=undefined;try{const out=await readMediaImages(id);if(version!==generation)return;record.value=out;if(props.deferred){const draft=draftImages.value.get(id);if(draft){draft.base=JSON.parse(JSON.stringify(out));for(const [kind,value]of Object.entries(draft.changes)){record.value.effective[kind as ImageKind]=value.file||value.path?{url:value.url||''}:out.base[kind as ImageKind];record.value.overrides[kind as ImageKind]='draft'}}}blocked.value=false;}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}}
async function toggle(){open.value=!open.value;if(open.value&&!record.value)await load()}
async function openEditor(){if(!open.value)await toggle()}
function close(){open.value=false;}
function isOpen(){return open.value;}
defineExpose({open:openEditor,close,isOpen,busy,dirty,saveDraft,discard,hasSaved,blocked});
async function chooseScope(id:string){target.value=id;const draft=props.deferred?draftImages.value.get(id):undefined;if(draft){record.value=JSON.parse(JSON.stringify(draft.base));for(const [kind,value]of Object.entries(draft.changes)){record.value!.effective[kind as ImageKind]=value.file||value.path?{url:value.url||''}:draft.base.base[kind as ImageKind];record.value!.overrides[kind as ImageKind]='draft'}picker.value=undefined;}else await load()}
function change(kind:ImageKind){return {kind,targetKey:record.value!.targetKey,revision:record.value!.revision}}
async function upload(kind:ImageKind,file:File){if(!file||!record.value)return;if(file.size>10*1024*1024){error.value='图片不能超过10MiB';return}if(props.deferred){const version=generation;busy.value=true;try{const url=await fileDataURL(file);if(version===generation)stage(kind,{file,url})}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}return}await save(()=>uploadMediaImage(selectedId.value,change(kind),file));}
async function save(action:()=>Promise<ImageRecord>){const version=++generation;busy.value=true;error.value='';notice.value='';try{const out=await action();if(version!==generation)return;record.value={...out,choices:out.choices.length?out.choices:record.value?.choices||[]};picker.value=undefined;notice.value='图片已保存';emit('saved');}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}}
async function showCandidates(kind:ImageKind){const version=++generation,id=selectedId.value;picker.value=kind;busy.value=true;error.value='';try{const out=await readImageCandidates(id);if(version===generation)candidates.value=out}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}}
</script>
<template>
<section v-if="open||!props.hideTrigger" class="image-editor">
 <header v-if="!externalControls&&(open||!props.hideTrigger)"><h3>图片</h3><el-button :aria-expanded="open" :disabled="busy" @click="toggle">{{open?'完成编辑':'编辑图片'}}</el-button></header>
 <template v-if="open">
  <p class="image-note">支持 JPG、PNG、WebP，最大10MiB。人工图片单独锁定，恢复后使用当前刮削值，不修改网盘、STRM 或 NFO。</p>
  <p v-if="error" role="alert" class="image-error">{{error}} <el-button :disabled="busy" @click="load">重新读取并核对</el-button></p><p v-if="notice" role="status">{{notice}}</p>
  <p v-if="busy" role="status">正在处理图片…</p>
  <template v-if="record">
   <label v-if="scopes.length>1" class="image-scope">编辑范围 <el-select :model-value="metadataTargetID(record.target)" :disabled="busy" @change="chooseScope($event)"><el-option v-for="t in scopes" :key="metadataTargetID(t)" :value="metadataTargetID(t)" :label="(scopeLabel(t))"/></el-select></label>
   <p v-if="!record.matched" class="image-note">未匹配：此季／集编号当前没有对应资产，人工图片仍保留。</p>
   <details v-if="record.unmatched.length"><summary>未匹配的人工图片（最多50项）</summary><el-button v-for="t in record.unmatched" :key="metadataTargetID(t)" :disabled="busy" @click="chooseScope(metadataTargetID(t))">{{scopeLabel(t)}} · 未匹配</el-button></details>
   <div v-for="kind in imageKinds" :key="kind" class="image-row">
    <div class="image-label"><strong>{{label(kind)}}</strong><span class="metadata-lock">{{staged(kind)?(staged(kind)?.file||staged(kind)?.path?'待保存':'待恢复'):record.overrides[kind]?'人工锁定':record.base[kind]?'刮削值':'暂无刮削值'}}</span></div>
    <div class="image-preview" :class="{'image-preview-poster':kind==='Primary'&&record.target.kind!=='episode'}"><img v-if="record.effective[kind]?.url&&!broken.has(record.effective[kind]!.url)" :src="record.effective[kind]!.url" :alt="'当前'+label(kind)" loading="lazy" @error="broken.add(record.effective[kind]!.url)"/><span v-else>暂无图片</span></div>
    <div class="image-actions"><FilePicker button-text="本地上传" accept=".jpg,.jpeg,.png,.webp" :label="'上传'+label(kind)" :disabled="busy||blocked" @select="upload(kind,$event)"/><el-button :disabled="busy||blocked" @click="showCandidates(kind)">TMDB 候选</el-button></div>
    <el-button class="image-restore" :disabled="busy||blocked||!record.overrides[kind]" @click="restoreImage(kind)">恢复为刮削值</el-button>
   </div>
   <section v-if="picker" class="image-picker" aria-label="TMDB 图片候选"><header><h4>{{label(picker)}}候选</h4><el-button :disabled="busy" @click="picker=undefined">关闭候选</el-button></header><p v-if="!busy&&!candidates?.[picker]?.length" class="image-note">暂无可用候选，请使用本地上传。</p><div class="image-candidates"><button v-for="(candidate,index) in candidates?.[picker]||[]" :key="candidate.path" :disabled="busy||blocked" :aria-label="'选择候选 '+(index+1)" @click="selectCandidate(picker!,candidate)"><img :src="candidate.url" :alt="label(picker)+'候选 '+(index+1)" loading="lazy"/></button></div></section>
  </template>
 </template>
</section>
</template>
<style scoped>
.image-editor{margin-block:16px;min-width:0}.image-editor header{display:flex;justify-content:space-between;align-items:center;gap:12px}.image-editor h3,.image-editor h4{margin:0}.image-editor .el-button{margin:0;min-height:44px}.image-note{font-size:13px;color:var(--mt-text-muted)}.image-error{color:var(--el-color-danger)}.image-row{display:grid;grid-template-columns:112px 88px minmax(0,1fr) auto;align-items:center;gap:12px;padding-block:16px;border-bottom:1px solid var(--mt-border-color)}.image-preview{width:88px;height:56px;display:grid;place-items:center;overflow:hidden;border:1px solid var(--mt-border-color);border-radius:8px;color:var(--mt-text-muted);font-size:12px;background:var(--mt-bg-surface)}.image-preview-poster{width:64px;height:96px;justify-self:start}.image-preview img{width:100%;height:100%;object-fit:contain}.image-label{display:flex;flex-direction:column;gap:4px;min-width:0}.image-row .image-restore{justify-self:end}.image-actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px;min-width:0}.image-actions>*{max-width:100%}.image-actions .el-button{height:auto;white-space:normal}.image-candidates{display:grid;grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:12px;margin-top:12px}.image-candidates button{padding:0;border:1px solid var(--mt-border-color);border-radius:8px;overflow:hidden;background:var(--mt-bg-surface);cursor:pointer;min-height:44px}.image-candidates img{width:100%;height:140px;object-fit:contain}.image-scope{display:grid;gap:8px;margin-block:12px}.image-scope select{min-height:44px;width:100%;font:inherit;background:var(--mt-bg-surface);color:var(--mt-text-primary);border:1px solid var(--mt-border-color);border-radius:var(--el-border-radius-base);padding:var(--mt-field-padding)}.image-editor details{max-height:240px;overflow:auto}.image-editor details .el-button{margin-block:4px;white-space:normal;max-width:100%;height:auto}.image-editor summary{cursor:pointer;min-height:44px;line-height:44px}@media(max-width:480px){.image-row{grid-template-columns:minmax(0,1fr);gap:12px}.image-label{flex-direction:row;justify-content:space-between}.image-preview-poster,.image-row .image-restore{justify-self:start}.image-row .image-restore{max-width:100%}}
</style>
