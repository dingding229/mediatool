<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue';
import { APIError, post, request } from '../api/client';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText } from '../workflows/readiness';
import { readIntent, writeIntent, type Intent } from './model';
const emit = defineEmits<{
  completed: [
  ];
}>(), visible = ref(false), taskId = ref(''), searchRunId = ref(''), preview = ref<any>(null), busy = ref(false), applying = ref(false), error = ref<unknown>(null), confirmed = ref(false), initialConfirmed = ref(false), intent = ref<Intent | null>(null);
let generation = 0;
watch(visible, open => { if (!open) generation++; });
onUnmounted(() => { generation++; });
async function refresh() {
  if (applying.value || !taskId.value)
    return;
  const version = ++generation;
  busy.value = true;
  preview.value = null;
  confirmed.value = false;
  initialConfirmed.value = false;
  try {
    const suffix = searchRunId.value.trim() ? `?searchRunId=${encodeURIComponent(searchRunId.value.trim())}` : '';
    const result = await request(`/downloads/${taskId.value}/source-preview${suffix}`);
    if (version !== generation) return;
    preview.value = result;
    intent.value = readIntent(`source:${taskId.value}:${preview.value.controlRevision}:${preview.value.planDigest}`);
    error.value = null;
  }
  catch (e) {
    if (version === generation) error.value = e;
  }
  finally {
    if (version === generation) busy.value = false;
  }
}
async function open(id: string) {
  taskId.value = id;
  searchRunId.value = '';
  error.value = null;
  intent.value = null;
  visible.value = true;
  await refresh();
}
function changed() {
  generation++; busy.value = false;
  preview.value = null;
  confirmed.value = false;
  initialConfirmed.value = false;
}
async function approve() {
  const p = preview.value;
  if (!p?.eligible || !confirmed.value || (p.mode === 'initial_identity' && !initialConfirmed.value) || applying.value || intent.value)
    return;
  applying.value = true;
  intent.value = { phase: 'reconciling' };
  writeIntent(`source:${taskId.value}:${p.controlRevision}:${p.planDigest}`, intent.value);
  try {
    const out = await post<{
      alreadyApplied: boolean;
      recoveryStillRequired: boolean;
    }>(`/downloads/${taskId.value}/reconfirm-source`, { searchRunId: p.searchRunId, controlRevision: p.controlRevision, planDigest: p.planDigest, confirmSourceBinding: true, confirmInitialIdentity: initialConfirmed.value });
    intent.value = { phase: 'queued' };
    writeIntent(`source:${taskId.value}:${p.controlRevision}:${p.planDigest}`, intent.value);
    if (out.recoveryStillRequired)
      error.value = Error('此前确认已应用，但当前又需要来源核对，请重新读取，不重复提交旧确认。');
    emit('completed');
  }
  catch (e) {
    error.value = e;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      writeIntent(`source:${taskId.value}:${p.controlRevision}:${p.planDigest}`, null);
      intent.value = null;
      preview.value = null;
      confirmed.value = false;
    }
  }
  finally {
    applying.value = false;
  }
}
defineExpose({ open });
</script>
<template>
  <el-dialog v-model="visible" title="重新确认下载来源" width="min(700px,94vw)" top="5vh" :close-on-click-modal="false" :show-close="!applying" :close-on-press-escape="!applying" class="acquire-dialog">
    <div class="acquire-form">
      <p>只恢复原任务，媒体身份、下载器和保存路径不变；不删除种子、不移动做种文件，也不另建下载任务。</p>
      <label>
        搜索记录 ID（可选）
        <el-input v-model="searchRunId" :disabled="applying" aria-label="搜索记录 ID" @input="changed"/>
      </label>
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <div v-if="preview" class="source-proof">
        <h3>{{ preview.candidateTitle||'当前任务尚不能确认来源' }}</h3>
        <dl class="acquire-facts">
          <div>
            <dt>原 InfoHash</dt>
            <dd>{{ preview.originalHash||'尚无已确认的种子身份' }}</dd>
          </div>
          <div>
            <dt>新来源 InfoHash</dt>
            <dd>{{ preview.candidateHash||'尚未取得' }}</dd>
          </div>
          <div>
            <dt>下载器观察</dt>
            <dd>{{ preview.downloaderObservation==='present'?'已有相同任务，先核对，不重复添加':preview.downloaderObservation==='absent'?'当前未找到此任务':'尚未确认' }}</dd>
          </div>
        </dl>
        <p v-for="reason in preview.reasons||[]" :key="reason">{{ reason }}</p>
        <label v-if="preview.eligible" class="acquire-consent">
          <input v-model="confirmed" type="checkbox" :disabled="applying||!!intent"/>
          我确认原媒体与任务不变，并批准使用此新来源
        </label>
        <label v-if="preview.eligible&&preview.mode==='initial_identity'" class="acquire-consent">
          <input v-model="initialConfirmed" type="checkbox" :disabled="applying||!!intent"/>
          原任务尚无 InfoHash，我明确确认其首次种子身份
        </label>
      </div>
      <AsyncResultBanner v-if="intent" :status="intent.phase" :message="intent.phase==='queued'?'来源确认已受理，后续任务尚未确认完成。':'确认回执未知，先重新预览核对，不重复提交。'" @refresh="refresh"/>
    </div>
    <template #footer>
      <el-button :loading="busy" :disabled="applying" @click="refresh">重新预览来源</el-button>
      <el-button :disabled="applying" @click="visible=false">关闭</el-button>
      <el-button type="primary" :loading="applying" :disabled="!preview?.eligible||!confirmed||(preview.mode==='initial_identity'&&!initialConfirmed)||busy||!!intent" @click="approve">明确确认并恢复原任务</el-button>
    </template>
  </el-dialog>
</template>
