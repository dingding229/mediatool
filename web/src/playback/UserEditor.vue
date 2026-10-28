<script setup lang="ts">
import { ref, reactive, watch, onUnmounted } from 'vue';
import { saveMediaUser } from '../api/mediaServer';
import { APIError, post, request } from '../api/client';
import { useOperation } from '../workflows/useOperation';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText } from '../workflows/readiness';
const props = defineProps<{
  libraries: any[];
}>(), emit = defineEmits<{
  saved: [
  ];
}>(), visible = ref(false), resetMode = ref(false), latest = ref<any>(null), original = ref<any>(null), form = reactive<any>({}), op = reactive(useOperation('user-editor')), notice = ref('');
function localTime(value: string | null) {
  if (!value)
    return '';
  const d = new Date(value);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}
function open(user: any = null, reset = false) {
  original.value = user ? JSON.parse(JSON.stringify(user)) : null;
  resetMode.value = reset;
  latest.value = null;
  op.error = null;
  notice.value = '';
  Object.assign(form, { id: user?.id || '', username: user?.username || '', displayName: user?.displayName || '', password: '', confirm: '', enabled: user?.enabled ?? true, mustChangePassword: reset ? false : user?.mustChangePassword ?? false, revision: user?.revision || 0, maxStreams: user?.maxStreams ?? 2, expiry: localTime(user?.expiresAt), grants: JSON.parse(JSON.stringify(user?.grants || [])) });
  visible.value = true;
}
function grant(id: string) {
  let row = form.grants.find((g: any) => g.libraryId === id);
  if (!row) {
    row = { libraryId: id, canPlay: true, canDownload: false };
    form.grants.push(row);
  }
  return row;
}
function selectLibrary(id: string, enabled: boolean) {
  form.grants = form.grants.filter((g: any) => g.libraryId !== id);
  if (enabled)
    form.grants.push({ libraryId: id, canPlay: true, canDownload: false });
}
async function reread() {
  const rows = await request<any[]>('/media-server/users');
  let prior: any;
  try {
    prior = op.pending ? JSON.parse(op.pending.key) : null;
  }
  catch {
  }
  const current = rows.find(u => prior?.id ? u.id === prior.id : prior?.username ? u.username === prior.username : form.id ? u.id === form.id : u.username === form.username);
  const same = current && (form.id ? current.id === form.id : current.username === form.username);
  latest.value = same ? current : null;
  if (op.pending && current && current.revision !== (prior?.revision ?? form.revision)) {
    op.clear();
    notice.value = '已重新读取当前用户，输入保留供对照。';
  }
  return same ? current : null;
}
async function save() {
  op.error = null;
  if ((!form.id || resetMode.value) && ([...form.password].length < 6 || [...form.password].length > 20 || form.password !== form.confirm)) {
    op.error = Error('密码需6–20个字符，且两次输入一致');
    return;
  }
  if (!Number.isInteger(form.maxStreams) || form.maxStreams < 1 || form.maxStreams > 20) {
    op.error = Error('同时播放上限需为1–20的整数');
    return;
  }
  let body: any;
  try {
    body = { username: form.username, displayName: form.displayName, enabled: form.enabled, mustChangePassword: form.mustChangePassword, revision: form.revision, grants: form.grants.map((g: any) => ({ ...g, canDownload: g.canPlay && g.canDownload })), maxStreams: form.maxStreams, expiresAt: form.expiry ? new Date(form.expiry).toISOString() : null, ...(!form.id ? { password: form.password } : {}) };
  }
  catch {
    op.error = Error('有效期无效');
    return;
  }
  if (original.value && !resetMode.value) {
    if (form.maxStreams === original.value.maxStreams)
      delete body.maxStreams;
    if (form.expiry === localTime(original.value.expiresAt))
      delete body.expiresAt;
  }
  const out = await op.run(JSON.stringify({ id: form.id, username: form.username, revision: form.revision }), () => resetMode.value ? post(`/media-server/users/${form.id}/password`, { revision: form.revision, password: form.password, mustChangePassword: form.mustChangePassword }) : saveMediaUser(form.id, body));
  try {
    const current = await reread();
    if (out !== undefined && current && (!form.id || current.revision !== form.revision)) {
      op.clear();
      visible.value = false;
      emit('saved');
    }
  }
  catch (e) {
    if (op.error instanceof APIError && op.error.status === 409)
      latest.value = { revision: null, username: form.username };
    op.error = e;
  }
  if (op.error instanceof APIError && op.error.status === 409 && !latest.value)
    latest.value = { revision: null, username: form.username };
  if (op.error)
    notice.value = '保存未完成或结果待核对，输入已保留。配置冲突时请对照最新记录后采用修订。';
}
watch(visible, v => {
  if (!v) {
    form.password = '';
    form.confirm = '';
  }
});
onUnmounted(() => {
  form.password = '';
  form.confirm = '';
});
defineExpose({ open, save, reread, form, op, latest });
</script>
<template>
  <el-dialog v-model="visible" :title="resetMode?'重置媒体用户密码':form.id?'编辑媒体用户':'新建媒体用户'" width="min(760px,94vw)" :close-on-click-modal="false" :show-close="!op.busy" class="media-dialog u5-dialog">
    <form class="u5-form" @submit.prevent="save">
      <template v-if="!resetMode">
        <label>
          用户名
          <input v-model="form.username" aria-label="媒体用户名" :disabled="!!form.id||op.busy||!!op.pending" autocomplete="off"/>
        </label>
        <label>
          显示名称
          <input v-model="form.displayName" aria-label="媒体用户显示名称" :disabled="op.busy||!!op.pending"/>
        </label>
        <label class="u5-check mt-form-checkbox">
          <input v-model="form.enabled" type="checkbox" :disabled="op.busy||!!op.pending"/>
          启用媒体用户
        </label>
        <label>
          同时播放上限
          <input v-model.number="form.maxStreams" type="number" min="1" max="20" aria-label="同时播放上限" :disabled="op.busy||!!op.pending"/>
        </label>
        <label>
          有效期（留空不限）
          <input v-model="form.expiry" class="mt-native-time-input" type="datetime-local" aria-label="媒体用户有效期" :disabled="op.busy||!!op.pending"/>
        </label>
        <fieldset>
          <legend>媒体库权限</legend>
          <p class="u5-permission-help">可见只允许浏览；可播放才允许观看。新授权库默认可播放、不可下载；已有权限保持原值。</p>
          <article v-for="library in libraries" :key="library.id" class="u5-grant">
            <strong>{{ library.name }}</strong>
            <label class="u5-check">
              <input type="checkbox" :aria-label="library.name+' 可见'" :checked="form.grants?.some((g:any)=>g.libraryId===library.id)" :disabled="op.busy||!!op.pending" @change="selectLibrary(library.id,($event.target as HTMLInputElement).checked)"/>
              可见 · 浏览条目
            </label>
            <template v-if="form.grants?.some((g:any)=>g.libraryId===library.id)">
              <label class="u5-check">
                <input v-model="grant(library.id).canPlay" type="checkbox" :aria-label="library.name+' 可播放'" :disabled="op.busy||!!op.pending" @change="!grant(library.id).canPlay&&(grant(library.id).canDownload=false)"/>
                可播放 · 观看视频
              </label>
              <label class="u5-check">
                <input v-model="grant(library.id).canDownload" type="checkbox" :aria-label="library.name+' 可下载'" :disabled="op.busy||!!op.pending||!grant(library.id).canPlay"/>
                可下载 · 保存原文件
              </label>
              <p v-if="!grant(library.id).canPlay" class="u5-permission-help">当前仅可浏览；播放请求将被拒绝。开启“可播放”后才能观看。</p>
            </template>
          </article>
          <p v-if="!libraries.length">暂无可分配的媒体库。</p>
        </fieldset>
      </template>
      <template v-if="!form.id||resetMode">
        <label>
          {{ resetMode?'新密码':'初始密码' }}
          <input v-model="form.password" type="password" :aria-label="resetMode?'新初始密码':'初始密码'" autocomplete="new-password" :disabled="op.busy||!!op.pending"/>
        </label>
        <label>
          确认密码
          <input v-model="form.confirm" type="password" :aria-label="resetMode?'确认新初始密码':'确认初始密码'" autocomplete="new-password" :disabled="op.busy||!!op.pending"/>
        </label>
      </template>
      <label class="u5-check mt-form-checkbox">
        <input v-model="form.mustChangePassword" type="checkbox" :disabled="op.busy||!!op.pending"/>
        要求先修改密码
      </label>
      <p class="mt-form-checkbox-help" v-if="form.mustChangePassword">勾选后，播放器暂不能登录，需要管理员完成密码修改。</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <section v-if="latest">
        <el-button v-if="latest.revision==null&&!op.phase" @click="reread().catch(e=>op.error=e)">核对账号配置</el-button>
        <p>当前用户：{{ latest.username }} · {{ latest.enabled?'启用':'停用' }} · 同时播放 {{ latest.maxStreams }}</p>
        <p v-if="!form.id">新建时遇到重复账号，请关闭后编辑已有账号。</p>

        <el-button :disabled="!form.id||latest.revision==null" @click="form.revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
      </section>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="reread"/>
    </form>
    <template #footer>
      <el-button :disabled="op.busy||!!op.pending" @click="visible=false">取消</el-button>
      <el-button :loading="op.busy" :disabled="!!op.pending||!!latest" @click="save" type="primary" class="mt-save-button">{{ resetMode?'重置密码':'保存媒体用户' }}</el-button>
    </template>
  </el-dialog>
</template>
