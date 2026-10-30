<script setup lang="ts">
import {computed,reactive,ref,watch} from 'vue';
import {APIError} from '../api/client';
import {ElMessageBox} from 'element-plus';
import 'element-plus/es/components/message-box/style/css';
import {metadataFields,readMediaMetadata,saveMediaMetadata,metadataTargetID,type MetadataField,type MetadataRecord,type MetadataTarget} from '../api/mediaMetadata';
import {inputValue,metadataValueLabel,metadataLabels,metadataPatch} from './metadata-form';
import {errorText} from '../workflows/readiness';
const unmatchedLimit=100;
const props=withDefaults(defineProps<{itemId:string;hideTrigger?:boolean}>(),{hideTrigger:false}),emit=defineEmits<{saved:[]}>();
const opened=ref(false),busy=ref(false),record=ref<MetadataRecord>(),draft=reactive(Object.fromEntries(metadataFields.map(k=>[k,''])) as Record<MetadataField,string>),restore=ref(new Set<MetadataField>()),error=ref(''),notice=ref(''),blocked=ref(false),activeID=ref(props.itemId),choices=ref<MetadataTarget[]>([]);
let epoch=0;
const patch=computed(()=>record.value?metadataPatch(record.value,draft,restore.value):undefined);
const dirty=computed(()=>!!patch.value&&(Object.keys(patch.value.set).length>0||patch.value.restore.length>0));
const label=(key:MetadataField)=>key==='title'&&record.value?.target.kind==='episode'?'集标题':metadataLabels[key];
const targetLabel=(t:MetadataTarget)=>t.kind==='episode'?`第 ${t.season} 季 · 第 ${t.episode} 集`:t.kind==='season'?`第 ${t.season} 季`:t.kind==='series'?'剧集':'电影';
function assign(r:MetadataRecord){for(const key of metadataFields)draft[key]=inputValue(r.effective[key],key);restore.value=new Set();}
async function load(keep=false){
 const version=++epoch,pending=keep&&record.value?metadataPatch(record.value,draft,restore.value):undefined;
 busy.value=true;error.value='';
 try{const out=await readMediaMetadata(activeID.value,1,unmatchedLimit);if(version!==epoch)return;
  record.value=out;assign(out);
  if(pending){for(const key of Object.keys(pending.set) as MetadataField[])draft[key]=inputValue(pending.set[key],key);restore.value=new Set(pending.restore);for(const key of pending.restore)draft[key]=inputValue(out.base[key],key);}
  if(activeID.value===props.itemId)choices.value=out.choices||[out.target];blocked.value=false;if(keep)notice.value='已读取当前版本并保留修改草稿，未改字段使用当前值，请比较后再保存。';
 }catch(e){if(version===epoch)error.value=errorText(e);}finally{if(version===epoch)busy.value=false;}
}
// External title actions resume the mounted draft when switching between editors.
function open(){if(opened.value)return;opened.value=true;if(props.hideTrigger&&record.value)return;activeID.value=props.itemId;notice.value='';blocked.value=false;void load();}
function close(){opened.value=false;}
function isOpen(){return opened.value;}
async function canClose(){
 if(busy.value)return false;
 if(!dirty.value)return true;
 try{await ElMessageBox.confirm('信息有未保存的修改，收起或切换后会保留草稿，不会自动保存。','保留未保存草稿',{confirmButtonText:'保留草稿并继续',cancelButtonText:'继续编辑',type:'warning'});return true;}catch{return false;}
}
async function requestClose(){if(await canClose())close();}
defineExpose({open,close,isOpen,busy,canClose});
function stageRestore(key:MetadataField){restore.value.add(key);draft[key]=inputValue(record.value?.base[key],key);notice.value='恢复操作尚未保存。';}
function changed(key:MetadataField){restore.value.delete(key);}
async function choose(id:string){activeID.value=id;notice.value='';await load();}
async function save(){if(!patch.value||!dirty.value||blocked.value)return;const version=epoch;busy.value=true;error.value='';try{const out=await saveMediaMetadata(activeID.value,patch.value);if(version!==epoch)return;record.value=out;assign(out);if((out.unmatchedTotal??0)>out.unmatched.length)await load();notice.value='信息已保存，修改字段已锁定；恢复字段已解除锁定。';emit('saved');}catch(e){if(version!==epoch)return;error.value=errorText(e);if(!(e instanceof APIError)||e.status>=500||e.status===409){blocked.value=true;error.value+=' 请重新读取并核对，不要重复提交。';}}finally{if(version===epoch)busy.value=false;}}
watch(()=>props.itemId,()=>{epoch++;opened.value=false;record.value=undefined;activeID.value=props.itemId;busy.value=false;blocked.value=false;});
</script>
<template>
<section v-if="opened||!props.hideTrigger" class="metadata-editor" aria-label="元数据信息">
 <el-button v-if="!opened&&!props.hideTrigger" @click="open">编辑信息</el-button>
 <template v-if="opened">
  <div class="metadata-heading"><h3>编辑信息</h3><el-button :disabled="busy" @click="requestClose">收起编辑</el-button></div>
  <p class="metadata-note">修改过的字段将逐项锁定，不会被后续刮削覆盖。同一作品或单集的物理版本共用文字信息，不改文件与目录分类。</p>
  <label v-if="choices.length>1" class="metadata-field">选择单集<select :value="record?metadataTargetID(record.target):activeID" :disabled="busy||dirty" @change="choose(($event.target as HTMLSelectElement).value)"><option v-for="t in choices" :key="metadataTargetID(t)" :value="metadataTargetID(t)">{{targetLabel(t)}}</option></select></label>
  <p v-if="busy&&!record" role="status">正在读取元数据…</p>
  <p v-if="error" class="metadata-error" role="alert">{{error}}</p>
  <p v-if="notice" role="status">{{notice}}</p>
  <el-button v-if="blocked||error&&!record" :disabled="busy" @click="load(!!record)">重新读取并核对</el-button>
  <template v-if="record">
   <p v-if="!record.matched" role="status">未匹配：当前季/集编号已无入库绑定，已保留人工信息，可继续核对。</p>
   <details v-if="(record.unmatchedTotal??record.unmatched.length)>0" class="metadata-unmatched">
    <summary>未匹配的人工信息（{{record.unmatchedTotal??record.unmatched.length}}）</summary>
    <p>编号失效的记录不会删除，也不会套用到其他季/集。</p>
    <p v-if="(record.unmatchedTotal??0)>record.unmatched.length" class="metadata-note">已列出前 {{record.unmatched.length}} 项，共 {{record.unmatchedTotal}} 项；接口单次最多 {{unmatchedLimit}} 项。</p>
    <div class="metadata-unmatched-items"><el-button v-for="t in record.unmatched" :key="metadataTargetID(t)" :disabled="busy||dirty" @click="choose(metadataTargetID(t))">{{targetLabel(t)}} · 未匹配</el-button></div>
   </details>
   <form class="metadata-form" @submit.prevent="save">
    <div v-for="key in metadataFields" :key="key" class="metadata-row" :class="{'metadata-row--overview':key==='overview'}">
     <label class="metadata-label" :for="`metadata-${key}`">{{label(key)}}<span class="metadata-lock">{{restore.has(key)?'待恢复':Object.hasOwn(record.overrides,key)?'人工锁定':'刮削值'}}</span></label>
     <div class="metadata-field">
      <textarea v-if="key==='overview'" :id="`metadata-${key}`" v-model="draft[key]" :disabled="busy" rows="5" maxlength="10000" @input="changed(key)"/>
      <input v-else :id="`metadata-${key}`" v-model="draft[key]" :disabled="busy" :type="key==='premiereDate'?'date':['year','communityRating'].includes(key)?'number':'text'" :step="key==='communityRating'?'0.1':key==='year'?'1':undefined" :min="key==='year'?1800:key==='communityRating'?0:undefined" :max="key==='year'?2100:key==='communityRating'?10:undefined" :maxlength="key==='tagline'?1000:['genres','tags'].includes(key)?undefined:500" :placeholder="['genres','tags'].includes(key)?'用顿号或逗号分隔，最多50项':undefined" @input="changed(key)"/>
      <span class="metadata-origin" :title="metadataValueLabel(record.base[key],key)">刮削值：{{metadataValueLabel(record.base[key],key)}}</span>
     </div>
     <el-button class="metadata-restore" :disabled="busy||!Object.hasOwn(record.overrides,key)&&!restore.has(key)" @click="stageRestore(key)">恢复为刮削值</el-button>
    </div>
    <div class="mt-save-actions"><el-button class="mt-save-button" type="primary" native-type="submit" :loading="busy" :disabled="busy||blocked||!dirty">保存信息</el-button><el-button :disabled="busy" @click="assign(record);notice='已撤回未保存修改'">撤回草稿</el-button></div>
   </form>
  </template>
 </template>
</section>
</template>
<style scoped>
.metadata-editor{margin-block:16px;min-width:0}.metadata-heading{display:flex;align-items:center;gap:12px;justify-content:space-between}.metadata-heading h3{margin:0}.metadata-note,.metadata-origin{color:var(--mt-text-muted);font-size:13px}
.metadata-form{display:grid;gap:16px}.metadata-row{display:grid;grid-template-columns:112px minmax(0,1fr) auto;align-items:start;gap:12px;min-width:0}.metadata-label{display:flex;flex-direction:column;align-items:flex-start;gap:2px;min-height:44px;justify-content:center}.metadata-field{display:grid;gap:4px;min-width:0}
.metadata-field input,.metadata-field textarea,.metadata-field select{width:100%;min-width:0;box-sizing:border-box;font:inherit;min-height:44px;padding:var(--mt-field-padding);border:1px solid var(--mt-border-color);border-radius:var(--el-border-radius-base);background:var(--mt-bg-surface);color:var(--mt-text-primary)}.metadata-field textarea{resize:vertical}.metadata-field input:focus-visible,.metadata-field textarea:focus-visible,.metadata-field select:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:2px}
.metadata-lock{font-size:12px;line-height:18px;color:var(--mt-text-muted);padding-inline:6px;border:1px solid var(--mt-border-color);border-radius:var(--radius-sm)}.metadata-origin{min-width:0;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}.metadata-row .metadata-restore{margin:0;min-height:44px}.metadata-error{color:var(--el-color-danger);overflow-wrap:anywhere}
.metadata-unmatched{padding:12px;border:1px solid var(--mt-border-color);border-radius:var(--radius-sm);margin-block:12px}.metadata-unmatched-items{max-height:240px;overflow:auto;display:flex;flex-wrap:wrap;gap:8px}.metadata-unmatched .el-button{margin:0;max-width:100%;white-space:normal;height:auto;min-height:44px}
@media(max-width:480px){.metadata-row{grid-template-columns:minmax(0,1fr);gap:8px}.metadata-label{flex-direction:row;justify-content:space-between;align-items:center;min-height:0}.metadata-row .metadata-restore{justify-self:start;max-width:100%}}
</style>
