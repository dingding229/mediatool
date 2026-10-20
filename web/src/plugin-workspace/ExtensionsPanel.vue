<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { extensions, runHistory, runExtension } from '../api/plugins';
import { useResource } from '../workflows/useResource';
import { savePrivate } from '../organize/model';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import { date, errorText } from '../workflows/readiness';
const props = defineProps<{
  id: string;
  running: boolean;
}>(), page = ref(1), read = reactive(useResource(() => Promise.all([extensions(props.id), runHistory(props.id, page.value)]))), op = reactive(useOperation('plugin-run')), selected = ref<any>(null), confirm = ref(false), detail = ref<any>(null), open = ref(false);
const runnable = computed(() => read.data?.[0].metadata.descriptors.filter(d => ['service', 'command', 'action'].includes(d.kind)) || []), states: Record<string, string> = { pending: '等待执行', running: '执行中', succeeded: '已完成', failed: '失败', unknown: '结果未知，请先核对', canceled: '已取消' };
async function load() {
  await read.load();
  if (op.pending && !read.error && read.data) {
    const p = JSON.parse(op.pending.key);
    if (p.runId && read.data[1].some(r => r.id === p.runId && !['pending', 'running', 'unknown'].includes(r.state)))
      op.clear();
  }
}
async function run() {
  const d = selected.value, out = await op.run(JSON.stringify({ descriptor: d.id }), () => runExtension(props.id, { kind: d.kind, id: d.id, operationKey: op.pending.token, confirm: true }));
  confirm.value = false;
  if (out && op.pending) {
    op.pending.key = JSON.stringify({ descriptor: d.id, ...out });
    savePrivate('media-tool:u5:plugin-run', op.pending);
  }
  await load();
}
onMounted(load);
</script>
<template>
  <section>
    <h3>服务与命令</h3>
    <p v-if="!running">插件尚未运行；请启用后再执行。</p>
    <ResourceState :loading="read.loading" :error="read.error" @reload="load"/>
    <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" message="请求尚未核验完成，请先读取运行记录，不重复执行。" @refresh="load"/>
    <article v-for="d in runnable" :key="d.kind+':'+d.id" class="organize-record">
      <h4>{{ d.name||d.id }}</h4>
      <p>{{ d.description||'—' }}</p>
      <el-button :disabled="!running||!!op.pending||!!read.error" @click="selected=d;confirm=true">执行 {{ d.name||d.id }}</el-button>
    </article>
    <h3>执行历史</h3>
    <ResourceState :empty="!!read.data&&!read.data[1].length"/>
    <article v-for="r in read.data?.[1]||[]" :key="r.id" class="organize-record">
      <p>{{ states[r.state]||'状态待核对' }} · {{ date(r.createdAt) }}</p>
      <p>{{ r.reason||'—' }}</p>
      <el-button @click="detail=r;open=true">运行详情</el-button>
    </article>
    <div class="media-actions">
      <el-button :disabled="page===1" @click="page--;load()">上一页运行</el-button>
      <el-button :disabled="(read.data?.[1].length||0)<50" @click="page++;load()">下一页运行</el-button>
    </div>
    <ConfirmAction v-model:visible="confirm" title="执行插件扩展" :description="'将执行 '+(selected?.name||selected?.id||'此扩展')" impact-scope="可能产生插件权限范围内的业务变更；结果未知时先核对，不会自动重试。" confirm-text="确认执行" class="media-dialog" :loading="op.busy" @confirm="run"/>
    <DetailDrawer v-model:visible="open" title="插件运行详情" :technical-data="detail" class="media-detail"/>
  </section>
</template>
