<script setup lang="ts">
import {joinMetadata} from '../presentation/metadata';

import {ref} from 'vue';
import {ElDropdown,ElDropdownMenu,ElDropdownItem} from 'element-plus';
import 'element-plus/es/components/dropdown/style/css';
import 'element-plus/es/components/dropdown-menu/style/css';
import 'element-plus/es/components/dropdown-item/style/css';
import {subtitleRowTitle,subtitleExtraTitle} from './subtitle-presentation';
import FilePicker from '../ui/FilePicker.vue';
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
import {subtitleDownload} from '../api/subtitles';
import {useSubtitles} from './useSubtitles';
import {subtitleLanguages,subtitleLanguageLabel,languageChoice} from './subtitle-language';
import {errorText} from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
const menus=ref<InstanceType<typeof ElDropdown>[]>([]);
function closeSubtitleMenus(){for(const menu of menus.value)menu.handleClose()}
const props=defineProps<{itemId:string}>();
const {asset,data,op,file,encoding,choice,detected,detecting,detectionError,notice,uploadOpen,editing,form,editLanguage,remove,confirm,rows,shownGroups,load,selectAsset,choose,upload,edit,save,applyDelete,cancelPending}=useSubtitles(()=>props.itemId);
const origin=(s:string)=>({automatic:'外挂',embedded:'内嵌',uploaded:'上传'}[s]||'待核对');
const versionName=(id:string)=>data.data.versions.find(v=>v.id===id)?.name||'物理版本';
const versionLabel=(v:{name:string;season?:number;episode?:number})=>(v.episode!=null?`第 ${v.season} 季 · 第 ${v.episode} 集 · `:'')+v.name;
</script>
<template>
 <section class="subtitle-panel catalog-detail-section" aria-label="字幕">
  <header class="subtitle-heading"><h3>字幕</h3><el-button :disabled="!asset||!!op.pending||!!data.error" @click="uploadOpen=true">上传字幕</el-button></header>
  <label v-if="data.data.versions.length>1" class="subtitle-field">选择版本或单集<el-select :model-value="asset" aria-label="字幕物理版本" filterable placeholder="先选择版本或单集" :disabled="!!op.pending" @change="selectAsset"><el-option v-for="version in data.data.versions" :key="version.id" :value="version.id" :label="versionLabel(version)"/></el-select></label>
  <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" @reload="load"/>
  <AsyncResultBanner v-if="op.phase" :status="op.phase" message="字幕操作结果待核对，不会自动重复提交。" @refresh="load"/>
  <el-button v-if="op.pending&&data.loaded&&!data.loading&&!data.error" @click="cancelPending">已核对，取消待定操作</el-button>
  <p v-if="op.error" role="alert">{{errorText(op.error)}}</p><p v-if="notice" role="status">{{notice}}</p>
  <p v-if="data.loaded&&!data.error&&!shownGroups.length" class="subtitle-empty">{{asset?'暂无字幕，可上传字幕文件。':'请先选择单集，查看或上传字幕。'}}</p>
  <p v-if="rows.some(row=>row.source==='embedded')" class="subtitle-embedded-note">内嵌字幕随视频播放，仅编辑显示信息，不提供提取或删除。</p>
  <section v-for="group in shownGroups" :key="group.assetId" class="subtitle-version" :aria-label="versionName(group.assetId)">
   <article v-for="row in group.items.filter(row=>!row.hidden)" :key="row.id" class="subtitle-row">
    <div class="subtitle-row-main"><strong><MiddleEllipsis :text="subtitleRowTitle(row)" :copy="false"/></strong><p v-if="row.source!=='embedded'||row.default||row.forced||row.deleting">{{joinMetadata([subtitleExtraTitle(row),...(row.source!=='embedded'?[subtitleLanguageLabel(row),row.extension.toUpperCase(),origin(row.source)]:[]),row.default?'默认':null,row.forced?'强制':null,row.deleting?'副本清理待重试':null])}}</p></div>
    <div v-if="editing?.id!==row.id" class="subtitle-actions"><el-button :disabled="!!op.pending||row.deleting" @click="edit(row)">编辑</el-button><ElDropdown v-if="row.source!=='embedded'" ref="menus" @keydown.esc.stop="closeSubtitleMenus" trigger="click" popper-class="media-dropdown"><el-button :disabled="!!op.pending">更多</el-button><template #dropdown><ElDropdownMenu @keydown.esc.stop.prevent="closeSubtitleMenus"><ElDropdownItem v-if="!row.deleting"><a :href="subtitleDownload(row.assetId,row.id)" target="_blank" rel="noopener noreferrer">下载字幕</a></ElDropdownItem><ElDropdownItem :disabled="!!op.pending" @click="remove=row;confirm=true">{{row.deleting?'重试清理':row.source==='uploaded'?'删除上传副本':'移除外挂字幕'}}</ElDropdownItem></ElDropdownMenu></template></ElDropdown></div>
    <form v-else class="subtitle-editor" @submit.prevent="save()"><p v-if="languageChoice(editing)==='chi'">简繁未识别，可手动选择</p><label class="subtitle-field">语言<el-select v-model="editLanguage" aria-label="字幕语言"><el-option v-for="language in subtitleLanguages.filter(v=>v.value!=='auto')" :key="language.value" :value="language.value" :label="language.label"/><el-option v-if="!subtitleLanguages.some(v=>v.value===editLanguage)" :value="editLanguage" :label="subtitleLanguageLabel(form)"/></el-select></label><label class="subtitle-field">显示名称<el-input v-model="form.title" aria-label="字幕标题" maxlength="200"/></label><div class="subtitle-checks"><el-checkbox v-model="form.default">默认字幕</el-checkbox><el-checkbox v-model="form.forced">强制字幕</el-checkbox></div><div class="subtitle-actions"><el-button :disabled="!!op.pending" @click="editing=undefined">取消编辑</el-button><el-button native-type="submit" :disabled="!!op.pending" :loading="op.busy">保存字幕信息</el-button></div></form>
   </article>
  </section>
  <details v-if="rows.some(row=>row.hidden)" class="subtitle-hidden"><summary>已隐藏的外挂字幕</summary><div v-for="row in rows.filter(row=>row.hidden)" :key="row.id"><MiddleEllipsis :text="row.filename" :copy="false"/><el-button :disabled="!!op.pending" @click="save(row,{...row,hidden:false})">恢复显示</el-button></div></details>
  <el-dialog v-model="uploadOpen" title="上传字幕" width="min(680px,calc(100vw - 32px))" class="subtitle-upload-dialog" append-to-body :close-on-click-modal="false" :show-close="!op.busy" :close-on-press-escape="!op.busy">
   <div class="subtitle-upload"><p>上传到：{{versionName(asset)}}</p><FilePicker accept=".srt,.ass,.ssa,.vtt" label="选择字幕文件" :disabled="!!op.pending||op.busy" @select="choose"/><MiddleEllipsis v-if="file" :text="file.name" :copy="false"/><p>支持 srt / ass / ssa / vtt，最大 5MiB；不修改媒体或 STRM。</p><p v-if="detecting" role="status">正在预识别字幕语言…</p><p v-else-if="detected" role="status" class="subtitle-detection">识别结果：{{detected.label}}（{{detected.evidence}}）<span v-if="!detected.known">，请手动选择语言。</span></p><p v-if="detectionError" role="alert">{{detectionError}}</p>
    <label class="subtitle-field">语言<el-select v-model="choice" aria-label="上传字幕语言" :disabled="!!op.pending||op.busy"><el-option v-for="language in subtitleLanguages" :key="language.value" :value="language.value" :label="language.label"/></el-select></label>
    <details><summary>编码有误时调整</summary><label class="subtitle-field">原编码<el-select v-model="encoding" aria-label="字幕编码" :disabled="!!op.pending||op.busy"><el-option value="auto" label="自动检测"/><el-option v-for="value in ['utf-8','utf-16le','utf-16be','gb18030','big5','shift-jis','windows-1252']" :key="value" :value="value" :label="value.toUpperCase()"/></el-select></label></details>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" message="字幕操作结果待核对，不会自动重复提交。" @refresh="load"/><p v-if="op.error" role="alert">{{errorText(op.error)}}</p>
   </div>
   <template #footer><el-button :disabled="op.busy" @click="uploadOpen=false">取消</el-button><el-button type="primary" :loading="op.busy" :disabled="!file||!asset||!detected||detecting||!!op.pending||!!detectionError" @click="upload">确认上传</el-button></template>
  </el-dialog>
  <ConfirmAction v-model:visible="confirm" :title="remove?.source==='automatic'?'移除外挂字幕':'删除上传字幕副本'" :description="`确认${remove?.source==='automatic'?'从列表移除':'删除'} ${remove?.filename||'此字幕'}？`" :impact-scope="remove?.source==='automatic'?'仅从本程序和播放列表隐藏，不删除云盘原文件；可恢复显示。':'仅删除本程序登记和上传副本；不会删除或修改网盘文件、媒体及 STRM。'" :confirm-text="remove?.source==='automatic'?'确认移除':'删除副本'" :loading="op.busy" @confirm="applyDelete"/>
 </section>
</template>
<style scoped>
.subtitle-panel{min-width:0}.subtitle-heading,.subtitle-actions,.subtitle-checks{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.subtitle-heading{justify-content:space-between}.subtitle-heading h3{margin:0}.subtitle-field{display:grid;gap:6px;min-width:0;font-size:13px;font-weight:600}.subtitle-panel>.subtitle-field{margin-block:12px}.subtitle-panel p,.subtitle-upload p{font-size:12px;color:var(--mt-text-muted);line-height:1.6;overflow-wrap:anywhere}.subtitle-empty{margin-block:12px}.subtitle-version h4{font-size:13px;margin:16px 0 0}.subtitle-row{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding-block:12px;border-top:1px solid var(--mt-border-color)}.subtitle-row-main{flex:1;min-width:180px}.subtitle-row p{margin:4px 0 0}.subtitle-actions .el-button{margin:0;min-height:44px}.subtitle-editor{display:grid;gap:12px;width:100%}.subtitle-upload{display:grid;gap:12px;min-width:0}.subtitle-hidden summary,.subtitle-upload summary{cursor:pointer;min-height:44px;line-height:44px}.subtitle-hidden>div{display:flex;gap:8px;align-items:center}.subtitle-hidden .middle-ellipsis{flex:1}
@media(max-width:480px){.subtitle-actions{width:100%}.subtitle-actions>.el-button{flex:1}.subtitle-row-main{min-width:0;width:100%;flex-basis:100%}}
</style>
<style>.subtitle-upload-dialog .el-button{min-height:44px;margin-left:0}.subtitle-upload-dialog .el-select__wrapper{min-height:44px}</style>
