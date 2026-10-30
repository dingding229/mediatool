<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import {reactive,ref,watch} from 'vue';
import {post} from '../api/client';
import {useOperation} from '../workflows/useOperation';
import MediaContext from './MediaContext.vue';
import type {Media} from './model';
import StateView from '../ui/StateView.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { useManual, type ManualTask } from './useManual';
import { label, progress, size } from './model';
import { date, errorText, stateFor } from '../workflows/readiness';
const {pageSize,page,total, tasks, dialog, kind, url, file, busy, loading, error, listError, intent, confirmOpen, target, actionBusy, observed, load, create, submit, reconcile, retry, approveRetry, associate } = useManual();
const detail = ref<ManualTask | null>(null), detailsOpen = ref(false), selectedMedia=ref<Media|null>(null), importConfirm=ref(false), importOp=reactive(useOperation('manual-import'));
watch(()=>detail.value?.id,()=>selectedMedia.value=null);
watch(tasks,rows=>{const fresh=rows.find(t=>t.id===detail.value?.id);if(fresh)detail.value=fresh; if(importOp.pending){const id=JSON.parse(importOp.pending.key).id,row=rows.find(t=>t.id===id);if(row?.managedTaskId||row?.processingState==='needs_confirmation')importOp.clear();}});
const importLabel=(s?:string)=>({download_only:'仅下载（未启用后处理）',waiting_download:'完成后自动识别入库',identifying:'识别入库排队 / 执行中',needs_confirmation:'待人工选择身份，原文件保留'}[s||'']||label(s));
async function readImport(){await load();if(importOp.pending){const id=JSON.parse(importOp.pending.key).id;const row=tasks.value.find(t=>t.id===id);if(row?.managedTaskId||row?.processingState==='needs_confirmation')importOp.clear();}}
async function importTask(){if(!detail.value||!selectedMedia.value)return;const id=detail.value.id,media=selectedMedia.value;await importOp.run(JSON.stringify({id}),()=>post(`/downloads/manual/${id}/import`,{confirm:true,mediaType:media.mediaType,tmdbId:media.id}));importConfirm.value=false;await readImport();}

function choose(event: Event) {
  file.value = (event.target as HTMLInputElement).files?.[0];
}
usePageRefresh(load,'manual');
</script>
<template>
  <section class="acquire-surface">
    <header class="acquire-section-heading">
      <div>
        <h2>手动种子下载</h2>
        <p>新建手动任务完成后自动识别、整理、上传 115 并生成 STRM；识别不确定时保留原文件。历史任务不自动回溯。</p>
      </div>
      <div class="acquire-actions">

        <RouterLink to="/downloads?tab=tasks">同步下载器任务</RouterLink>
        <el-button type="primary" @click="create">添加种子</el-button>
      </div>
    </header>
    <AsyncResultBanner v-if="intent" :status="intent.phase" :message="intent.phase==='queued'?'手动下载已排队或已关联现有任务，尚未确认下载器接收。':'提交回执未知，先核对手动下载与下载器，不重复添加。'" @refresh="load"/>
    <StateView v-if="loading" state="loading" title="正在读取手动下载…"/>
    <StateView v-else-if="listError" :state="stateFor(listError)" title="手动任务读取失败" :description="errorText(listError)" @retry="load"/>
    <StateView v-else-if="!tasks.length" state="empty" title="暂无手动下载任务"/>
    <article v-for="task in tasks" :key="task.id" class="acquire-task">
      <div>
        <h3>{{ task.title }}</h3>
        <p>{{ label(task.state) }} · {{ progress(task.progress) }} · {{ date(task.syncedAt) }}</p>
        <p v-if="task.progress==null">进度待下载器同步，不以0%代替未知进度。</p>
        <p v-else>下载 {{ task.syncedAt?size(task.downloadSpeed)+'/s':'—' }}</p>
        <p>后处理：{{importLabel(task.processingState)}}</p>
        <p>{{ task.message||'—' }}</p>
      </div>
      <div class="acquire-actions">
        <el-button @click="detail=task;detailsOpen=true">查看详情</el-button>
        <RouterLink v-if="task.managedTaskId" to="/downloads?tab=uploads">查看上传入库</RouterLink>
        <el-button v-else-if="task.state==='completed'" @click="detail=task;detailsOpen=true">选择身份后入库</el-button>
        <el-button :loading="actionBusy===task.id" @click="reconcile(task)">核对下载器</el-button>
        <el-button v-if="['blocked','failed','absent_confirmed'].includes(task.state)&&(!task.submitAttempted||task.state==='absent_confirmed')" @click="retry(task)">确认后重试</el-button>
      </div>
    </article>
    <PagePagination v-model:current-page="page" :total="total" v-model:page-size="pageSize" @current-change="load"/>
  </section>
  <el-dialog v-model="dialog" title="添加种子下载" width="min(650px,94vw)" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" class="acquire-dialog">
    <form class="acquire-form" @submit.prevent="submit">
      <p>文件不超过10MiB；相同种子身份由服务端去重，回执未知时先核对。</p>
      <el-radio-group v-model="kind" :disabled="busy||!!intent" aria-label="种子输入方式">
        <el-radio-button value="file">种子文件</el-radio-button>
        <el-radio-button value="link">种子链接</el-radio-button>
      </el-radio-group>
      <label v-if="kind==='file'">
        种子文件（.torrent）
        <input type="file" accept=".torrent" aria-label="种子文件（.torrent）" :disabled="busy||!!intent" @change="choose"/>
      </label>
      <label v-else>
        种子链接或磁力链接
        <el-input v-model="url" aria-label="种子链接或磁力链接" :disabled="busy||!!intent"/>
      </label>
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <template v-if="intent?.phase==='reconciling'">
        <p>先读取列表并在下载器确认。找到对应任务后，可以关联，不会重新投递。</p>
        <el-button @click="load">读取已有手动任务</el-button>
        <el-select v-model="observed" aria-label="已核对的现有手动任务">
          <el-option v-for="task in tasks" :key="task.id" :value="task.id" :label="task.title"/>
        </el-select>
        <el-button :disabled="!observed" @click="associate">关联此任务，不重新提交</el-button>
      </template>
      <el-button v-else type="primary" native-type="submit" :loading="busy" :disabled="!!intent">确认添加下载</el-button>
    </form>
  </el-dialog>
  <ConfirmAction v-model:visible="confirmOpen" title="重试手动下载" :description="target?.state==='absent_confirmed'?'已核对未找到，但此前提交可能延迟接收；请确认接受重复风险。':'修复原因后，明确重新提交此任务。'" impact-scope="只重试原种子的投递；已明确启用后处理的任务完成后接入原流水线。未完成上传校验与入库不会删除本地文件。" confirm-text="确认重试" :loading="!!actionBusy" class="acquire-dialog" @confirm="approveRetry"/>
  <DetailDrawer v-model:visible="detailsOpen" title="手动下载详情" size="large" class="acquire-detail">
    <h3>{{ detail?.title }}</h3>
    <p>{{ label(detail?.state) }} · {{ progress(detail?.progress) }}</p>
    <p>保存路径：{{ detail?.savePath||'—' }}</p>
    <p>{{ detail?.message||'—' }}</p>
    <template v-if="detail?.state==='completed'&&!detail?.managedTaskId">
      <p>明确选择此原任务的媒体身份后入库，不重新添加种子；季集不明仍保留待人工处理。</p>
      <MediaContext :key="detail.id" :keyword="detail.title" :initial="null" :disabled="!!importOp.pending" @selected="selectedMedia=$event"/>
      <el-button :disabled="!selectedMedia||!!importOp.pending" @click="importConfirm=true">确认所选身份后入库</el-button>
    </template>
    <AsyncResultBanner v-if="importOp.phase" :status="importOp.phase" message="原任务后处理已排队或回执待核对，尚未证明入库完成。" @refresh="readImport"/>
    <el-alert v-if="importOp.error" :title="errorText(importOp.error)" type="error" :closable="false"/>
  </DetailDrawer>
  <ConfirmAction v-model:visible="importConfirm" title="原任务后处理" description="按已选择的媒体身份整理并上传此任务？" impact-scope="不重新添加下载。上传校验及入库成功后，才按冻结设置与做种/H&amp;R保护决定是否清理本地源文件；未完成或不确定时保留。" confirm-text="确认身份并入库" :loading="importOp.busy" @confirm="importTask"/>
</template>
