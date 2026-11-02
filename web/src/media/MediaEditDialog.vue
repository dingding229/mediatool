<script setup lang="ts">
import {joinMetadata} from '../presentation/metadata';

import {computed,nextTick,ref,watch} from 'vue';
import {ElMessageBox} from 'element-plus';
import 'element-plus/es/components/message-box/style/css';
import MetadataEditor from './MetadataEditor.vue';
import ImageEditor from './ImageEditor.vue';
import PosterArtwork from './PosterArtwork.vue';
import {image,mediaType,type MediaItem} from './model';
const props=defineProps<{item:MediaItem}>();
const visible=defineModel<boolean>('visible',{default:false});
const emit=defineEmits<{saved:[]}>();
const tab=ref<'metadata'|'images'>('metadata'),session=ref(0),saving=ref(false),error=ref('');
const metadata=ref<InstanceType<typeof MetadataEditor>>(),images=ref<InstanceType<typeof ImageEditor>>();
const dirty=computed(()=>!!(metadata.value?.dirty||images.value?.dirty));
const busy=computed(()=>saving.value||!!metadata.value?.busy||!!images.value?.busy);
const blocked=computed(()=>!!(metadata.value?.blocked||images.value?.blocked));
watch(visible,async open=>{if(!open)return;session.value++;tab.value='metadata';error.value='';await nextTick();});
watch([visible,metadata,images,session],()=>{if(visible.value&&metadata.value&&images.value){metadata.value.open();void images.value.open()}},{flush:'post'});
async function canClose(){if(busy.value)return false;if(!dirty.value)return true;try{await ElMessageBox.confirm('有未保存的修改，关闭后将放弃本次草稿。','放弃未保存修改？',{confirmButtonText:'放弃修改',cancelButtonText:'继续编辑',type:'warning'});return true}catch{return false}}
async function requestClose(done?:()=>void){if(!await canClose())return;metadata.value?.discard();images.value?.discard();if(done)done();else visible.value=false;}
function discard(){if(busy.value)return;metadata.value?.discard();images.value?.discard();error.value='';}
async function save(){
 if(busy.value||!dirty.value||blocked.value)return;saving.value=true;error.value='';let informationSaved=false;
 try{
  if(metadata.value?.dirty){if(!await metadata.value.save()){error.value='信息未保存，图片尚未提交；草稿已保留，请核对错误后再保存。';return}informationSaved=true;}
  if(images.value?.dirty&&!await images.value.saveDraft()){error.value=(informationSaved?'信息已保存；':'')+'图片未全部保存，未完成的图片草稿已保留，请重新读取并核对。';if(informationSaved||images.value.hasSaved)emit('saved');return;}
  emit('saved');visible.value=false;
 }finally{saving.value=false;}
}
defineExpose({dirty,busy,save,discard,requestClose});
</script>
<template>
 <el-dialog v-model="visible" title="编辑作品" class="media-edit-dialog" width="min(1120px,calc(100vw - 32px))" align-center :close-on-click-modal="false" :before-close="requestClose" :close-on-press-escape="!busy" :show-close="!busy">
  <div class="media-edit-layout">
   <aside class="media-edit-summary"><PosterArtwork :src="image(item.poster)" :title="item.title"/><div><h2>{{item.title}}</h2><p>{{joinMetadata([(item.year||'年份未标注'),(mediaType(item.mediaType||item.type))])}}</p></div></aside>
   <div class="media-edit-workspace">
    <nav class="media-edit-tabs mt-segments" aria-label="编辑内容"><button type="button" :aria-pressed="tab==='metadata'" :disabled="busy" @click="tab='metadata'">信息</button><button type="button" :aria-pressed="tab==='images'" :disabled="busy" @click="tab='images'">图片</button></nav>
    <div class="media-edit-scroll"><MetadataEditor :key="'metadata'+session" v-show="tab==='metadata'" ref="metadata" :item-id="item.id" hide-trigger external-controls/><ImageEditor :key="'images'+session" v-show="tab==='images'" ref="images" :item-id="item.id" hide-trigger external-controls deferred/></div>
    <p v-if="error" role="alert" class="media-edit-error">{{error}}</p>
   </div>
  </div>
  <template #footer><div class="media-edit-save"><el-button :disabled="busy" @click="requestClose()">取消</el-button><el-button :disabled="busy||!dirty" @click="discard">撤回草稿</el-button><el-button type="primary" :loading="saving" :disabled="busy||!dirty||blocked" @click="save">保存</el-button></div></template>
 </el-dialog>
</template>
<style>
.el-dialog.media-edit-dialog{max-height:85dvh;display:flex;flex-direction:column;overflow:hidden}.media-edit-dialog>.el-dialog__header,.media-edit-dialog>.el-dialog__footer{flex:none}.media-edit-dialog>.el-dialog__body{flex:1;min-height:0;display:flex;overflow:hidden}.media-edit-layout{display:grid;grid-template-columns:156px minmax(0,1fr);gap:24px;min-height:0;width:100%}.media-edit-summary{min-width:0;padding-right:16px;border-right:1px solid var(--mt-border-color)}.media-edit-summary .poster-artwork{width:96px}.media-edit-summary h2{font-size:16px;overflow-wrap:anywhere;line-height:1.5}.media-edit-summary p{font-size:12px;color:var(--mt-text-muted)}.media-edit-workspace{display:flex;flex-direction:column;min-width:0;min-height:0}.media-edit-tabs{flex:none;padding-bottom:12px;border-bottom:1px solid var(--mt-border-color)}.media-edit-tabs button{min-height:44px}.media-edit-scroll{min-height:0;overflow:auto;flex:1;padding-right:8px}.media-edit-scroll .metadata-editor,.media-edit-scroll .image-editor{margin-top:12px}.media-edit-save{position:sticky;bottom:0;z-index:2;display:flex;justify-content:flex-end;gap:8px;border-top:1px solid var(--mt-border-color);padding-top:12px;width:100%;background:var(--mt-bg-surface)}.media-edit-save .el-button{min-height:44px;margin:0}.media-edit-error{color:var(--mt-color-danger);font-size:13px;overflow-wrap:anywhere}.media-edit-dialog .el-dialog__headerbtn{min-width:44px;min-height:44px}
@media(max-width:600px){.el-dialog.media-edit-dialog{width:calc(100vw - 16px);max-width:calc(100vw - 16px);height:calc(100dvh - 16px);max-height:calc(100dvh - 16px);margin:8px auto}.media-edit-layout{display:flex;flex-direction:column;gap:12px}.media-edit-summary{display:flex;gap:12px;align-items:center;border:0;padding:0;flex:none}.media-edit-summary .poster-artwork{width:40px;flex:none}.media-edit-summary h2,.media-edit-summary p{margin:0}.media-edit-workspace{flex:1}.media-edit-tabs{position:sticky;top:0;background:var(--mt-bg-surface);z-index:1}.media-edit-save>.el-button{flex:1;min-width:0;padding-inline:8px}.media-edit-scroll{padding-right:0}}
</style>
