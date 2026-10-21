<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { onMounted, onUnmounted, ref } from 'vue';
import { post, request } from '../api/client';
import ConfirmAction from '../ui/ConfirmAction.vue';
import StateView from '../ui/StateView.vue';
import { date } from '../workflows/readiness';
interface Message {
  id: string;
  title: string;
  body: string;
  createdAt: string;
  telegramState: string;
  deliveryId?: string;
  deliveryError: string;
}
const items = ref<Message[]>([]), loading = ref(true), error = ref(''), pending = ref(false), target = ref<Message | null>(null), open = ref(false), busy = ref(false), uncertain = ref(false);
const labels: Record<string, string> = { sent: '已发送', pending: '等待投递', retry: '等待重试', sending: '正在投递', outcome_unknown: '结果未知，需先在接收端核对', failed: '投递失败', not_configured: '仅站内通知' };
let disposed = false, timer: ReturnType<typeof setInterval> | undefined;
async function load() {
  loading.value = true;
  try {
    const out = await request<Message[]>('/notifications');
    if (!disposed) {
      items.value = out;
      error.value = '';
      uncertain.value = false;
    }
  }
  catch {
    error.value = '投递记录读取失败，不能视为没有通知。';
  }
  finally {
    loading.value = false;
  }
}
async function retry() {
  if (!target.value?.deliveryId || busy.value || uncertain.value)
    return;
  busy.value = true;
  try {
    await post(`/notification-deliveries/${target.value.deliveryId}/retry`, { acceptDuplicateRisk: target.value.telegramState === 'outcome_unknown' });
    open.value = false;
    pending.value = true;
    await load();
  }
  catch {
    uncertain.value = true;
    error.value = '重发回执未知，请先读取投递记录并在接收端核对，不要重复发送。';
  }
  finally {
    busy.value = false;
  }
}
onMounted(() => {
  void load();
  timer = setInterval(load, 10000);
});
onUnmounted(() => {
  disposed = true;
  if (timer)
    clearInterval(timer);
});
usePageRefresh(load);
</script>
<template>
<section class="connection-manager" aria-label="通知投递记录">
  <p>测试连接只核对账号与收件目标是否可访问，不会发送测试消息。实际送达请在投递记录和收件端核对；失败或结果未知的通知可核对后人工重发。</p>
  <header class="connection-section-heading">
    <div>
      <h2>投递记录</h2>
      <p>结果未知时先在接收端核对，避免重复通知。</p>
    </div>

  </header>
  <el-alert v-if="pending" title="重发已排队，尚未确认投递完成。" type="info" :closable="false" />
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <StateView v-if="loading" state="loading" title="正在读取投递记录…" />
  <StateView v-else-if="!items.length&&!error" state="empty" title="还没有业务通知" />
  <article v-for="item in items" :key="item.id" class="notification-entry">
    <div>
      <h3>{{ item.title }}</h3>
      <p>{{ item.body }}</p>
      <small>{{ date(item.createdAt) }}</small>
    </div>
    <div>
      <span>{{ labels[item.telegramState]||'状态待核对' }}</span>
      <p>{{ item.deliveryError }}</p>
      <el-button v-if="item.deliveryId&&['failed','outcome_unknown'].includes(item.telegramState)" :disabled="uncertain" @click="target=item;open=true">人工重发</el-button>
    </div>
  </article>
  <ConfirmAction v-model:visible="open" title="重发通知" :description="target?.telegramState==='outcome_unknown'?'请先在接收端核对上次是否已发送，并确认接受重复风险。':'将重新投递到相同版本的目标。'" impact-scope="只影响此条通知，不重复执行媒体任务。" confirm-text="确认重发" :loading="busy" @confirm="retry" />
</section>
</template>
