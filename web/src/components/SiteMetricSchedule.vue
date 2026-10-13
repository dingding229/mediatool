<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { Timer } from "@element-plus/icons-vue";
import { request } from "../api/client";
interface Schedule {
  enabled: boolean;
  cron: string;
  revision: number;
  nextRunAt: string;
  lastDispatchedAt: string | null;
}
const schedule = ref<Schedule | null>(null),
  dialog = ref(false),
  busy = ref(false),
  error = ref(""),
  enabled = ref(true),
  cron = ref("0 * * * *"),
  revision = ref(0);
let timer: ReturnType<typeof setInterval> | undefined;
async function load() {
  try {
    schedule.value = await request<Schedule>("/sites/metrics/schedule");
    error.value = "";
  } catch (e) {
    error.value = String(e);
  }
}
async function edit() {
  error.value = "";
  busy.value = true;
  await load();
  busy.value = false;
  if (!schedule.value || error.value) return;
  enabled.value = schedule.value.enabled;
  cron.value = schedule.value.cron;
  revision.value = schedule.value.revision;
  dialog.value = true;
}
async function save() {
  busy.value = true;
  error.value = "";
  try {
    schedule.value = await request<Schedule>("/sites/metrics/schedule", {
      method: "PUT",
      body: JSON.stringify({
        enabled: enabled.value,
        cron: cron.value,
        revision: revision.value,
      }),
    });
    dialog.value = false;
    ElMessage.success("定时刷新设置已保存");
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
onMounted(() => {
  load();
  timer = setInterval(() => {
    if (!document.hidden && !dialog.value && !busy.value) load();
  }, 30000);
});
onUnmounted(() => clearInterval(timer));
</script>
<template>
  <div class="schedule-bar">
    <div>
      <p v-if="schedule" class="field-hint">
        <strong>{{
          schedule.enabled
            ? `自动刷新 · ${schedule.cron} · Asia/Shanghai`
            : "自动刷新已关闭"
        }}</strong
        ><span v-if="schedule.enabled">
          · 下次调度
          {{ new Date(schedule.nextRunAt).toLocaleString("zh-CN") }}</span
        >
      </p>
      <p class="field-hint">
        后台采集站点数据，关闭页面仍运行；本页每30秒更新已保存快照，不会直接请求站点。
      </p>
    </div>
    <el-button :icon="Timer" :loading="busy && !dialog" @click="edit"
      >定时刷新</el-button
    >
  </div>
  <el-alert
    v-if="error && !dialog"
    :title="error"
    type="error"
    :closable="false"
  />
  <el-dialog
    v-model="dialog"
    title="站点数据定时刷新"
    width="min(540px,94vw)"
    :close-on-click-modal="false"
  >
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <form class="schedule-form" @submit.prevent="save">
      <el-checkbox v-model="enabled" :disabled="busy">启用定时刷新</el-checkbox
      ><label for="metrics-cron">Cron 表达式</label><el-input id="metrics-cron" v-model="cron" aria-label="站点刷新Cron" :disabled="busy" placeholder="0 * * * *"/>
      <p class="field-hint">五字段：分 时 日 月 周。时区 Asia/Shanghai，最少15分钟。例如 0 * * * * 每小时，0 9,21 * * * 每天9点和21点。仅刷新启用的站点，不重复并发采集。</p>
      <el-button type="primary" native-type="submit" :loading="busy"
        >保存定时刷新</el-button
      >
    </form>
  </el-dialog>
</template>
<style scoped>
.schedule-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}
.schedule-bar strong {
  color: var(--text);
}
.schedule-form {
  display: grid;
  gap: 12px;
}
.schedule-form :deep(.el-checkbox) {
  min-height: 44px;
}
.schedule-form :deep(.el-button) {
  justify-self: start;
  min-height: 44px;
}
@media (max-width: 680px) {
  .schedule-bar {
    align-items: stretch;
    flex-direction: column;
  }
  .schedule-bar .el-button {
    align-self: flex-start;
  }
}
</style>
