<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';
import {beijingTime as date} from '../presentation/time';
import {usePageRefresh} from '../ui/page-refresh';
import { onMounted, onUnmounted, ref } from 'vue';
import { post, request, responseStatus, testExternalToken } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ConfirmAction from '../ui/ConfirmAction.vue';
import StateView from '../ui/StateView.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { stateFor, messageFor } from './revision';
const keysPage=usePageNumber('apiKeysPage'),callsPage=usePageNumber('apiCallsPage');
interface Key {
  id: string;
  name: string;
  scopes: string[];
  createdAt?: string;
  lastUsedAt?: string;
  revokedAt?: string;
  prefix?: string;
}
const publicKey = ({ id, name, scopes, createdAt, lastUsedAt, revokedAt, prefix }: Key): Key => ({ id, name, scopes, createdAt, lastUsedAt, revokedAt, prefix });
const keys = useResource(async () => (await request<Key[]>('/external-api/keys')).map(publicKey), []), calls = useResource(() => request<any[]>('/external-api/calls'), []), op = useOperation('settings-api'), dialog = ref(false), name = ref(''), scopes = ref(['status.read', 'library.read', 'layout.preview', 'tasks.read']), token = ref(''), tokenOpen = ref(false), testToken = ref(''), verification = ref(''), verifying = ref(false), copyNotice = ref(''), revoke = ref<Key>(), notice = ref('');
const options = [['status.read', '版本与状态只读'], ['library.read', '媒体与播放文件只读'], ['layout.preview', '分类与命名预览'], ['tasks.read', '下载与上传状态只读'], ['search.run', '创建站点搜索与读取结果'], ['downloads.run', '创建真实下载任务（写入）']];
let alive = true;
async function load() {
  await Promise.all([keys.load(), calls.load()]);
  if (op.pending.value && !keys.error.value && keys.data.value) {
    const key = op.pending.value.key;
    if (key.startsWith('revoke:') && keys.data.value.find(x => x.id === key.slice(7))?.revokedAt) {
      op.clear();
      notice.value = '已核对：凭据已撤销。';
    }
    else
      notice.value = '已读取凭据列表，请核对上次操作。未收到的明文不会补显；不会重复创建。';
  }
}
async function create() {
  if (!name.value.trim() || !scopes.value.length || op.pending.value)
    return;
  const out = await op.run('create', () => post<Key & {
    token: string;
  }>('/external-api/keys', { name: name.value.trim(), scopes: scopes.value, confirm: true }));
  if (!alive)
    return;
  if (out?.token && responseStatus(out) === 201) {
    token.value = out.token;
    tokenOpen.value = true;
    dialog.value = false;
    name.value = '';
    op.clear();
  }
  await load();
}
async function revokeKey() {
  const key = revoke.value;
  if (!key)
    return;
  revoke.value = undefined;
  await op.run('revoke:' + key.id, () => post(`/external-api/keys/${key.id}/revoke`, { confirm: true }));
  if (alive)
    await load();
}
function hideToken() {
  token.value = '';
  tokenOpen.value = false;
  copyNotice.value = '';
}
async function copy() {
  const original = token.value;
  try {
    if (!navigator.clipboard?.writeText)
      throw Error();
    await navigator.clipboard.writeText(original);
    if (alive && token.value === original)
      copyNotice.value = '凭据已复制，请妥善保管。';
  }
  catch {
    if (alive && token.value === original)
      copyNotice.value = '复制失败，请在下方字段手动选择并复制。';
  }
}
async function verify() {
  if (!testToken.value || verifying.value)
    return;
  verifying.value = true;
  verification.value = '';
  const input = testToken.value;
  try {
    await testExternalToken(input);
    if (alive)
      verification.value = '验证通过：此凭据可以读取状态。';
  }
  catch (e) {
    if (alive)
      verification.value = messageFor(e);
  }
  finally {
    if (alive) {
      verifying.value = false;
      testToken.value = '';
      void calls.load();
    }
  }
}
function acknowledge() {
  if (op.busy.value || keys.error.value || keys.loading.value)
    return;
  op.clear();
  notice.value = '已结束本次核对。再次创建需要重新确认；若发现重复凭据，请先撤销。';
}
onMounted(load);
onUnmounted(() => {
  alive = false;
  hideToken();
  testToken.value = '';
});
usePageRefresh(load, 'api');
</script>
<template>
  <div class="settings-stack">
    <section class="settings-card" aria-label="外部 API 凭据">
      <header>
        <div>
          <h2>外部 API 凭据</h2>
          <p>独立授权，可随时撤销。默认只读；下载权限会产生真实任务。</p>
        </div>
        <div class="settings-actions">

          <el-button :disabled="!!op.pending.value" @click="dialog=true">创建 API 凭据</el-button>
        </div>
      </header>
      <AsyncResultBanner v-if="op.phase.value" :status="op.phase.value" @refresh="load" :message="op.phase.value==='queued'?'请求已排队，请核对凭据列表。':'上次操作结果未确认，请先核对列表，不会重复创建或撤销。'"/>
      <StateView v-if="op.error.value" :state="stateFor(op.error.value)" :description="messageFor(op.error.value)" retry-text="核对凭据列表" @retry="load"/>
      <p v-if="notice" role="status">{{notice}}</p>
      <el-button v-if="op.pending.value" :disabled="op.busy.value||!!keys.error.value||keys.loading.value||!keys.data.value" @click="acknowledge">我已核对列表，结束本次操作</el-button>
<div class="f5-key-rows">
      <StateView v-if="keys.loading.value" state="loading"/>
      <StateView v-else-if="keys.error.value" :state="stateFor(keys.error.value)" :description="messageFor(keys.error.value)" @retry="keys.load"/>
      <StateView v-else-if="!keys.data.value?.length" state="empty" title="尚未创建外部 API 凭据" description="创建后只显示一次明文，请保存在安全的位置。"/>
      <article v-for="key in keys.data.value?.slice((keysPage-1)*10,keysPage*10)" :key="key.id" class="settings-key">
        <header>
          <div>
            <h3>{{key.name}}</h3>
            <p>{{key.revokedAt?'已撤销':'有效，直到主动撤销'}} · 最近使用 {{date(key.lastUsedAt)}}</p>
          </div>
          <el-button :disabled="!!key.revokedAt||!!op.pending.value" @click="revoke=key">撤销</el-button>
        </header>
        <p>权限：{{key.scopes.map(s=>options.find(o=>o[0]===s)?.[1]||s).join('、')}}</p>
        <small>创建时间 {{date(key.createdAt)}} · 已保存，明文不再显示</small>
      </article>
<PagePagination label="API 凭据分页" v-model:current-page="keysPage" :page-size="10" :total="keys.loaded.value?keys.data.value?.length:undefined"/>

</div>
      <details>
        <summary>接入说明</summary>
        <p>通过 HTTPS 发送 Authorization: Bearer 凭据；每个凭据每分钟最多 120 次。不能用地址参数或后台登录状态代替。</p>
        <pre>GET /api/external/v1/status\nGET /api/external/v1/strm\nGET /api/external/v1/strm/{assetId}/content\nPOST /api/external/v1/layout/preview\nGET /api/external/v1/downloads\nGET /api/external/v1/transfers\nPOST /api/external/v1/search\nGET /api/external/v1/search/{id}\nPOST /api/external/v1/downloads</pre>
        <p>不开放管理员密码、原始数据库或任意文件读写。</p>
      </details>
    </section>
    <section class="settings-card" aria-label="验证 API 凭据">
      <h2>验证凭据</h2>
      <p>输入你已保管的凭据，仅向本站发送一次状态查询，不写入存储。需要状态只读权限。</p>
      <form @submit.prevent="verify">
        <label for="verify-api-key">待验证凭据</label>
        <el-input id="verify-api-key" v-model="testToken" :disabled="verifying" type="password" show-password autocomplete="off"/>
        <footer>
          <el-button native-type="submit" :disabled="!testToken" :loading="verifying">验证状态访问</el-button>
          <p role="status">{{verification}}</p>
        </footer>
      </form>
    </section>
    <section class="settings-card" aria-label="最近 API 调用">
      <h2>最近调用</h2>
      <StateView v-if="calls.error.value" :state="stateFor(calls.error.value)" :description="messageFor(calls.error.value)" @retry="calls.load"/>
      <StateView v-else-if="calls.loading.value" state="loading"/>
      <StateView v-else-if="!calls.data.value?.length" state="empty" title="暂无外部 API 调用"/>
      <ol class="settings-calls">
        <li v-for="(call,i) in calls.data.value?.slice((callsPage-1)*25,callsPage*25)" :key="i">
          <strong>{{call.name||'—'}} · {{call.status??'—'}}</strong>
          <code>{{call.method}} {{call.path}}</code>
          <time>{{date(call.createdAt)}}</time>
        </li>
      </ol>
<PagePagination label="API 调用记录分页" v-model:current-page="callsPage" :page-size="25" :total="calls.loaded.value?calls.data.value?.length:undefined"/>

    </section>
    <el-dialog class="settings-dialog" v-model="dialog" title="创建独立 API 凭据" width="min(560px,94vw)">
      <form class="settings-stack" @submit.prevent="create">
        <p v-if="op.error.value" role="alert">{{messageFor(op.error.value)}}</p>
        <p v-if="op.pending.value" role="status">{{op.phase.value==='queued'?'创建请求已排队，请先核对列表。':'创建结果尚未确认，请先核对列表，不会重复创建。'}}</p>
        <el-button v-if="op.pending.value" @click="dialog=false;load()">关闭并核对凭据列表</el-button>
        <label for="external-key-name">凭据名称</label>
        <el-input id="external-key-name" v-model="name" :disabled="op.busy.value" maxlength="100"/>
        <div class="settings-scopes" role="group" aria-label="凭据权限">
          <label v-for="[value,label] in options" :key="value" class="settings-choice">
            <input v-model="scopes" :disabled="op.busy.value" type="checkbox" :value="value"/>
            <span>{{label}}</span>
          </label>
        </div>
        <p>凭据永久有效，直到撤销。搜索和下载权限可产生业务操作，请只授予确有需要的权限。</p>
        <p>创建后只显示一次完整凭据，请立即保存。</p>
        <el-button native-type="submit" type="primary" :loading="op.busy.value" :disabled="!name.trim()||!scopes.length||!!op.pending.value">确认创建</el-button>
      </form>
    </el-dialog>
    <el-dialog class="settings-dialog" :model-value="tokenOpen" destroy-on-close title="请立即保存新凭据" width="min(600px,94vw)" :close-on-click-modal="false" @update:model-value="hideToken">
      <p>完整凭据只在创建时显示一次。关闭或离开此页后无法再次查看，请先妥善保存。</p>
      <label for="created-api-key">新凭据（仅此一次）</label>
      <el-input id="created-api-key" :model-value="token" readonly type="textarea" :rows="3"/>
      <p role="status">{{copyNotice}}</p>
      <template #footer>
        <el-button @click="copy">复制新凭据</el-button>
        <el-button type="primary" @click="hideToken">已保存，关闭</el-button>
      </template>
    </el-dialog>
    <ConfirmAction class="settings-dialog" :visible="!!revoke" title="撤销 API 凭据" :description="`立即停止“${revoke?.name||''}”的后续调用；不删除媒体或任务。`" confirm-text="确认撤销" @update:visible="v=>{if(!v)revoke=undefined}" @confirm="revokeKey"/>
  </div>
</template>
