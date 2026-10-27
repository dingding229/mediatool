<script setup lang="ts">
import {onMounted,onUnmounted,ref} from 'vue';
import {beijingTime as date} from '../presentation/time';
import {usePageRefresh} from '../ui/page-refresh';
import {responseStatus} from '../api/client';
import {listExternalKeys,createExternalKey,revealExternalKey,revokeExternalKey,deleteExternalKey,externalCalls,type ExternalKey} from '../api/external-credentials';
import {useResource} from '../workflows/useResource';
import {useOperation} from '../workflows/useOperation';
import ConfirmAction from '../ui/ConfirmAction.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import {stateFor,messageFor} from './revision';
const options=[['status.read','版本与状态只读'],['library.read','媒体与播放文件只读'],['layout.preview','分类与命名预览'],['tasks.read','下载与上传状态只读'],['search.run','创建站点搜索与读取结果'],['downloads.run','创建真实下载任务（写入）']];
const keys=useResource(listExternalKeys,[]),detail=ref<ExternalKey>(),calls=useResource(()=>externalCalls(detail.value?.id),[]),op=useOperation('settings-api');
const dialog=ref(false),name=ref(''),scopes=ref(['status.read']),token=ref(''),tokenOpen=ref(false),activeKey=ref<ExternalKey>(),copyNotice=ref(''),notice=ref(''),revoke=ref<ExternalKey>(),remove=ref<ExternalKey>();
let alive=true,generation=0;
async function load(){await keys.load();if(detail.value){const k=keys.data.value.find(v=>v.id===detail.value?.id);if(k)detail.value=k;await calls.load()}if(op.pending.value&&!keys.loading.value&&!keys.error.value)notice.value='已读取列表，请核对上次操作；不会自动重试创建或披露凭据。'}
usePageRefresh(load,'api');onMounted(load);
async function create(){if(!name.value.trim()||name.value.trim().length>100||!scopes.value.length||op.pending.value)return;const out=await op.run('create',()=>createExternalKey(name.value.trim(),scopes.value));if(!alive)return;if(out?.token&&responseStatus(out)===201){activeKey.value=out;token.value=out.token;tokenOpen.value=true;dialog.value=false;name.value='';op.clear()}await load()}
function hideToken(){generation++;token.value='';tokenOpen.value=false;activeKey.value=undefined;copyNotice.value=''}
async function disclose(k:ExternalKey,action:'view'|'copy'='view'){if(k.revokedAt||op.busy.value||op.pending.value)return;const current=++generation;const out=await op.run('reveal:'+k.id,()=>revealExternalKey(k.id,action));if(!alive||current!==generation)return;if(out?.token){activeKey.value=k;token.value=out.token;tokenOpen.value=true;op.clear();if(action==='copy'){try{if(!navigator.clipboard?.writeText)throw Error();await navigator.clipboard.writeText(out.token);if(alive&&current===generation)copyNotice.value='凭据已复制，请妥善保管。'}catch{if(alive&&current===generation)copyNotice.value='复制失败，可在明文字段手动选择复制；本次披露已记审计。'}}}}
async function copy(){if(activeKey.value)await disclose(activeKey.value,'copy')}
async function revokeKey(){const k=revoke.value;if(!k)return;revoke.value=undefined;await op.run('revoke:'+k.id,()=>revokeExternalKey(k.id));if(alive)await load()}
async function deleteKey(){const k=remove.value;if(!k)return;remove.value=undefined;await op.run('delete:'+k.id,()=>deleteExternalKey(k.id));if(alive)await load()}
function closeDetail(value:boolean){if(!value)detail.value=undefined}
async function showDetail(k:ExternalKey){detail.value=k;await calls.load()}
function acknowledge(){if(!op.busy.value&&!keys.error.value&&!keys.loading.value){op.clear();notice.value='已结束核对，可重新操作；每次查看或复制都会记审计。'}}
onUnmounted(()=>{alive=false;hideToken()});
</script>
<template>
 <section class="settings-card external-keys" aria-label="外部 API 凭据">
  <header><div><h2>外部 API 凭据</h2><p>独立 JWT 授权，不设有效期；撤销后新请求立即拒绝。每次查看或复制均记审计。</p></div><el-button :disabled="!!op.pending.value" @click="dialog=true">创建 API 凭据</el-button></header>
  <AsyncResultBanner v-if="op.phase.value" :status="op.phase.value" @refresh="load" :message="op.phase.value==='queued'?'操作已排队，请核对列表。':'操作结果未确认，请先核对列表，不会自动重试。'"/>
  <StateView v-if="op.error.value" :state="stateFor(op.error.value)" :description="messageFor(op.error.value)" retry-text="核对列表" @retry="load"/>
  <p v-if="notice" role="status">{{notice}}</p><el-button v-if="op.pending.value" :disabled="op.busy.value||!!keys.error.value||keys.loading.value" @click="acknowledge">我已核对列表，结束本次操作</el-button>
  <StateView compact v-if="keys.loading.value&&!keys.loaded.value" state="loading"/>
  <StateView compact v-else-if="keys.error.value" :state="stateFor(keys.error.value)" :description="messageFor(keys.error.value)" @retry="load"/>
  <StateView compact v-else-if="!keys.data.value.length" state="empty" title="尚无外部 API 凭据" description="按最小权限创建，可重复查看和复制，均记审计。"/>
  <div v-else class="external-key-list">
   <article v-for="key in keys.data.value" :key="key.id" class="external-key-row">
    <strong class="external-key-name" :title="key.name">{{key.name}}</strong><el-tag :type="key.revokedAt?'info':'success'">{{key.revokedAt?'已撤销':'有效'}}</el-tag>
    <details class="external-key-scopes"><summary>{{key.scopes.length}} 项权限</summary><ul><li v-for="scope in key.scopes" :key="scope">{{options.find(v=>v[0]===scope)?.[1]||scope}}</li></ul></details>
    <div class="external-last"><span>{{key.lastIP||'暂无调用 IP'}}</span><time>{{date(key.lastUsedAt)||'尚未调用'}}</time></div>
    <div class="external-key-actions"><el-button size="small" :disabled="!!key.revokedAt||op.busy.value||!!op.pending.value" @click="disclose(key)">查看</el-button><el-button size="small" :disabled="!!key.revokedAt||op.busy.value||!!op.pending.value" @click="disclose(key,'copy')">复制</el-button><el-button size="small" @click="showDetail(key)">详情</el-button><el-button v-if="!key.revokedAt" size="small" :disabled="!!op.pending.value" @click="revoke=key">撤销</el-button><el-button v-else size="small" type="danger" plain :disabled="!!op.pending.value" @click="remove=key">删除</el-button></div>
   </article>
  </div>
  <details class="external-help"><summary>接入说明</summary><p>通过 HTTPS 使用 Authorization: Bearer 凭据；每个凭据每分钟最多120次。不能用地址参数、后台登录或 STRM key 替代。主密钥轮换会使旧凭据失效，需重建并更新调用方。</p><p>正式状态查询等外部业务接口保持不变；调用 IP 异步记录，通常数秒内可见，最多保留30天且每凭据最多1000条。</p></details>
  <el-dialog v-model="dialog" title="创建独立 API 凭据" width="min(560px,94vw)" class="settings-dialog" :close-on-click-modal="false"><p v-if="op.error.value" role="alert">{{messageFor(op.error.value)}}</p><p v-if="op.pending.value" role="status">创建结果尚未确认，请先核对列表，不会重复创建。</p><el-button v-if="op.pending.value" @click="dialog=false;load()">关闭并核对凭据列表</el-button><label for="external-key-name">凭据名称</label><el-input id="external-key-name" v-model="name" maxlength="100"/><div class="external-scope-options"><el-checkbox-group v-model="scopes"><el-checkbox v-for="option in options" :key="option[0]" :value="option[0]">{{option[1]}}</el-checkbox></el-checkbox-group></div><p>下载权限会产生真实任务，请仅选择所需权限。</p><template #footer><el-button @click="dialog=false">取消</el-button><el-button type="primary" :disabled="!name.trim()||!scopes.length||!!op.pending.value" :loading="op.busy.value" @click="create">确认创建</el-button></template></el-dialog>
  <el-dialog destroy-on-close v-model="tokenOpen" title="API 凭据明文" width="min(680px,94vw)" class="settings-dialog" :close-on-click-modal="false" @closed="hideToken"><p>本次披露已记审计。请勿把凭据放入 URL、日志或公开文件。关闭后清空当前页面明文，需要时可再次查看。</p><el-input :model-value="token" type="textarea" :rows="5" readonly aria-label="完整API凭据"/><p v-if="copyNotice" role="status">{{copyNotice}}</p><template #footer><el-button :loading="op.busy.value" :disabled="op.busy.value||!!op.pending.value" @click="copy">复制凭据</el-button><el-button @click="hideToken">关闭凭据</el-button></template></el-dialog>
  <DetailDrawer :visible="!!detail" title="凭据详情" style="overflow-wrap:anywhere" size="large" class="settings-dialog" @update:visible="closeDetail"><template v-if="detail"><h3>{{detail.name}}</h3><p>凭据 ID：{{detail.id}}</p><p>创建：{{date(detail.createdAt)}} · {{detail.revokedAt?'已撤销':'有效'}}</p><h3>全部权限</h3><ul><li v-for="scope in detail.scopes" :key="scope">{{options.find(v=>v[0]===scope)?.[1]||scope}}</li></ul><h3>最近调用</h3><p>最近50条；IP异步写入，最多保留30天、每凭据1000条。</p><StateView v-if="calls.loading.value" state="loading"/><StateView v-else-if="calls.error.value" :state="stateFor(calls.error.value)" :description="messageFor(calls.error.value)" @retry="calls.load"/><StateView v-else-if="!calls.data.value.length" state="empty" title="暂无调用记录"/><ol v-else class="external-calls"><li v-for="(call,i) in calls.data.value" :key="i"><strong>{{call.clientIP||'未知 IP'}} · {{call.status||'处理中'}}</strong><code>{{call.method}} {{call.path}}</code><time>{{date(call.createdAt)}}</time><small>{{call.ipSource==='trusted_proxy'?'可信代理':call.ipSource==='direct'?'直连来源':'来源未确认'}}</small></li></ol></template><template #footer><el-button @click="calls.load">刷新记录</el-button><el-button @click="detail=undefined">关闭详情</el-button></template></DetailDrawer>
  <ConfirmAction class="settings-dialog" confirm-text="确认撤销" :visible="!!revoke" title="撤销 API 凭据" description="撤销提交后新请求立即拒绝，不能再次查看或复制；不取消已获准的在途任务。" @cancel="revoke=undefined" @confirm="revokeKey"/>
  <ConfirmAction class="settings-dialog" danger-level="danger" confirm-text="确认删除" :visible="!!remove" title="删除已撤销凭据" description="只删除本条已撤销凭据及关联调用记录，安全审计保留。" @cancel="remove=undefined" @confirm="deleteKey"/>
 </section>
</template>
<style scoped>
.external-keys{min-width:0}.external-key-list{display:grid;gap:0}.external-key-row{display:grid;grid-template-columns:minmax(100px,1fr) 62px 90px minmax(160px,1fr) auto;align-items:center;gap:12px;padding:12px 0;border-bottom:1px solid var(--mt-border-color);min-width:0}.external-key-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0}.external-key-actions{display:flex;justify-content:flex-end;gap:6px;flex-wrap:wrap}.external-key-actions .el-button+.el-button{margin-left:0}.external-key-scopes{min-width:0}.external-key-scopes summary{white-space:nowrap}.external-key-scopes ul{padding-left:18px;line-height:1.8}.external-last{display:grid;gap:4px;min-width:0;font-size:13px;overflow-wrap:anywhere}.external-last time{color:var(--mt-text-muted);font-size:12px}.external-help{margin-top:20px}.external-calls{list-style:none;padding:0;display:grid;gap:12px}.external-calls li{display:grid;gap:6px;overflow-wrap:anywhere;padding-bottom:12px;border-bottom:1px solid var(--mt-border-color)}.external-calls code{white-space:normal}.external-scope-options{margin:16px 0}.external-scope-options .el-checkbox-group{display:flex;gap:8px 16px;flex-wrap:wrap}@media(max-width:1100px){.external-key-row{grid-template-columns:minmax(100px,1fr) 62px 90px}.external-last{grid-column:1/3}.external-key-actions{grid-column:1/-1;justify-content:flex-start}}@media(max-width:500px){.external-key-row{gap:8px}.external-key-name{grid-column:1/3}.external-key-row>.el-tag{grid-column:3}.external-key-scopes{grid-column:1}.external-last{grid-column:2/4}.external-key-actions{grid-column:1/-1}}
</style>
