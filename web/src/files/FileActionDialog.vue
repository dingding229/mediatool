<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue';
import { APIError, post, request } from '../api/client';
import PreviewApprove from '../ui/PreviewApprove.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { readPrivate, savePrivate, label, rows } from '../organize/model';
import { date, errorText } from '../workflows/readiness';
import { stepState } from './workbench';
import FileIdentityFields from './FileIdentityFields.vue';
import {createFileIdentity,filePayload,validateFileIdentity} from './identity';
const identity=ref<any>(null);
const emit = defineEmits<{
  changed: [
  ];
}>(), visible = ref(false), root = ref<any>(null), file = ref<any>(null), form = ref<any>({}), plan = ref<any>(null), result = ref<any>(null), busy = ref(false), error = ref<unknown>(null), phase = ref<'queued' | 'reconciling' | null>(null), confirmOpen = ref(false), historyOnly = ref(false), verified = ref(false);
const key = 'media-tool:file-operation:v1', titles: Record<string, string> = { mkdir: '新建文件夹', rename: '重命名', move: '移动文件 / 目录', trash: '删除（可恢复）', organize: '影视分类整理' };
let version = 0, disposed = false;
watch(visible, v => {
  if (!v) {
    version++; identity.value?.dispose();
  }
});
async function open(operation: string, chosenRoot: any, directory: string, row: any = null) {
  if (busy.value)
    return;
  const pending = readPrivate(key);
  visible.value = true;
  error.value = null;
  verified.value = false;
  historyOnly.value = false;
  result.value = null;
  if (pending?.id) {
    historyOnly.value = true;
    plan.value = pending;
    phase.value = 'reconciling';
    await readback();
    return;
  }
  const current = ++version;
  root.value = { ...chosenRoot };
  file.value = row ? { ...row } : null;
  plan.value = null;
  phase.value = null;
  form.value = { operation, source: row?.path || '', destination: operation === 'organize' ? '.' : directory, name: operation === 'mkdir' ? '' : row?.name || '', template: '{title} ({year}){ext}', nameMode: row?.directory ? 'direct' : 'default', title: row?.name.replace(/\.[^.]+$/, '') || '', year: '', mediaType: 'unknown', category: '', season: undefined, episode: undefined, resolution: '', includeSidecars: true };
  identity.value?.dispose(); identity.value=null;
  if(row&&operation==='organize'){
    identity.value=createFileIdentity(form.value,row.name);
    busy.value=true;
    try { await identity.value.recognize(); } finally {busy.value=false;}
  }
  if (row && !row.directory && operation==='rename') {
    busy.value = true;
    try {
      const out: any = await post('/media/recognize-name', { name: row.name });
      if (disposed || current !== version)
        return;
      for (const key of ['title', 'year', 'mediaType', 'season', 'episode', 'resolution', 'episodes'])
        if (out.hint[key] !== undefined)
          form.value[key] = out.hint[key];
      if (out.hint.warnings?.length)
        error.value = Error(out.hint.warnings.join('；'));
    }
    catch (e) {
      if (current === version)
        error.value = e;
    }
    finally {
      busy.value = false;
    }
  }
}
async function preview() {
  if (busy.value || phase.value || identity.value?.loading)
    return;
  busy.value = true;
  error.value = null;
  const current = ++version;
  try {
    const {nameMode,episodes}=form.value;
    const input=filePayload(form.value);
    if(input.operation==='organize'){const message=validateFileIdentity(form.value,!!file.value?.directory);if(message)throw Error(message);}
    if (episodes?.length > 1 && nameMode !== 'direct' && ['rename', 'organize'].includes(input.operation))
      throw Error('多集文件请使用完整自定义名称，不套用单集模板。');
    const out: any = await post('/files/preview', { ...input, connectionId: root.value.id, expectedFileId: file.value?.id, expectedSize: file.value?.size, expectedModified: Number.isSafeInteger(file.value?.modified) ? file.value.modified : undefined, name: ['move', 'trash'].includes(input.operation) ? '' : input.operation === 'mkdir' || nameMode === 'direct' ? input.name : '', template: ['move', 'trash'].includes(input.operation) ? '' : nameMode === 'template' ? input.template : '' });
    if (disposed || current !== version)
      return;
    plan.value = out;
    result.value = null;
    verified.value = false;
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function execute() {
  if (!plan.value || busy.value || phase.value || historyOnly.value)
    return;
  busy.value = true;
  phase.value = 'reconciling';
  error.value = null;
  savePrivate(key, plan.value);
  try {
    result.value = await post(`/files/operations/${plan.value.id}/execute`, { digest: plan.value.digest, confirm: true });
    phase.value = 'queued';
    confirmOpen.value = false;
    emit('changed');
  }
  catch (e) {
    error.value = e;
    confirmOpen.value = false;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      await readback(true);
      error.value = Error(e.status === 409 ? '文件或配置已变化，已重新读取。请核对记录并重新预览，不重复执行旧计划。' : errorText(e));
    }
  }
  finally {
    busy.value = false;
  }
}
async function readback(fromRejected = false) {
  if (!plan.value || busy.value && !fromRejected)
    return;
  const current = ++version;
  busy.value = true;
  try {
    const all = await request<any[]>('/files/operations');
    if (disposed || current !== version)
      return;
    const found = rows(all).find(r => r.id === plan.value.id);
    if (!found)
      throw Error('尚未取得此操作的持久记录，请继续核对，不重复提交。');
    result.value = found;
    verified.value = true;
    if (['succeeded', 'failed', 'expired'].includes(found.state) || fromRejected && found.state === 'preview') {
      phase.value = null;
      if (readPrivate(key)?.id === plan.value?.id)
        savePrivate(key, null);
    }
    emit('changed');
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function reconcile() {
  if (!plan.value || busy.value)
    return;
  busy.value = true;
  try {
    result.value = await post(`/files/operations/${plan.value.id}/reconcile`, {});
    verified.value = true;
    if (['succeeded', 'failed', 'expired'].includes(result.value.state)) {
      phase.value = null;
      if (readPrivate(key)?.id === plan.value?.id)
        savePrivate(key, null);
    }
    emit('changed');
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
function inspect(row: any) {
  if (busy.value)
    return;
  visible.value = true;
  historyOnly.value = true;
  verified.value = false;
  error.value = null;
  plan.value = row.plan;
  result.value = row;
  phase.value = readPrivate(key)?.id === row.id ? 'reconciling' : null;
  void readback();
}
onUnmounted(() => {
  disposed = true; identity.value?.dispose();
  version++;
});
defineExpose({ open, inspect });
</script>
<template>
  <el-dialog v-model="visible" :title="titles[plan?.operation||form.operation]||'文件操作记录'" width="min(780px,94vw)" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" class="media-dialog files-dialog">
    <PreviewApprove :current-step="!plan?1:verified&&['succeeded','failed','expired'].includes(result?.state)?4:phase?3:2">
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <div v-if="!plan" class="media-form">
        <p>{{ root?.name }} · {{ file?.path||form.destination }}</p>
        <label v-if="form.operation==='mkdir'">
          文件夹名称
          <el-input v-model="form.name" aria-label="新文件夹名称" :disabled="busy"/>
        </label>
        <label v-if="['move','organize'].includes(form.operation)">
          目标相对目录（. 使用分类目录）
          <el-input v-model="form.destination" aria-label="目标相对目录" :disabled="busy"/>
        </label>
        <template v-if="['rename','organize'].includes(form.operation)">
          <label>
            命名方式
            <el-select v-model="form.nameMode" aria-label="命名方式" :disabled="busy">
              <el-option value="default" label="系统模板"/>
              <el-option value="direct" label="直接输入完整名称"/>
              <el-option value="template" label="自定义模板"/>
            </el-select>
          </label>
          <label v-if="form.nameMode==='direct'">
            完整名称
            <el-input v-model="form.name" aria-label="完整名称" :disabled="busy"/>
          </label>
          <label v-if="form.nameMode==='template'">
            命名模板
            <el-input v-model="form.template" aria-label="命名模板" :disabled="busy"/>
          </label>
          <FileIdentityFields v-if="form.operation==='organize'&&identity" :identity="identity" :draft="form" :disabled="busy"/>
          <template v-else>
          <label>
            标题
            <el-input v-model="form.title" aria-label="媒体标题" :disabled="busy"/>
          </label>
          <label>
            年份
            <el-input v-model="form.year" aria-label="媒体年份" :disabled="busy"/>
          </label>
          <label>
            媒体类型
            <el-select v-model="form.mediaType" aria-label="媒体类型" :disabled="busy">
              <el-option value="unknown" label="未识别"/>
              <el-option value="movie" label="电影"/>
              <el-option value="series" label="剧集"/>
            </el-select>
          </label>
          <label v-if="form.mediaType==='series'">
            季号
            <el-input-number v-model="form.season" aria-label="季号" :min="0" :max="9999" :disabled="busy"/>
          </label>
          <label v-if="form.mediaType==='series'">
            集号
            <el-input-number v-model="form.episode" aria-label="集号" :min="1" :max="9999" :disabled="busy"/>
          </label>
          </template>
          <label v-if="form.operation!=='organize'">
            分辨率
            <el-input v-model="form.resolution" aria-label="分辨率" :disabled="busy"/>
          </label>
        </template>
        <el-checkbox v-model="form.includeSidecars" :disabled="busy">同时处理同名侧车文件</el-checkbox>
        <p v-if="form.operation==='trash'">删除会移入此目录内的回收区，不是永久删除；下载做种源和托管输出不允许直接修改。</p>
        <el-button type="primary" :loading="busy" :disabled="identity?.loading" @click="preview">预览变更</el-button>
      </div>
      <template v-else>
        <p>预览有效至 {{ date(plan.expiresAt) }} · {{ result&&verified?label(result.state):phase==='queued'?'已排队':phase?'正在核对':'等待核对' }}</p>
        <article v-for="(item,index) in plan.items" :key="index" class="organize-record files-plan-item">
          <strong>{{ item.source?.name||'文件变更' }}</strong>
          <p>最终路径：{{ plan.root }}/{{ item.target }}</p>
          <p>目录分类：{{plan.category||'—'}} · 命名结果：{{item.target.split('/').pop()}}</p>
          <p>原路径：{{item.source?.path||'—'}}</p>
        </article>
        <details v-if="result&&verified" class="files-technical">
          <summary>执行步骤与核验</summary>
          <article v-for="(step,index) in plan.steps" :key="index">
            <strong>{{ step.action==='mkdir'?'创建目录':'移动或改名' }} · {{ label(stepState(result.state,result.completedSteps||0,Number(index))) }}</strong>
            <p>{{ step.source?.path||'—' }} → {{ step.target }}</p>
          </article>
        </details>
        <p v-if="result&&verified">{{ result.message||'—' }}</p>
        <AsyncResultBanner v-if="phase" :status="phase" :message="phase==='queued'?'文件操作已排队，尚未核验完成。':'正在核对上次结果，未取得结果前不重复执行。'" @refresh="readback()"/>
        <div class="media-actions">
          <el-button v-if="!phase&&!historyOnly&&!result&&Date.parse(plan.expiresAt)>Date.now()" type="primary" @click="confirmOpen=true">核对后执行</el-button>

          <el-button :disabled="busy" @click="reconcile">核对结果（不重放）</el-button>
          <el-button v-if="!phase&&!historyOnly&&verified&&result?.state!=='succeeded'" @click="plan=null;result=null;verified=false">重新预览</el-button>
        </div>
        <details class="files-technical">
          <summary>技术详情</summary>
          <pre>{{ JSON.stringify({plan,result},null,2) }}</pre>
        </details>
      </template>
    </PreviewApprove>
    <template #footer>
      <el-button :disabled="busy" @click="visible=false">关闭</el-button>
    </template>
  </el-dialog>
  <ConfirmAction v-model:visible="confirmOpen" :title="plan?.operation==='trash'?'确认移入回收目录':'确认文件变更'" description="只执行刚核对的原位置与目标，文件或配置变化时拒绝旧计划。" :impact-scope="plan?.operation==='trash'?'文件及选中的侧车将移入回收目录，可由文件浏览核对后移回；不永久删除媒体。':'可能改名或移动文件及同名侧车；不修改受保护的下载做种源，不变更固定播放凭据。'" :confirm-text="plan?.operation==='trash'?'确认移入回收目录':'确认执行变更'" :danger-level="plan?.operation==='trash'?'danger':'warning'" :loading="busy" class="media-dialog" @confirm="execute"/>
</template>

<style scoped>
.files-dialog .media-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.files-dialog .media-form>p,.files-dialog .media-form>.el-alert,.files-dialog .media-form>.f6-identity,.files-dialog .media-form>.el-checkbox,.files-dialog .media-form>.el-button{grid-column:1/-1}.files-dialog .media-form>label{min-width:0}.files-dialog .media-form>.el-checkbox{display:flex;align-items:center;gap:8px}@media(max-width:560px){.files-dialog .media-form{grid-template-columns:1fr}}
</style>
