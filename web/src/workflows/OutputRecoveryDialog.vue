<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue';
import { APIError, post, request } from '../api/client';
import StateView from '../ui/StateView.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText, stateFor } from '../workflows/readiness';
import { readIntent, writeIntent, type Intent } from './intents';
const recoveryScope='只恢复已校验文件的入库；不重新上传、不改已确认媒体身份，也不重写 STRM 固定链接。';
const emit = defineEmits<{
  completed: [
  ];
}>(), visible = ref(false), id = ref(''), preview = ref<any>(null), busy = ref(false), applying = ref(false), error = ref<unknown>(null), intent = ref<Intent | null>(null);
const scope = () => `output:${id.value}:${preview.value?.controlRevision}:${preview.value?.planDigest}`;
let generation = 0;
watch(visible, open => {
  if (!open)
    generation++;
});
onUnmounted(() => {
  generation++;
});
async function refresh() {
  if (applying.value)
    return;
  const version = ++generation;
  busy.value = true;
  error.value = null;
  try {
    const result = await request(`/transfers/${id.value}/output-preview`);
    if (version !== generation)
      return;
    preview.value = result;
    intent.value = readIntent(scope());
  }
  catch (e) {
    if (version === generation)
      error.value = e;
  }
  finally {
    if (version === generation)
      busy.value = false;
  }
}
async function open(value: string) {
  id.value = value;
  preview.value = null;
  intent.value = null;
  visible.value = true;
  await refresh();
}
async function resume() {
  if (!preview.value?.ready || preview.value.state !== 'needs_output_reconciliation' || applying.value || intent.value)
    return;
  const key = scope();
  applying.value = true;
  intent.value = { phase: 'reconciling' };
  writeIntent(key, intent.value);
  try {
    await post(`/transfers/${id.value}/resume-import`, { controlRevision: preview.value.controlRevision, planDigest: preview.value.planDigest });
    intent.value = { phase: 'queued' };
    writeIntent(key, intent.value);
    emit('completed');
  }
  catch (e) {
    error.value = e;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      writeIntent(key, null);
      intent.value = null;
      preview.value = null;
    }
  }
  finally {
    applying.value = false;
  }
}
defineExpose({ open });
</script>
<template>
  <el-dialog v-model="visible" title="核对并恢复已有入库" width="min(700px,94vw)" top="5vh" :close-on-click-modal="false" :show-close="!applying" :close-on-press-escape="!applying" class="output-recovery-dialog">
    <div class="output-recovery-form">
      <p>{{recoveryScope}}</p>
      <StateView v-if="busy" state="loading" title="正在核对已有输出…"/>
      <el-alert v-if="error" :title="errorText(error)+'；请重新核对后再确认。'" type="error" :closable="false"/>
      <template v-if="preview">
        <p v-if="preview.message&&preview.message!==recoveryScope">{{ preview.message }}</p>
        <div class="output-recovery-paths" aria-label="已有入库输出核对">
          <section>
            <h3>已有输出</h3>
            <p v-for="path in preview.existingPaths" :key="path">{{ path }}</p>
          </section>
          <section>
            <h3>应有布局</h3>
            <p v-for="path in preview.expectedPaths" :key="path">{{ path }}</p>
          </section>
        </div>
        <RouterLink v-if="preview.needsMigration||preview.needsArtifactRecovery" :to="{path:'/organize',query:{tab:'assets',assetId:preview.assetId}}" @click="visible=false">前往云盘索引核对该资产、恢复已有输出并明确预览迁移</RouterLink>
      </template>
      <AsyncResultBanner v-if="intent" :status="intent.phase" :message="intent.phase==='queued'?'入库恢复已受理；不重新上传，请核对后续状态。':'恢复回执未知，先重新核对，不重复提交。'" @refresh="refresh"/>
    </div>
    <template #footer>
      <el-button :loading="busy" :disabled="applying" @click="refresh">重新核对</el-button>
      <el-button :disabled="applying" @click="visible=false">关闭</el-button>
      <el-button type="primary" :loading="applying" :disabled="!preview?.ready||preview?.state!=='needs_output_reconciliation'||busy||!!intent" @click="resume">明确恢复入库（不重新上传）</el-button>
    </template>
  </el-dialog>
</template>
<style scoped>
.output-recovery-form{display:grid;gap:16px;min-width:0}.output-recovery-paths{display:grid;grid-template-columns:minmax(0,1fr);gap:12px;min-width:0;max-height:30vh;overflow:auto}.output-recovery-paths section{min-width:0;padding:12px;border:1px solid var(--mt-border-color);border-radius:8px}.output-recovery-paths h3{margin:0;font-size:14px}.output-recovery-form p{font-size:13px;line-height:1.7;overflow-wrap:anywhere;color:var(--mt-text-muted)}.output-recovery-form a{display:inline-flex;align-items:center;min-height:44px;color:var(--mt-color-primary)}
:global(.output-recovery-dialog .el-button),:global(.output-recovery-dialog .el-dialog__headerbtn){min-height:44px}:global(.output-recovery-dialog .el-dialog__headerbtn){min-width:44px}:global(.output-recovery-dialog .el-dialog__footer){display:flex;flex-wrap:wrap;gap:8px;justify-content:flex-end}:global(.output-recovery-dialog .el-button){height:auto;white-space:normal;margin-left:0;max-width:100%}@media(max-width:600px){:global(.output-recovery-dialog .el-dialog__footer){display:grid;grid-template-columns:minmax(0,1fr)}}
</style>
