<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';

import { useRoute } from 'vue-router';
import { onMounted, ref, reactive, watch } from 'vue';
import { request, post } from '../api/client';
import { listNativeSessions } from '../api/mediaServer';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText } from '../workflows/readiness';
const route = useRoute();
const page = usePageNumber(), data = reactive(useResource(() => listNativeSessions((page.value - 1) * 50))), detail = ref<any>(null), open = ref(false), confirm = ref(false), op = reactive(useOperation('sessions')), notice = ref('');
const account = reactive(useResource<any>(() => request(`/media-server/users/${encodeURIComponent(String(route.query.userId))}`)));
async function accountEntry() {
  const id = route.query.userId;
  if (typeof id !== 'string' || !id || route.query.tab !== 'sessions' || op.pending)
    return;
  const user = await account.load();
  if (!user || route.query.tab !== 'sessions' || route.query.userId !== id)
    return;
  detail.value = { userId: id, userName: user.displayName || user.username, itemName: '用户登录', account: user };
  open.value = true;
}
watch(() => [route.query.tab, route.query.userId], () => {
  if (route.query.tab === 'sessions')
    void accountEntry();
  else
    open.value = false;
});
async function load() {
  await data.load();
  await accountEntry();
  if (op.pending) {
    try {
      const u: any = await request(`/media-server/users/${encodeURIComponent(op.pending.key)}`);
      if (u.activeSessions === 0) {
        op.clear();
        notice.value = '已重新读取：该用户的登录已撤销，当前播放链接可能短期仍有效。';
      }
    }
    catch (e) {
      op.error = e;
    }
  }
}
async function revoke() {
  try {
    const id = detail.value.userId;
    const user: any = await request(`/media-server/users/${encodeURIComponent(id)}`);
    await op.run(id, () => post(`/media-server/users/${encodeURIComponent(id)}/sessions/revoke`, { revision: user.revision }));
    confirm.value = false;
    open.value = false;
    await load();
  }
  catch (e) {
    op.error = e;
  }
}
onMounted(load);
defineExpose({ load, data, page });
</script>
<template>
  <section class="media-surface">
    <h2>在线会话</h2>
    <p>查看设备正在播放的内容与最近进度。</p>
    <ResourceState :stale="data.loaded" :loading="data.loading" :error="data.error" :empty="!data.data?.items.length" @reload="load"/>
    <p>当前不支持单独结束一条会话。可在详情中明确撤销该用户的全部登录。</p>
    <ResourceState :error="account.error" @reload="accountEntry"/>
    <p v-if="notice" role="status">{{ notice }}</p>
    <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
    <article v-for="session in data.data?.items||[]" :key="session.id" class="organize-record" :class="{highlighted:route.query.userId===session.userId}">
      <h3>{{ session.itemName||'—' }}</h3>
      <p>{{ session.userName||'—' }} · {{ session.deviceName||'—' }} · {{ session.state==='playing'?'播放中':session.state==='paused'?'已暂停':'状态待核对' }}</p>
      <p>最近进度 {{ date(session.lastProgressAt) }}</p>
      <el-button @click="detail=session;open=true">会话详情</el-button>
    </article>
    <PagePagination v-model:current-page="page" :page-size="50" :total="data.data.total" layout="prev,pager,next,total" @current-change="load"/>
  </section>
  <DetailDrawer v-model:visible="open" title="在线会话详情" :technical-data="detail" class="media-detail">
    <h3>{{ detail?.itemName||'—' }}</h3>
    <p>{{ detail?.userName||'—' }} · {{ detail?.deviceName||'—' }}</p>
    <p v-if="detail?.account">有效登录 {{detail.account.activeSessions??'—'}} · {{detail.account.enabled?'账号已启用':'账号已停用'}}</p>
    <p v-else>已播放 {{ detail?.positionTicks==null?'—':Math.floor(detail.positionTicks/600000000)+' 分钟' }}</p>
    <el-button :disabled="!!op.pending||op.busy||detail?.account?.activeSessions===0" @click="confirm=true">撤销该用户全部登录</el-button>
  </DetailDrawer>
  <ConfirmAction v-model:visible="confirm" title="撤销该用户全部登录" description="这不是单会话操作，将影响该用户的全部设备。" impact-scope="需要重新输入密码；已经签发的播放链接可能短期仍有效，不删除媒体。" confirm-text="确认撤销全部登录" :loading="op.busy" class="media-dialog" @confirm="revoke"/>
</template>
