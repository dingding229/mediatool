<script setup lang="ts">
import CronInput from '../ui/CronInput.vue';
const cronValid=ref(false);
import {scheduleLabel} from '../presentation/labels';
import { onMounted, ref, reactive } from 'vue';
import { request } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText } from '../workflows/readiness';
const data = reactive(useResource<any>(() => request('/sites/metrics/schedule'))), visible = ref(false), form = reactive({ enabled: false, cron: '', revision: 0 }), latest = ref<any>(null), op = reactive(useOperation('metric-schedule'));
async function load() {
  const value = await data.load();
  if (visible.value && latest.value && value)
    latest.value = value;
  if (op.pending && value && value.revision !== Number(op.pending.key)) {
    op.clear();
    latest.value = value;
  }
}
async function open() {
  await load();
  if (data.data) {
    Object.assign(form, { enabled: data.data.enabled, cron: data.data.cron, revision: data.data.revision });
    latest.value = null;
    visible.value = true;
  }
}
async function save() {
  if(!cronValid.value||latest.value||op.pending||op.busy)return;
  const body = { ...form };
  await op.run(String(form.revision), () => request('/sites/metrics/schedule', { method: 'PUT', body: JSON.stringify(body) }));
  await load();
  if (op.error)
    latest.value = data.error ? { revision: null } : data.data || { revision: null };
  else if (!op.pending)
    visible.value = false;
}
onMounted(load);
defineExpose({ load, open, save, form, data, latest, op });

</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>指标采集计划</h2>
      <el-button @click="open">编辑采集计划</el-button>
    </header>
    <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" @reload="load"/>
    <p v-if="data.data">{{ data.data.enabled?'已启用':'已停用' }} · {{ scheduleLabel(data.data.cron) }} · 下次 {{ data.data.nextRunAt===null?'计划已结束':date(data.data.nextRunAt) }}</p>

    <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'计划变更已排队，尚未核验。':'正在核对当前计划，不重复保存。'" @refresh="load"/>
  </section>
  <el-dialog v-model="visible" title="指标采集计划" width="min(600px,94vw)" class="media-dialog u5-dialog" :close-on-click-modal="false">
    <div class="u5-form">
      <p v-if="data.data">最近调度 {{ date(data.data.lastDispatchedAt) }}</p>
      <label class="u5-check">
        <input v-model="form.enabled" type="checkbox" aria-label="启用定时采集" :disabled="op.busy||!!op.pending"/>
        启用定时采集
      </label>
      <CronInput v-model="form.cron" purpose="site-metrics" label="站点指标 Cron" :disabled="op.busy||!!op.pending" @valid="cronValid=$event"/>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <section v-if="latest">
        <p>当前计划：{{ scheduleLabel(latest.cron) }} · {{ latest.enabled?'启用':'停用' }}；你的输入已保留。</p>

        <el-button :disabled="latest.revision==null" @click="form.revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
      </section>
    </div>
    <template #footer>
      <el-button :disabled="op.busy" @click="visible=false">取消</el-button>
      <el-button :loading="op.busy" :disabled="!!latest||!!op.pending||!cronValid" @click="save" type="primary" class="mt-save-button">保存采集计划</el-button>
    </template>
  </el-dialog>
</template>
