<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';
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
import IpAddress from '../ui/IpAddress.vue';
import type {MediaUser} from '../api/mediaServer';
import { errorText, date } from '../workflows/readiness';
const page=usePageNumber(),users = reactive(useResource(()=>listMediaUsers(page.value), {items:[],total:0})), libraries = reactive(useResource(listNativeLibraries, [])), editor = ref<InstanceType<typeof UserEditor>>(), activity = ref<any>(null), activityOpen = ref(false), target = ref<any>(null), confirm = ref(false), op = reactive(useOperation('users'));
async function load() {
  await Promise.all([users.load(), libraries.load()]);
  if (op.pending && !users.error) {
    const [kind, id] = op.pending.key.split(':');
    if (kind === 'delete' && !users.data.items.some(u => u.id === id) || kind === 'sessions' && users.data.items.some(u => u.id === id && u.activeSessions === 0))
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
function status(user:MediaUser) {
 if(!user.enabled)return '停用';
 if(user.lockedUntil&&Date.parse(user.lockedUntil)>Date.now())return '锁定';
 if(user.expiresAt&&Date.parse(user.expiresAt)<=Date.now())return '锁定';
 return '启用';
}
function statusHint(user:MediaUser){return user.lockedUntil&&Date.parse(user.lockedUntil)>Date.now()?'登录失败次数过多，解锁时间 '+date(user.lockedUntil):user.expiresAt&&Date.parse(user.expiresAt)<=Date.now()?'账号已到期':'账号'+status(user)}
function permission(user:MediaUser){const grants=user.grants||[];return `可见 ${grants.length} 个库 · 可播放 ${grants.filter(g=>g.canPlay).length} 个 · 可下载 ${grants.filter(g=>g.canDownload).length} 个`}
onMounted(load);
defineExpose({ load, create });
</script>
<template>
  <section class="media-surface media-users-panel">
    <h2>媒体用户</h2>
    <p>为家人或设备分配可看的媒体库；修改权限或密码后，需要重新登录。</p>
    <ResourceState :stale="users.loaded" :loading="users.loading" :error="users.error" :empty="!users.data.items.length" @reload="load"/>
    <ResourceState :error="libraries.error" @reload="libraries.load"/>
    <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
    <div class="media-user-grid">
    <article v-for="user in users.data.items" :key="user.id" class="organize-record media-user-card">
      <header class="media-user-identity">
        <span class="media-user-avatar" aria-hidden="true">{{ Array.from(user.displayName||user.username)[0]?.toLocaleUpperCase()||'用' }}</span>
        <div class="media-user-name"><h3 :title="user.displayName||user.username">{{ user.displayName||user.username }}</h3><p :title="user.username">{{ user.username }}</p></div>
        <el-tag disable-transitions :type="status(user)==='启用'?'success':status(user)==='停用'?'info':'warning'" :title="statusHint(user)">{{ status(user) }}</el-tag>
      </header>
      <dl class="media-user-summary">
        <div><dt>最近播放</dt><dd>{{ user.lastPlayedAt?date(user.lastPlayedAt):'暂无播放' }}</dd></div>
        <div><dt>最近 IP 归属地</dt><dd><IpAddress v-if="user.lastIp" :ip="user.lastIp"/><span v-else>暂无公网记录</span></dd></div>
      </dl>
      <div class="media-actions media-user-actions" :aria-label="user.username+' 的操作'">
        <el-button :disabled="!!users.error||!!libraries.error" @click="editor?.open(user)">编辑用户</el-button>
        <el-button @click="activity=user;activityOpen=true">用户详情</el-button>
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
    </div>
  <PagePagination v-model:current-page="page" :total="users.data.total" :page-size="25" @current-change="users.load"/>
  </section>
  <UserEditor ref="editor" :libraries="libraries.data" @saved="load"/>
  <DetailDrawer v-model:visible="activityOpen" :title="(activity?.username||'媒体用户')+' · 用户详情'" class="media-detail">
    <dl v-if="activity" class="media-user-summary media-user-detail-summary">
      <div><dt>设备与登录</dt><dd>{{ activity.deviceCount??'—' }} 台设备 · {{ activity.activeSessions??'—' }} 个有效登录</dd></div>
      <div><dt>权限摘要</dt><dd>{{ permission(activity) }}</dd></div>
      <div><dt>同时播放上限</dt><dd>{{ activity.maxStreams??'—' }}</dd></div>
      <div><dt>年龄分级上限</dt><dd>{{ activity.maxParentalRating??'不限' }}</dd></div>
      <div><dt>到期</dt><dd>{{ date(activity.expiresAt||undefined) }}</dd></div>
      <div><dt>账号状态</dt><dd>{{ statusHint(activity) }}</dd></div>
    </dl>
    <RouterLink v-if="activity" class="el-button media-user-session-link" :to="{path:'/playback',query:{tab:'sessions',userId:activity.id}}" @click="activityOpen=false">查看会话与撤销登录</RouterLink>
    <h3>设备与历史</h3>
    <UserActivityPanel v-if="activity&&activityOpen" :user-id="activity.id" :user-name="activity.username"/>
  </DetailDrawer>
  <ConfirmAction v-model:visible="confirm" title="删除媒体用户" :description="`将处理 ${target?.username||'此用户'} 的账号或登录权限。`" impact-scope="删除账号、权限、登录及该用户的播放事件和 IP 统计；不删除媒体文件或其他用户的数据。" confirm-text="确认删除用户" :loading="op.busy" class="media-dialog" @confirm="apply"/>
</template>

<style scoped>
.media-users-panel{min-width:0;max-width:100%;box-sizing:border-box}
.media-user-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(100%,340px),1fr));gap:16px}
.media-user-card{padding:16px;min-width:0;margin:0;display:flex;flex-direction:column;gap:16px}
.media-user-identity{display:flex;gap:12px;align-items:center}
.media-user-avatar{display:grid;place-items:center;flex:0 0 40px;height:40px;border-radius:50%;background:var(--el-color-primary-light-9);color:var(--el-color-primary);font-size:1.1rem;font-weight:600}
.media-user-name{min-width:0;width:0;flex:1}
.media-user-name h3,.media-user-name p{margin:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.media-user-name p{color:var(--el-text-color-secondary);font-size:.85rem;margin-top:4px}
.media-user-summary{display:grid;gap:10px;margin:0}
.media-user-summary>div{display:grid;grid-template-columns:7.5em minmax(0,1fr);gap:8px;align-items:baseline}
.media-user-summary dt{color:var(--el-text-color-secondary);font-size:.85rem}
.media-user-summary dd{margin:0;overflow-wrap:anywhere}
.media-user-actions{padding-top:12px;border-top:1px solid var(--el-border-color-lighter);align-items:center;margin-top:auto;gap:8px}
.media-user-actions :deep(.el-button){height:44px;margin:0;padding:8px 12px;text-decoration:none}
.media-user-actions :deep(.el-dropdown){display:inline-flex}
.media-user-detail-summary{margin-bottom:16px}
.media-user-session-link{color:var(--el-button-text-color);text-decoration:none;min-height:44px}
.media-user-session-link:hover,.media-user-session-link:focus-visible{color:var(--el-button-hover-text-color)}
</style>
