<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import {scheduleLabel} from '../presentation/labels';
import { onMounted, reactive, ref } from 'vue';
import { request } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { policyFields } from './model';
import ResourceState from '../workflows/ResourceState.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText } from '../workflows/readiness';
const read = reactive(useResource<any>(() => request('/ops'))), form = ref<any>(null), revision = ref(0), latest = ref<any>(null), op = reactive(useOperation('ops-policy'));
async function load() {
  try {
    const out = await read.load();
    if (out && !form.value) {
      form.value = JSON.parse(JSON.stringify(out.policy.value));
      revision.value = out.policy.revision;
    }
    if (out && form.value && out.policy.revision !== revision.value)
      latest.value = out.policy;
    if (op.pending && out && out.policy.revision !== Number(op.pending.key))
      op.clear();
  }
  catch (e) {
    read.error = e;
  }
}
async function reloadPolicy() {
  await load();
  if (read.data && !read.error) {
    form.value = JSON.parse(JSON.stringify(read.data.policy.value));
    revision.value = read.data.policy.revision;
    latest.value = null;
  }
}
async function save() {
  if (!form.value || latest.value || op.pending)
    return;
  await op.run(String(revision.value), () => request('/ops/policy', { method: 'PUT', body: JSON.stringify({ value: form.value, revision: revision.value }) }));
  await load();
  if (op.error && read.error)
    latest.value = { revision: null };
  if (!op.error && !op.pending)
    await reloadPolicy();
}
onMounted(load);
defineExpose({ load, reloadPolicy, save, form, revision, latest, op, read });
usePageRefresh(load,'maintenance');
</script>
<template>
  <section class="media-surface">
    <h2>备份、采样与告警策略</h2>
    <ResourceState :loading="read.loading" :error="read.error" @reload="load"/>
    <form v-if="form" class="u5-form" @submit.prevent="save">
      <div class="u6-policy">
        <label class="u5-check">
          <input v-model="form.backupEnabled" type="checkbox"/>
          启用定时备份
        </label>
        <label>
          备份计划（Asia/Shanghai）
          <input v-model="form.backupCron" aria-label="备份Cron"/>
        </label>
        <label class="u5-check">
          <input v-model="form.alertsEnabled" type="checkbox"/>
          启用阈值告警
        </label>
        <label v-for="field in policyFields" :key="field.key">
          {{ field.label }}
          <input v-model.number="form[field.key]" type="number" :aria-label="field.label" :min="field.min" :max="field.max" :step="field.step||1" required/>
        </label>
      </div>
      <p>下次备份 {{ date(read.data?.policy.nextBackupAt) }}；关闭计划不删除已有备份，固定备份不参与保留清理。</p>
      <p v-if="latest">配置已被修改，已重新读取；你的输入已保留。</p>
      <details v-if="latest?.value">
        <summary>对照当前保存策略（只读）</summary>
        <p>定时备份：{{ latest.value.backupEnabled?'启用':'停用' }} · {{ scheduleLabel(latest.value.backupCron) }}；阈值告警：{{ latest.value.alertsEnabled?'启用':'停用' }}</p>
        <dl class="u5-detail-list">
          <div v-for="field in policyFields" :key="field.key">
            <dt>{{ field.label }}</dt>
            <dd>{{ latest.value[field.key]??'—' }}</dd>
          </div>
        </dl>
      </details>
      <div v-if="latest" class="media-actions">
        <el-button @click="load">重新核对策略</el-button>
        <el-button :disabled="latest.revision==null" @click="revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
      </div>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" message="策略保存结果尚未确认，请先重新读取核对。" @refresh="load"/>
      <div class="media-actions">
        <el-button :disabled="!!op.pending||!!latest||!!read.error" native-type="submit">保存维护策略</el-button>
        <el-button @click="reloadPolicy">放弃草稿</el-button>
      </div>
    </form>
  </section>
</template>
