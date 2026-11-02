<script setup lang="ts">
import NotificationEntry from './NotificationEntry.vue';
import type {NotificationMessage as Message} from '../api/notifications';
import {usePageNumber,usePageSize} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import {usePageRefresh} from '../ui/page-refresh';
import { onUnmounted, ref } from 'vue';
import { post, request,checkedPage,type ListPage } from '../api/client';
import ConfirmAction from '../ui/ConfirmAction.vue';
import StateView from '../ui/StateView.vue';

const page=usePageNumber(),pageSize=usePageSize(25),total=ref(0);
const items = ref<Message[]>([]), loading = ref(true), error = ref(''), pending = ref(false), target = ref<Message | null>(null), open = ref(false), busy = ref(false), uncertain = ref(false);

let disposed = false, ready = false;
async function load() {
  loading.value = !ready;
  try {
    const out = await request<ListPage<Message>>(`/notifications?page=${page.value}&pageSize=${pageSize.value}`).then(checkedPage<Message>);
    if (!disposed) {
      items.value = out.items;total.value=out.total;
      error.value = '';
      uncertain.value = false;
    }
  }
  catch {
    if(!disposed) error.value = '投递记录读取失败，不能视为没有通知。';
  }
  finally {
    if(!disposed){loading.value = false; ready = true;}
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
useRefresh(load, {interval:30000});
onUnmounted(() => {
  disposed = true;
});
usePageRefresh(load);
</script>
<template>
<section class="connection-manager" aria-label="通知投递记录">
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
  <NotificationEntry v-for="item in items" :key="item.id" :item="item" :blocked="uncertain" @retry="target=item;open=true"/>
  <ConfirmAction v-model:visible="open" title="重发通知" :description="target?.telegramState==='outcome_unknown'?'请先在接收端核对上次是否已发送，并确认接受重复风险。':'将重新投递到相同版本的目标。'" impact-scope="只影响此条通知，不重复执行媒体任务。" confirm-text="确认重发" :loading="busy" @confirm="retry" />
<PagePagination v-model:current-page="page" :total="total" v-model:page-size="pageSize" @current-change="load"/>
  </section>
</template>
