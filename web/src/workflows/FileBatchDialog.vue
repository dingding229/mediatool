<script setup lang="ts">
import { ref, onUnmounted, watch } from 'vue';
import FileIdentityFields from '../files/FileIdentityFields.vue';
import {createFileIdentity,filePayload,validateFileIdentity} from '../files/identity';
import { post, request } from '../api/client';
import PreviewApprove from '../ui/PreviewApprove.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { canApprove } from '../files/workbench';
import { errorText, date } from '../workflows/readiness';
import { readIntent, writeIntent } from '../acquisition/model';
import { label, readPrivate, savePrivate } from '../organize/model';
const emit = defineEmits<{
  changed: [
  ];
  editing: [
    boolean
  ];
}>(), visible = ref(false), root = ref<any>(null), files = ref<any[]>([]), items = ref<any[]>([]), operation = ref('organize'), destination = ref('.'), nameMode = ref('default'), templates = ref<any>({}), batch = ref<any>(null), busy = ref(false), error = ref<unknown>(null), approveOpen = ref(false), phase = ref<'queued' | 'reconciling' | null>(null);
const verified = ref(false);
let generation = 0, disposed = false;
function cancelIdentification(){for(const row of items.value)row.identity?.dispose();}
watch(visible,v=>{if(!v)cancelIdentification();});
async function identifyRows(current:number){
 let index=0;
 const worker=async()=>{while(index<items.value.length){const row=items.value[index++];if(current!==generation||disposed)return;row.identity=createFileIdentity(row,files.value.find(f=>f.path===row.source)?.name||row.source.split('/').pop());await row.identity.recognize();}};
 await Promise.all([worker(),worker()]);
}
onUnmounted(() => {
  disposed = true; cancelIdentification();
  generation++;
});
async function open(kind: 'candidates' | 'assets', selected: any[]) {
  if (busy.value || !selected.length || selected.length > 20)
    return;
  const current = ++generation;
  visible.value = true;
  emit('editing', true);
  const remembered = readPrivate('media-tool:organize-batch:v1');
  if (remembered?.id) {
    batch.value = remembered;
    phase.value = 'reconciling';
    await readback(false);
    return;
  }
  busy.value = true;
  batch.value = null;
  phase.value = null;
  error.value = null;
  operation.value = 'organize';
  destination.value = '.';
  nameMode.value = 'default';
  try {
    const out: any = await post('/library/organize-selection', kind === 'candidates' ? { candidateIds: selected.map(r => r.id) } : { assetIds: selected.map(r => r.id) });
    if (disposed || current !== generation)
      return;
    root.value = out.root;
    files.value = out.files;
    cancelIdentification();
    items.value = out.rows.map((r: any) => ({ ...r, category:r.destination||'', name: r.name || '', warnings: r.warnings || [] }));
    const settings: any = await request('/settings/application');
    templates.value = settings.value || {};
    await identifyRows(current);
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function openFiles(chosenRoot: any, chosenFiles: any[]) {
  if (busy.value || !chosenFiles.length || chosenFiles.length > 20)
    return;
  const remembered = readPrivate('media-tool:organize-batch:v1');
  visible.value = true;
  emit('editing', true);
  error.value = null;
  if (remembered?.id) {
    batch.value = remembered;
    phase.value = 'reconciling';
    await readback(false);
    return;
  }
  const current = ++generation;
  busy.value = true;
  batch.value = null;
  phase.value = null;
  root.value = { ...chosenRoot };
  files.value = chosenFiles.map(f => ({ ...f }));
  cancelIdentification();
  items.value = [];
  operation.value = 'organize';
  destination.value = '.';
  nameMode.value = 'default';
  try {
    const settings: any = await request('/settings/application');
    if (current !== generation || disposed)
      return;
    templates.value = settings.value || {};
    cancelIdentification();
    items.value=chosenFiles.map(file=>({title:file.name,year:'',mediaType:'unknown',category:'',source:file.path,name:'',warnings:[]}));
    await identifyRows(current);
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function preview() {
  if (busy.value || phase.value || items.value.some(row=>row.identity?.loading))
    return;
  const current = ++generation, draft = JSON.stringify([items.value.map(filePayload), operation.value, destination.value, nameMode.value]);
  busy.value = true;
  error.value = null;
  try {
    if(operation.value==='organize'){for(const row of items.value){const message=validateFileIdentity(row);if(message)throw Error(message);}}
    if (operation.value === 'organize' && nameMode.value !== 'direct' && items.value.some(r => (r.episodes?.length || 0) > 1))
      throw Error('多集文件请使用逐项自定义完整名称，不套用单集模板。');
    const next: any = await post('/files/batches/preview', { items: items.value.map(row => ({ ...filePayload(row), expectedFileId: files.value.find(f => f.path === row.source)?.id, expectedSize: files.value.find(f => f.path === row.source)?.size, connectionId: root.value.id, operation: operation.value, destination: destination.value, includeSidecars: true, name: nameMode.value === 'direct' ? row.name : '' })) });
    if (disposed || current !== generation)
      return;
    if (draft !== JSON.stringify([items.value.map(filePayload), operation.value, destination.value, nameMode.value]))
      throw Error('输入已变化，请重新预览后再核对。');
    batch.value = next;
    verified.value = false;
  }
  catch (e) {
    error.value = e;
  }
  finally {
    busy.value = false;
  }
}
async function approve() {
  if (!canApprove(batch.value) || busy.value || phase.value)
    return;
  verified.value = false;
  const id = batch.value.id, key = 'organize-batch:' + id;
  phase.value = readIntent(key)?.phase || null;
  if (phase.value)
    return;
  busy.value = true;
  phase.value = 'reconciling';
  writeIntent(key, { phase: 'reconciling' });
  savePrivate('media-tool:organize-batch:v1', batch.value);
  try {
    batch.value = await post(`/files/batches/${id}/approve`, { digest: batch.value.digest, confirm: true });
    phase.value = 'queued';
    writeIntent(key, { phase: 'queued' });
    approveOpen.value = false;
  }
  catch (e) {
    error.value = e;
    approveOpen.value = false;
  }
  finally {
    busy.value = false;
  }
}
async function readback(reconcile = false) {
  if (!batch.value || busy.value)
    return;
  busy.value = true;
  try {
    const id = batch.value.id;
    batch.value = reconcile ? await post(`/files/batches/${id}/reconcile`, {}) : await request(`/files/batches/${id}`);
    verified.value = true;
    if (['succeeded', 'partial', 'failed', 'cancelled', 'expired'].includes(batch.value.state)) {
      savePrivate('media-tool:organize-batch:v1', null);
      phase.value = null;
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
function close() {
  if (!busy.value) {
    visible.value = false;
    emit('editing', false);
    emit('changed');
  }
}
defineExpose({ open, openFiles });
</script>
<template>
  <el-dialog v-model="visible" title="批次整理工作台" width="min(820px,94vw)" :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy" class="media-dialog" @closed="emit('editing',false)">
    <PreviewApprove :current-step="!batch?1:phase&&!verified?3:batch.state==='preview'?2:verified&&['succeeded','partial','failed'].includes(batch.state)?4:3">
      <div class="media-form">
        <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
        <template v-if="!batch">
          <p>预览不移动文件，需逐项核对原路径和目标。每批最多20个普通媒体文件，保留做种源。</p>
          <label>
            动作
            <el-select v-model="operation" aria-label="批次动作">
              <el-option value="organize" label="分类整理"/>
              <el-option value="rename" label="重命名"/>
              <el-option value="move" label="移动（保留原名）"/>
            </el-select>
          </label>
          <label>
            目标相对目录（. 使用分类目录）
            <el-input v-model="destination" aria-label="批次目标目录"/>
          </label>
          <label>
            命名方式
            <el-select v-model="nameMode" aria-label="批次命名方式">
              <el-option value="default" label="系统电影 / 剧集模板"/>
              <el-option value="direct" label="逐项自定义名称"/>
            </el-select>
          </label>
          <p>{{ root?.name }} · {{ root?.path }}</p>
          <p>
            电影模板：
            <code>{{ templates.movieTemplate||'—' }}</code>
          </p>
          <p>
            剧集模板：
            <code>{{ templates.seriesTemplate||'—' }}</code>
          </p>
          <article v-for="row in items" :key="row.source" class="organize-record">
            <h3>{{ row.source }}</h3>
            <FileIdentityFields v-if="row.identity" :identity="row.identity" :draft="row" :disabled="busy" batch/>
            <label v-if="nameMode==='direct'">
              完整文件名
              <el-input v-model="row.name" aria-label="批次自定义文件名"/>
            </label>
          </article>
          <el-button type="primary" :loading="busy" :disabled="items.some(row=>row.identity?.loading)" @click="preview">预览全部变更</el-button>
        </template>
        <template v-else>
          <p>{{ phase&&!verified?(phase==='queued'?'已排队，待核验':'正在核对上次结果'):label(batch.state) }} · 预览截止 {{ date(batch.expiresAt) }}</p>
          <article v-for="row in batch.items" :key="row.ordinal" class="organize-record">
            <h3>{{ row.source }}</h3>
            <p>{{ phase&&!verified?'等待核验':label(row.state) }} · {{ !phase||verified?row.message||'—':'以重新读取的记录为准' }}</p>
            <p>目录分类：{{row.result?.plan?.category||'—'}}</p>
            <p v-for="(item,index) in row.result?.plan?.items||[]" :key="index">最终路径：{{row.result.plan.root}}/{{item.target}} · 命名结果：{{item.target.split('/').pop()}}</p>
          </article>
          <AsyncResultBanner v-if="phase" :can-refresh="false" :status="phase" message="批准不等于文件整理完成，先读取或只读核对，不重放写入。" @refresh="readback(false)"/>
          <div class="media-actions">
            <el-button v-if="canApprove(batch)&&!phase" type="primary" @click="approveOpen=true">明确批准整个批次</el-button>
            <el-button :loading="busy" @click="readback(false)">核对结果</el-button>
            <el-button @click="readback(true)">核对结果（不重放）</el-button>
            <RouterLink :to="{path:'/organize',query:{tab:'history',batchId:batch.id}}" @click="close">查看持久化批次</RouterLink>
          </div>
        </template>
      </div>
    </PreviewApprove>
    <template #footer>
      <el-button :disabled="busy" @click="close">关闭</el-button>
    </template>
  </el-dialog>
  <ConfirmAction v-model:visible="approveOpen" title="批准文件批次" description="按刚读取的原位置与目标计划执行，计划过期或文件变化会拒绝执行。" impact-scope="会改名或移动所选云盘文件及同名侧车；不会移动本地下载做种源，不重写固定播放链接。" confirm-text="确认批准" :loading="busy" class="media-dialog" @confirm="approve"/>
</template>
