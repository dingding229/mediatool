<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { ref } from 'vue';
import StateView from '../ui/StateView.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { useManual, type ManualTask } from './useManual';
import { label, progress, size } from './model';
import { date, errorText, stateFor } from '../workflows/readiness';
const { tasks, dialog, kind, url, file, busy, loading, error, listError, intent, confirmOpen, target, actionBusy, observed, load, create, submit, reconcile, retry, approveRetry, associate } = useManual();
const detail = ref<ManualTask | null>(null), detailsOpen = ref(false);
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
        <p>仅添加到下载器；不自动识别、上传、入库或删除原文件。</p>
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
        <p>{{ task.message||'—' }}</p>
      </div>
      <div class="acquire-actions">
        <el-button @click="detail=task;detailsOpen=true">查看详情</el-button>
        <el-button :loading="actionBusy===task.id" @click="reconcile(task)">核对下载器</el-button>
        <el-button v-if="['blocked','failed','absent_confirmed'].includes(task.state)&&(!task.submitAttempted||task.state==='absent_confirmed')" @click="retry(task)">确认后重试</el-button>
      </div>
    </article>
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
  <ConfirmAction v-model:visible="confirmOpen" title="重试手动下载" :description="target?.state==='absent_confirmed'?'已核对未找到，但此前提交可能延迟接收；请确认接受重复风险。':'修复原因后，明确重新提交此任务。'" impact-scope="只重新投递此种子，不上传、不入库，也不删除本地文件。" confirm-text="确认重试" :loading="!!actionBusy" class="acquire-dialog" @confirm="approveRetry"/>
  <DetailDrawer v-model:visible="detailsOpen" title="手动下载详情" :technical-data="detail" class="acquire-detail">
    <h3>{{ detail?.title }}</h3>
    <p>{{ label(detail?.state) }} · {{ progress(detail?.progress) }}</p>
    <p>保存路径：{{ detail?.savePath||'—' }}</p>
    <p>{{ detail?.message||'—' }}</p>
  </DetailDrawer>
</template>
