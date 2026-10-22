<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';

import { onMounted, reactive, ref, watch } from 'vue';
import { listMediaDevices, listMediaHistory, revokeMediaDevice } from '../api/mediaServer';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText } from '../workflows/readiness';
const props = defineProps<{
  userId: string;
  userName: string;
}>(), tab = ref('devices'), page = usePageNumber(), data = reactive(useResource<any>(() => tab.value === 'devices' ? listMediaDevices(props.userId, (page.value - 1) * 50) : listMediaHistory(props.userId, (page.value - 1) * 50))), op = reactive(useOperation('devices')), confirm = ref(false), target = ref<any>(null);
async function load() {
  await data.load();
  if (op.pending) {
    try {
      const p = JSON.parse(op.pending.key), proof = await listMediaDevices(p.userId, p.startIndex, 50);
      const row = proof.items.find(r => r.id === p.id);
      if (row?.activeTokens === 0 || proof.total === 0)
        op.clear();
    }
    catch (e) {
      op.error = e;
    }
  }
}
async function revoke() {
  if (!target.value)
    return;
  await op.run(JSON.stringify({ id: target.value.id, userId: props.userId, startIndex: (page.value - 1) * 50 }), () => revokeMediaDevice(props.userId, target.value.id));
  confirm.value = false;
  await load();
}
function changeTab(value: string) {
  tab.value = value;
  page.value = 1;
  data.data = null;
  void load();
}
watch(() => props.userId, () => changeTab('devices'));
onMounted(load);
defineExpose({ load, tab, page, data, changeTab, revoke, target, op });
</script>
<template>
  <nav class="media-actions mt-segments" aria-label="用户活动分类">
    <el-button :aria-pressed="tab==='devices'" @click="changeTab('devices')">登录设备</el-button>
    <el-button :aria-pressed="tab==='history'" @click="changeTab('history')">观看历史</el-button>
  </nav>
  <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" :empty="!data.data?.items.length" @reload="load"/>
  <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
  <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
  <article v-for="row in data.data?.items||[]" :key="row.id||row.itemId" class="organize-record">
    <h3>{{ row.name||'—' }}</h3>
    <template v-if="tab==='devices'">
      <p>{{ row.client||'—' }} · {{ row.appVersion||'—' }} · 有效登录 {{ row.activeTokens??'—' }}</p>
      <p>最近活动 {{ date(row.lastSeenAt) }}</p>
      <el-button :disabled="!!op.pending||op.busy||!!data.error" @click="target=row;confirm=true">撤销设备登录</el-button>
    </template>
    <p v-else>观看 {{ row.playCount??'—' }} 次 · 已观看 {{ Math.floor((row.positionTicks||0)/600000000) }} 分钟 · {{ row.played?'已看完':'未看完' }} · {{ date(row.lastPlayedAt) }}</p>
  </article>
  <PagePagination v-model:current-page="page" :page-size="50" :total="data.data.total" layout="prev,pager,next,total" @current-change="load"/>
  <ConfirmAction v-model:visible="confirm" title="撤销设备登录" :description="`撤销 ${userName} 在 ${target?.name||'此设备'} 的登录？`" impact-scope="该设备需要重新输入密码，其他设备不受影响；不删除媒体或观看记录。" confirm-text="确认撤销" :loading="op.busy" class="media-dialog" @confirm="revoke"/>
</template>
