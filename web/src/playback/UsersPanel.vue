<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElDropdown, ElDropdownMenu, ElDropdownItem } from 'element-plus';
import 'element-plus/es/components/dropdown/style/css'
import 'element-plus/es/components/dropdown-menu/style/css'
import 'element-plus/es/components/dropdown-item/style/css'
import { request } from '../api/client';
import { listMediaUsers, listNativeLibraries } from '../api/mediaServer';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import UserEditor from './UserEditor.vue';
import UserActivityPanel from './UserActivityPanel.vue';
import { errorText, date } from '../workflows/readiness';
const users = reactive(useResource(listMediaUsers, [])), libraries = reactive(useResource(listNativeLibraries, [])), editor = ref<InstanceType<typeof UserEditor>>(), activity = ref<any>(null), activityOpen = ref(false), target = ref<any>(null), confirm = ref(false), op = reactive(useOperation('users'));
async function load() {
  await Promise.all([users.load(), libraries.load()]);
  if (op.pending && !users.error) {
    const [kind, id] = op.pending.key.split(':');
    if (kind === 'delete' && !users.data.some(u => u.id === id) || kind === 'sessions' && users.data.some(u => u.id === id && u.activeSessions === 0))
      op.clear();
  }
}
function create() {
  if (libraries.error || libraries.loading) {
    op.error = Error('请先重新读取媒体库权限列表');
    return;
  }
  editor.value?.open();
}
async function apply() {
  const row = target.value;
  await op.run('delete:' + row.id, () => request(`/media-server/users/${row.id}`, { method: 'DELETE', body: JSON.stringify({ revision: row.revision }) }));
  confirm.value = false;
  await load();
}
onMounted(load);
defineExpose({ load, create });
</script>
<template>
  <section class="media-surface">
    <h2>媒体用户</h2>
    <p>为家人或设备分配可看的媒体库；修改权限或密码后，需要重新登录。</p>
    <ResourceState :stale="users.loaded" :loading="users.loading" :error="users.error" :empty="!users.data.length" @reload="load"/>
    <ResourceState :error="libraries.error" @reload="libraries.load"/>
    <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
    <article v-for="user in users.data" :key="user.id" class="organize-record">
      <h3>{{ user.displayName||user.username }}</h3>
      <p>{{ user.username }} · {{ user.enabled?'启用':'停用' }} · 有效登录 {{ user.activeSessions??'—' }} · 同时播放上限 {{ user.maxStreams??'—' }}</p>
      <p>到期 {{ date(user.expiresAt||undefined) }} · 可见媒体库 {{ user.grants?.length??'—' }} 个</p>
      <div class="media-actions">
        <el-button :disabled="!!users.error||!!libraries.error" @click="editor?.open(user)">编辑用户</el-button>
        <el-button @click="activity=user;activityOpen=true">设备与历史</el-button>
        <RouterLink :to="{path:'/playback',query:{tab:'sessions',userId:user.id}}">查看会话与撤销登录</RouterLink>
        <el-dropdown trigger="click" popper-class="media-dropdown">
          <el-button>更多</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="editor?.open(user,true)">重置密码</el-dropdown-item>
              <el-dropdown-item :disabled="!!op.pending" @click="target=user;confirm=true">删除用户</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </article>
  </section>
  <UserEditor ref="editor" :libraries="libraries.data" @saved="load"/>
  <DetailDrawer v-model:visible="activityOpen" :title="(activity?.username||'媒体用户')+' · 设备与历史'" class="media-detail">
    <UserActivityPanel v-if="activity&&activityOpen" :user-id="activity.id" :user-name="activity.username"/>
  </DetailDrawer>
  <ConfirmAction v-model:visible="confirm" title="删除媒体用户" :description="`将处理 ${target?.username||'此用户'} 的账号或登录权限。`" impact-scope="删除账号、权限和登录，不删除媒体文件或其他用户。" confirm-text="确认删除用户" :loading="op.busy" class="media-dialog" @confirm="apply"/>
</template>
