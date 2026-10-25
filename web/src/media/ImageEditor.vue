<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import {APIError} from '../api/client';
import {readMediaImages,readImageCandidates,uploadMediaImage,changeMediaImage,imageKinds,type ImageKind,type ImageRecord,type ImageCandidates} from '../api/mediaImages';
import {metadataTargetID,type MetadataTarget} from '../api/mediaMetadata';
import {errorText} from '../workflows/readiness';
const props=defineProps<{itemId:string}>(),emit=defineEmits<{saved:[]}>();
const open=ref(false),record=ref<ImageRecord>(),target=ref(''),busy=ref(false),error=ref(''),notice=ref(''),blocked=ref(false),picker=ref<ImageKind>(),candidates=ref<ImageCandidates>(),broken=ref(new Set<string>());
let generation=0;
const selectedId=computed(()=>target.value||props.itemId);
const label=(kind:ImageKind)=>kind==='Primary'?(record.value?.target.kind==='episode'?'单集缩略图':'海报'):kind==='Backdrop'?'背景图':'Logo';
const scopeLabel=(t:MetadataTarget)=>t.kind==='movie'?'电影':t.kind==='series'?'剧集':t.kind==='season'?`第 ${t.season} 季`:`第 ${t.season} 季 · 第 ${t.episode} 集`;
const scopes=computed(()=>{if(!record.value)return [];const t=record.value.target,list=[t,...record.value.choices];if(t.kind==='episode')list.push({...t,kind:'season',episode:-1});if(t.kind==='episode'||t.kind==='season')list.push({...t,kind:'series',season:-1,episode:-1});return [...new Map(list.map(x=>[metadataTargetID(x),x])).values()]});
watch(()=>props.itemId,()=>{generation++;busy.value=false;open.value=false;record.value=undefined;target.value='';error.value='';notice.value='';picker.value=undefined;candidates.value=undefined;blocked.value=false});
function failed(e:unknown){error.value=errorText(e);if(e instanceof APIError&&e.status===409)blocked.value=true;}
async function load(){const version=++generation,id=selectedId.value;busy.value=true;error.value='';picker.value=undefined;candidates.value=undefined;try{const out=await readMediaImages(id);if(version!==generation)return;record.value=out;blocked.value=false;}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}}
async function toggle(){open.value=!open.value;if(open.value&&!record.value)await load()}
async function chooseScope(id:string){target.value=id;await load()}
function change(kind:ImageKind){return {kind,targetKey:record.value!.targetKey,revision:record.value!.revision}}
async function upload(kind:ImageKind,event:Event){const input=event.target as HTMLInputElement,file=input.files?.[0];if(!file||!record.value)return;input.value='';if(file.size>10*1024*1024){error.value='图片不能超过10MB';return}await save(()=>uploadMediaImage(selectedId.value,change(kind),file));}
async function save(action:()=>Promise<ImageRecord>){const version=++generation;busy.value=true;error.value='';notice.value='';try{const out=await action();if(version!==generation)return;record.value={...out,choices:out.choices.length?out.choices:record.value?.choices||[]};picker.value=undefined;notice.value='图片已保存';emit('saved');}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}}
async function showCandidates(kind:ImageKind){const version=++generation,id=selectedId.value;picker.value=kind;busy.value=true;error.value='';try{const out=await readImageCandidates(id);if(version===generation)candidates.value=out}catch(e){if(version===generation)failed(e)}finally{if(version===generation)busy.value=false}}
</script>
<template>
<section class="image-editor">
 <header><h3>图片</h3><el-button :aria-expanded="open" :disabled="busy" @click="toggle">{{open?'收起图片编辑':'编辑图片'}}</el-button></header>
 <template v-if="open">
  <p class="image-note">支持 JPG、PNG、WebP，最大10MB。人工图片单独锁定，恢复后使用当前刮削值，不修改网盘、STRM 或 NFO。</p>
  <p v-if="error" role="alert" class="image-error">{{error}} <el-button :disabled="busy" @click="load">重新读取并核对</el-button></p><p v-if="notice" role="status">{{notice}}</p>
  <p v-if="busy" role="status">正在处理图片…</p>
  <template v-if="record">
   <label v-if="scopes.length>1" class="image-scope">编辑范围 <select :value="metadataTargetID(record.target)" :disabled="busy" @change="chooseScope(($event.target as HTMLSelectElement).value)"><option v-for="t in scopes" :key="metadataTargetID(t)" :value="metadataTargetID(t)">{{scopeLabel(t)}}</option></select></label>
   <p v-if="!record.matched" class="image-note">未匹配：此季／集编号当前没有对应资产，人工图片仍保留。</p>
   <details v-if="record.unmatched.length"><summary>未匹配的人工图片（最多50项）</summary><el-button v-for="t in record.unmatched" :key="metadataTargetID(t)" :disabled="busy" @click="chooseScope(metadataTargetID(t))">{{scopeLabel(t)}} · 未匹配</el-button></details>
   <div v-for="kind in imageKinds" :key="kind" class="image-row">
    <div class="image-preview" :class="{'image-preview-poster':kind==='Primary'&&record.target.kind!=='episode'}"><img v-if="record.effective[kind]?.url&&!broken.has(record.effective[kind]!.url)" :src="record.effective[kind]!.url" :alt="'当前'+label(kind)" loading="lazy" @error="broken.add(record.effective[kind]!.url)"/><span v-else>暂无图片</span></div>
    <div class="image-controls"><strong>{{label(kind)}}</strong><span class="image-note">{{record.overrides[kind]?'人工锁定':record.base[kind]?'刮削值':'暂无刮削值'}}</span>
     <div class="image-actions"><label class="image-upload" :class="{disabled:busy||blocked}">本地上传<input type="file" accept=".jpg,.jpeg,.png,.webp" :aria-label="'上传'+label(kind)" :disabled="busy||blocked" @change="upload(kind,$event)"/></label><el-button :disabled="busy||blocked" @click="showCandidates(kind)">TMDB 候选</el-button><el-button :disabled="busy||blocked||!record.overrides[kind]" @click="save(()=>changeMediaImage(selectedId,change(kind)))">恢复为刮削值</el-button></div>
    </div>
   </div>
   <section v-if="picker" class="image-picker" aria-label="TMDB 图片候选"><header><h4>{{label(picker)}}候选</h4><el-button :disabled="busy" @click="picker=undefined">关闭候选</el-button></header><p v-if="!busy&&!candidates?.[picker]?.length" class="image-note">暂无可用候选，请使用本地上传。</p><div class="image-candidates"><button v-for="(candidate,index) in candidates?.[picker]||[]" :key="candidate.path" :disabled="busy||blocked" :aria-label="'选择候选 '+(index+1)" @click="save(()=>changeMediaImage(selectedId,change(picker!),candidate.path))"><img :src="candidate.url" :alt="label(picker)+'候选 '+(index+1)" loading="lazy"/></button></div></section>
  </template>
 </template>
</section>
</template>
<style scoped>
.image-editor{margin-block:16px;min-width:0}.image-editor header{display:flex;justify-content:space-between;align-items:center;gap:12px}.image-editor h3,.image-editor h4{margin:0}.image-editor .el-button{margin:0;min-height:44px}.image-note{font-size:13px;color:var(--mt-text-muted)}.image-error{color:var(--el-color-danger)}.image-row{display:grid;grid-template-columns:96px minmax(0,1fr);gap:16px;padding-block:16px;border-bottom:1px solid var(--mt-border-color)}.image-preview{aspect-ratio:16/9;display:grid;place-items:center;overflow:hidden;border:1px solid var(--mt-border-color);border-radius:8px;color:var(--mt-text-muted);font-size:12px;background:var(--mt-bg-surface)}.image-preview-poster{aspect-ratio:2/3}.image-preview img{width:100%;height:100%;object-fit:contain}.image-controls{display:flex;flex-direction:column;gap:8px;min-width:0}.image-actions{display:flex;flex-wrap:wrap;gap:8px}.image-upload{position:relative;display:grid;place-items:center;min-height:44px;box-sizing:border-box;padding:8px 12px;border:1px solid var(--mt-border-color);border-radius:var(--radius-sm);cursor:pointer;color:var(--mt-text-primary)}.image-upload input{position:absolute;inset:0;opacity:0;width:100%;height:100%;cursor:pointer}.image-upload:focus-within{outline:2px solid var(--mt-color-primary);outline-offset:2px}.image-upload.disabled{opacity:.5;cursor:not-allowed}.image-candidates{display:grid;grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:12px;margin-top:12px}.image-candidates button{padding:0;border:1px solid var(--mt-border-color);border-radius:8px;overflow:hidden;background:var(--mt-bg-surface);cursor:pointer;min-height:44px}.image-candidates img{width:100%;height:140px;object-fit:contain}.image-scope{display:grid;gap:8px;margin-block:12px}.image-scope select{min-height:44px;width:100%;font:inherit;background:var(--mt-bg-surface);color:var(--mt-text-primary);border:1px solid var(--mt-border-color);border-radius:8px;padding:8px}.image-editor details .el-button{margin-block:4px;white-space:normal;max-width:100%;height:auto}.image-editor summary{cursor:pointer;min-height:44px;line-height:44px}@media(max-width:480px){.image-row{grid-template-columns:72px minmax(0,1fr);gap:12px}}
</style>
