<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue';
import { post } from '../api/client';
import StateView from '../ui/StateView.vue';
import { sourceState } from '../media/model';
import { errorText, date } from '../workflows/readiness';
const path = ref(''), pickCode = ref(''), preview = ref<any>(null), diagnosis = ref<any>(null), loading = ref(false), error = ref<unknown>(null), feedback = ref(''), paths = ref(''), links = ref<any[]>([]), batchBusy = ref(false), batchError = ref<unknown>(null);
withDefaults(defineProps<{
  allowedRoots?: string[];
}>(), { allowedRoots: () => [] });
const lastKind = ref<'preview' | 'diagnose'>('preview');
let epoch = 0, batchEpoch = 0, disposed = false;
watch([path, pickCode], () => {
  epoch++;
  preview.value = null;
  diagnosis.value = null;
  error.value = null;
  feedback.value = '';
  loading.value = false;
});
watch(paths, () => {
  batchEpoch++;
  links.value = [];
  batchError.value = null;
  batchBusy.value = false;
});
async function run(kind: 'preview' | 'diagnose') {
  if (loading.value || !path.value.trim() && !pickCode.value.trim())
    return;
  lastKind.value = kind;
  const current = ++epoch;
  loading.value = true;
  error.value = null;
  if (kind === 'preview')
    preview.value = null;
  else
    diagnosis.value = null;
  try {
    const body = { path: path.value.trim(), pickCode: pickCode.value.trim() };
    const out: any = kind === 'preview' ? await post('/strm/preview', body) : await post('/cloudStorage/diagnose', body);
    if (disposed || current !== epoch)
      return;
    if (kind === 'preview')
      preview.value = out;
    else
      diagnosis.value = out;
  }
  catch (e) {
    if (current === epoch)
      error.value = e;
  }
  finally {
    if (current === epoch)
      loading.value = false;
  }
}
async function batchPreview() {
  if (batchBusy.value)
    return;
  const current = ++batchEpoch;
  batchBusy.value = true;
  batchError.value = null;
  links.value = [];
  try {
    const out: any = await post('/strm/batch-preview', { paths: paths.value.split(/\r?\n/).map(s => s.trim()).filter(Boolean) });
    if (!disposed && current === batchEpoch)
      links.value = out;
  }
  catch (e) {
    if (current === batchEpoch)
      batchError.value = e;
  }
  finally {
    if (current === batchEpoch)
      batchBusy.value = false;
  }
}
async function copy() {
  try {
    await navigator.clipboard.writeText(preview.value.url);
    feedback.value = '固定解析链接已复制';
  }
  catch {
    feedback.value = '复制失败，请手动选择下方完整内容。';
  }
}
function download() {
  if (!preview.value)
    return;
  const url = URL.createObjectURL(new Blob([preview.value.content], { type: 'text/plain;charset=utf-8' })), a = document.createElement('a');
  a.href = url;
  a.download = (path.value.split('/').pop() || 'media').replace(/\.[^.]+$/, '') + '.strm';
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
onUnmounted(() => {
  disposed = true;
  epoch++;
  batchEpoch++;
});
</script>
<template>
  <section class="media-surface">
    <h2>播放链路诊断</h2>
    <p>播放时直接跳转到 115 官方地址，服务器不中转流量。</p>
    <p>允许的根目录：{{ allowedRoots.length?allowedRoots.join('、'):'—' }}</p>
    <form class="media-form" @submit.prevent="run('preview')">
      <label>
        CloudDrive2 完整路径
        <el-input v-model="path" aria-label="CloudDrive2 完整路径" placeholder="输入已有媒体的完整路径"/>
      </label>
      <label>
        115 文件标识（可选）
        <el-input v-model="pickCode" aria-label="115文件标识"/>
      </label>
      <p>文件标识可留空；只填写真实 PickCode，不用 CloudDrive 文件 ID 替代。</p>
      <div class="media-actions">
        <el-button native-type="submit" :disabled="loading||!path.trim()&&!pickCode.trim()">生成固定链接</el-button>
        <el-button :loading="loading" :disabled="!path.trim()&&!pickCode.trim()" @click="run('diagnose')">诊断此文件</el-button>
      </div>
    </form>
    <StateView v-if="loading" state="loading" title="正在读取链路信息…"/>
    <StateView v-else-if="error" :state="sourceState(error)" title="链路核对未完成" :description="errorText(error)" retry-text="重新读取链路信息" @retry="run(lastKind)"/>
    <section v-if="preview" class="organize-plan">
      <h3>固定链接与 STRM 原文</h3>
      <el-input :model-value="preview.content" type="textarea" :rows="5" readonly aria-label="STRM原文"/>
      <p>{{ preview.url }}</p>
      <p>{{ preview.warning||'—' }}</p>
      <div class="media-actions">
        <el-button @click="copy">复制完整链接</el-button>
        <el-button @click="download">下载 STRM</el-button>
      </div>
      <p role="status">{{ feedback }}</p>
    </section>
    <section v-if="diagnosis" class="organize-plan">
      <h3>诊断结果</h3>
      <p>文件：{{ diagnosis.file?.name||'—' }} · {{ diagnosis.file?.path||'—' }}</p>
      <p>大小：{{ diagnosis.file?.size??'—' }} 字节</p>
      <p>投递方式：{{ diagnosis.playback?.mode==='redirect'?'支持跳转':'此文件暂不能按直链方式投递' }}</p>
      <p>目标地址校验：{{ diagnosis.playback?.targetValidated?'已校验':'尚未确认' }} · 媒体字节校验：{{ diagnosis.playback?.mediaBytesChecked?'已核对':'未核对' }}</p>
      <p>播放器标识要求：{{ diagnosis.playback?.requiresUserAgent?'需使用实际播放器标识':'无额外要求' }} · 请求头要求：{{ diagnosis.playback?.requiresHeaders?'存在额外请求头要求':'无额外要求' }}</p>
      <p>有效期 {{ date(diagnosis.playback?.expiresAt) }} · {{ diagnosis.note||'—' }}</p>
    </section>
  </section>
  <details class="media-surface files-link-batch">
    <summary>批量链接预览</summary>
    <p>每行一个已有媒体路径；仅生成预览，不写入或修复文件。</p>
    <el-input v-model="paths" type="textarea" :rows="4" aria-label="批量媒体路径"/>
    <el-button :disabled="!paths.trim()" :loading="batchBusy" @click="batchPreview">预览这些路径</el-button>
    <StateView v-if="batchError" :state="sourceState(batchError)" title="批量链接预览失败" :description="errorText(batchError)" retry-text="重新预览路径" @retry="batchPreview"/>
    <article v-for="(row,index) in links" :key="index" class="organize-record">
      <h3>{{ row.path }}</h3>
      <p>{{ row.valid?'已取得链接预览':'此项未通过' }} · {{ row.message||'—' }}</p>
      <el-input v-if="row.content||row.url" :model-value="row.content||row.url" type="textarea" readonly :aria-label="'路径预览 '+(index+1)"/>
    </article>
  </details>
</template>
