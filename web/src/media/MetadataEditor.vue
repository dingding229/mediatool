<script setup lang="ts">
import {computed,reactive,ref,watch} from 'vue';
import {APIError} from '../api/client';
import PagePagination from '../ui/PagePagination.vue';
import {metadataFields,readMediaMetadata,saveMediaMetadata,metadataTargetID,type MetadataField,type MetadataRecord,type MetadataTarget} from '../api/mediaMetadata';
import {inputValue,metadataLabels,metadataPatch} from './metadata-form';
import {errorText} from '../workflows/readiness';
const props=defineProps<{itemId:string}>(),emit=defineEmits<{saved:[]}>();
const opened=ref(false),busy=ref(false),record=ref<MetadataRecord>(),draft=reactive(Object.fromEntries(metadataFields.map(k=>[k,''])) as Record<MetadataField,string>),restore=ref(new Set<MetadataField>()),error=ref(''),notice=ref(''),blocked=ref(false),activeID=ref(props.itemId),choices=ref<MetadataTarget[]>([]);
let epoch=0;
const patch=computed(()=>record.value?metadataPatch(record.value,draft,restore.value):undefined);
const dirty=computed(()=>!!patch.value&&(Object.keys(patch.value.set).length>0||patch.value.restore.length>0));
const label=(key:MetadataField)=>key==='title'&&record.value?.target.kind==='episode'?'集标题':metadataLabels[key];
const targetLabel=(t:MetadataTarget)=>t.kind==='episode'?`第 ${t.season} 季 · 第 ${t.episode} 集`:t.kind==='season'?`第 ${t.season} 季`:t.kind==='series'?'剧集':'电影';
function assign(r:MetadataRecord){for(const key of metadataFields)draft[key]=inputValue(r.effective[key],key);restore.value=new Set();}
async function load(keep=false,page=1){
 const version=++epoch,pending=keep&&record.value?metadataPatch(record.value,draft,restore.value):undefined;
 busy.value=true;error.value='';
 try{const out=await readMediaMetadata(activeID.value,page);if(version!==epoch)return;
  record.value=out;assign(out);
  if(pending){for(const key of Object.keys(pending.set) as MetadataField[])draft[key]=inputValue(pending.set[key],key);restore.value=new Set(pending.restore);for(const key of pending.restore)draft[key]=inputValue(out.base[key],key);}
  if(activeID.value===props.itemId)choices.value=out.choices||[out.target];blocked.value=false;if(keep)notice.value='已读取当前版本并保留修改草稿，未改字段使用当前值，请比较后再保存。';
 }catch(e){if(version===epoch)error.value=errorText(e);}finally{if(version===epoch)busy.value=false;}
}
function open(){opened.value=true;activeID.value=props.itemId;notice.value='';blocked.value=false;void load();}
function stageRestore(key:MetadataField){restore.value.add(key);draft[key]=inputValue(record.value?.base[key],key);notice.value='恢复操作尚未保存。';}
function changed(key:MetadataField){restore.value.delete(key);}
async function choose(id:string){activeID.value=id;notice.value='';await load();}
async function save(){if(!patch.value||!dirty.value||blocked.value)return;const version=epoch;busy.value=true;error.value='';try{const out=await saveMediaMetadata(activeID.value,patch.value);if(version!==epoch)return;record.value=out;assign(out);notice.value='信息已保存，修改字段已锁定；恢复字段已解除锁定。';emit('saved');}catch(e){if(version!==epoch)return;error.value=errorText(e);if(!(e instanceof APIError)||e.status>=500||e.status===409){blocked.value=true;error.value+=' 请重新读取并核对，不要重复提交。';}}finally{if(version===epoch)busy.value=false;}}
watch(()=>props.itemId,()=>{epoch++;opened.value=false;record.value=undefined;activeID.value=props.itemId;busy.value=false;blocked.value=false;});
</script>
<template>
<section class="metadata-editor" aria-label="元数据信息">
 <el-button v-if="!opened" @click="open">编辑信息</el-button>
 <template v-else>
  <div class="metadata-heading"><h3>编辑信息</h3><el-button :disabled="busy" @click="opened=false">收起编辑</el-button></div>
  <p class="metadata-note">修改过的字段将逐项锁定，不会被后续刮削覆盖。同一作品或单集的物理版本共用文字信息，不改文件与目录分类。</p>
  <label v-if="choices.length>1" class="metadata-field">选择单集<select :value="record?metadataTargetID(record.target):activeID" :disabled="busy||dirty" @change="choose(($event.target as HTMLSelectElement).value)"><option v-for="t in choices" :key="metadataTargetID(t)" :value="metadataTargetID(t)">{{targetLabel(t)}}</option></select></label>
  <p v-if="busy&&!record" role="status">正在读取元数据…</p>
  <p v-if="error" class="metadata-error" role="alert">{{error}}</p>
  <p v-if="notice" role="status">{{notice}}</p>
  <el-button v-if="blocked||error&&!record" :disabled="busy" @click="load(!!record)">重新读取并核对</el-button>
  <template v-if="record">
   <p v-if="!record.matched" role="status">未匹配：当前季/集编号已无入库绑定，已保留人工信息，可继续核对。</p>
   <details v-if="record.unmatched.length" class="metadata-unmatched"><summary>未匹配的人工信息（{{record.unmatchedTotal??record.unmatched.length}}）</summary><p>编号失效的记录不会删除，也不会套用到其他季/集。</p><el-button v-for="t in record.unmatched" :key="metadataTargetID(t)" :disabled="busy||dirty" @click="choose(metadataTargetID(t))">{{targetLabel(t)}} · 未匹配</el-button><PagePagination v-if="(record.unmatchedTotal??0)>50" :current-page="record.unmatchedPage" :page-size="50" :total="record.unmatchedTotal" :disabled="busy||dirty" label="未匹配信息分页" @current-change="n=>load(false,n)"/></details>
   <form class="metadata-form" @submit.prevent="save">
    <div v-for="key in metadataFields" :key="key" class="metadata-row">
     <label class="metadata-field" :for="`metadata-${key}`"><span>{{label(key)}} <span class="metadata-lock">{{restore.has(key)?'待恢复':Object.hasOwn(record.overrides,key)?'人工锁定':'刮削值'}}</span></span>
      <textarea v-if="key==='overview'" :id="`metadata-${key}`" v-model="draft[key]" :disabled="busy" rows="5" maxlength="10000" @input="changed(key)"/>
      <input v-else :id="`metadata-${key}`" v-model="draft[key]" :disabled="busy" :type="key==='premiereDate'?'date':['year','communityRating'].includes(key)?'number':'text'" :step="key==='communityRating'?'0.1':key==='year'?'1':undefined" :min="key==='year'?1800:key==='communityRating'?0:undefined" :max="key==='year'?2100:key==='communityRating'?10:undefined" :maxlength="key==='tagline'?1000:['genres','tags'].includes(key)?undefined:500" :placeholder="['genres','tags'].includes(key)?'用顿号或逗号分隔，最多50项':undefined" @input="changed(key)"/>
     </label>
     <div class="metadata-origin"><span :title="inputValue(record.base[key],key)">刮削值：{{inputValue(record.base[key],key)||'暂无刮削值'}}</span><el-button :disabled="busy||!Object.hasOwn(record.overrides,key)&&!restore.has(key)" @click="stageRestore(key)">恢复为刮削值</el-button></div>
    </div>
    <div class="mt-save-actions"><el-button class="mt-save-button" type="primary" native-type="submit" :loading="busy" :disabled="busy||blocked||!dirty">保存信息</el-button><el-button :disabled="busy" @click="assign(record);notice='已撤回未保存修改'">撤回草稿</el-button></div>
   </form>
  </template>
 </template>
</section>
</template>
<style scoped>
.metadata-editor{margin-block:16px;min-width:0}.metadata-heading,.metadata-origin{display:flex;align-items:center;gap:12px;justify-content:space-between;min-width:0}.metadata-heading h3{margin:0}.metadata-note,.metadata-origin{color:var(--mt-text-muted);font-size:13px}.metadata-form{display:grid;gap:16px}.metadata-row{display:grid;gap:8px;min-width:0}.metadata-field{display:grid;gap:8px;min-width:0}.metadata-field input,.metadata-field textarea,.metadata-field select{width:100%;min-width:0;box-sizing:border-box;font:inherit;min-height:44px;padding:10px 12px;border:1px solid var(--mt-border-color);border-radius:var(--radius-sm);background:var(--mt-bg-surface);color:var(--mt-text-primary)}.metadata-field textarea{resize:vertical}.metadata-field input:focus-visible,.metadata-field textarea:focus-visible,.metadata-field select:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:2px}.metadata-lock{font-size:12px;color:var(--mt-text-muted);margin-inline-start:8px}.metadata-origin>span{min-width:0;overflow:hidden;white-space:nowrap;text-overflow:ellipsis;flex:1}.metadata-origin .el-button{flex:none;margin:0;min-height:44px}.metadata-error{color:var(--el-color-danger);overflow-wrap:anywhere}.metadata-unmatched{padding:12px;border:1px solid var(--mt-border-color);border-radius:var(--radius-sm);margin-block:12px}.metadata-unmatched .el-button{margin:4px;max-width:100%;white-space:normal;height:auto;min-height:44px}
@media(max-width:480px){.metadata-origin{align-items:flex-start;flex-direction:column;gap:4px}.metadata-origin>span{max-width:100%}}
</style>
