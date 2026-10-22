<script setup lang="ts">
import { useRefresh } from '../composables/useRefresh';
import {pendingRestore,rememberRestore,clearRestore} from '../workflows/restore-session';
import {onMounted,onUnmounted,ref} from 'vue';
import {request,post} from '../api/client';
import {usePageRefresh} from '../ui/page-refresh';
import {date,errorText} from '../workflows/readiness';
import {formatBytes} from './model';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
const props=defineProps<{backups:any[];backupError?:unknown;backupsLoading?:boolean}>();
const records=ref<any[]>([]),loading=ref(true),error=ref<unknown>(),preview=ref<any>(),previewOpen=ref(false),previewLoading=ref(false),previewError=ref<unknown>(),writeError=ref<unknown>(),confirm=ref(false),busy=ref(false),notice=ref('');
let alive=true,epoch=0;
async function load(){const version=++epoch;loading.value=epoch===1;error.value=undefined;try{const out=await request<any[]>('/ops/restores');if(alive&&version===epoch){records.value=out;const receipt=out.find(r=>r.id===pendingRestore());if(receipt&&['completed','rolled_back','failed'].includes(receipt.state)){notice.value='已读取恢复结果：'+label(receipt.state)+'；'+receipt.message;clearRestore(receipt.id)}}}catch(e){if(alive&&version===epoch)error.value=e}finally{if(alive&&version===epoch)loading.value=false}}
async function inspect(id:string){if(busy.value)return;preview.value=undefined;writeError.value=undefined;previewOpen.value=true;previewLoading.value=true;previewError.value=undefined;try{const out=await post('/ops/restores/preview',{backupId:id});if(alive){preview.value=out;records.value=[(out as any).restore,...records.value.filter(r=>r.id!==(out as any).restore.id)]}}catch(e){if(alive)previewError.value=e}finally{if(alive)previewLoading.value=false}}
async function execute(){const p=preview.value;if(!p||busy.value)return;busy.value=true;notice.value='';rememberRestore(p.restore.id);try{const out=await post<any>(`/ops/restores/${p.restore.id}/execute`,{digest:p.restore.digest,text:p.confirmText});if(alive){notice.value=['pending','running'].includes(out.state)?'已排队，尚未恢复。请查看执行记录；成功后需要重新登录。':'已核对原请求：'+label(out.state)+'；没有重复执行。';previewOpen.value=false;confirm.value=false;records.value=[out,...records.value.filter(r=>r.id!==out.id)]}}catch(e){if(alive){notice.value=errorText(e);writeError.value=e;confirm.value=false}}finally{if(alive){busy.value=false;await load()}}}
async function reconcile(id:string){if(busy.value)return;busy.value=true;try{await post(`/ops/restores/${id}/reconcile`,{confirm:true});if(alive)notice.value='已核对原恢复记录，没有再次执行恢复。'}catch(e){if(alive)notice.value=errorText(e)}finally{if(alive){busy.value=false;await load()}}}
const label=(state:string)=>({preview:'仅预览',pending:'已排队',running:'恢复执行中',unknown:'结果未知，需核对',completed:'恢复成功',rolled_back:'失败后已自动回滚',failed:'未执行恢复'}[state]||state);
useRefresh(load, {interval: 15000, active:()=>!busy.value});
onUnmounted(()=>{alive=false;++epoch});
usePageRefresh(()=>load());
</script>
<template>
<section aria-label="网页备份恢复" class="media-surface">
 <h2>网页备份恢复</h2>
 <p>先预览校验，再输入备份确认短语。确认需10分钟内的新近登录；执行前自动保留恢复前快照。</p>
 <p>账号、115授权、连接、设置、媒体记录、插件配置及KV全部随备份回退；不覆盖现存播放文件。数据库恢复后旧任务暂停，需逐项核对。</p>
 <p v-if="notice" role="status">{{notice}}</p>
 <ResourceState :loading="props.backupsLoading" :error="props.backupError"/>
 <article v-for="b in (props.backupError?[]:props.backups).filter(b=>b.state==='ready')" :key="b.id" class="organize-record">
  <h3>{{b.id}}</h3><p>来源 {{b.origin}} · {{date(b.createdAt)}} · 数据结构 {{b.manifest?.schema??'—'}} · {{formatBytes(b.bytes)}}</p>
  <p>SHA256 {{b.digest}} · 完整性：{{records.some(r=>r.backupId===b.id&&r.digest===b.digest)?'上次预览校验通过；再次执行前会重检':'尚未校验，请先预览'}} · 包含数据库、清单和登记的受管输出</p>
  <el-button :disabled="busy||previewLoading||!!props.backupError||props.backupsLoading" @click="inspect(b.id)">{{b.origin==='pre-restore'?'预览回滚到恢复前':'预览网页恢复'}}</el-button>
 </article>
 <ResourceState :empty="!props.backupsLoading&&!props.backupError&&!props.backups.some(b=>b.state==='ready')" empty-title="暂无可恢复备份"/>
 <h3>网页恢复执行记录</h3>
 <ResourceState :loading="loading" :error="error" :empty="!loading&&!error&&!records.length" empty-title="暂无网页恢复记录" @reload="load"/>
 <template v-if="!error"><article v-for="r in records" :key="r.id" class="organize-record">
  <h3>{{label(r.state)}}</h3><p>备份 {{r.backupId}} · {{date(r.createdAt)}}</p><p>{{r.message||'—'}}</p>
  <p v-if="r.snapshotId">恢复前快照 {{r.snapshotId}}</p>
  <el-button v-if="r.state==='unknown'" :disabled="busy" @click="reconcile(r.id)">核对中断结果</el-button>
  <el-button v-if="r.snapshotId&&['completed','rolled_back'].includes(r.state)" :disabled="busy||previewLoading||!!props.backupError||props.backupsLoading" @click="inspect(r.snapshotId)">预览回滚到恢复前</el-button>
 </article></template>
 <DetailDrawer v-model:visible="previewOpen" title="网页恢复预览">
  <ResourceState :loading="previewLoading" :error="previewError"/>
  <p v-if="writeError" role="alert">{{errorText(writeError)}}</p>
  <p v-if="writeError">先核对执行记录；需要重新登录时，登录后重新预览。</p>
  <template v-if="preview&&!previewLoading&&!previewError"><p>{{preview.message}}</p><h3>高风险提示</h3><ul><li v-for="s in preview.warnings" :key="s">{{s}}</li></ul>
<h3>将回退的业务表</h3><p>行数相同不代表内容相同；列出的表仍整体回退。旧schema备份升级时可能初始化新增表。</p><div class="restore-table-scroll"><table aria-label="恢复业务表行数对比"><thead><tr><th>表名</th><th>当前行数</th><th>备份行数</th><th>行数变化</th></tr></thead><tbody><tr v-for="t in preview.restore.tableChanges" :key="t.name"><td>{{t.name}}</td><td>{{t.currentMissing?'不存在':t.currentRows}}</td><td>{{t.backupRows}}</td><td>{{t.backupRows-t.currentRows}}</td></tr></tbody></table></div>
<h3>恢复包含范围</h3><ul><li v-for="s in preview.includes" :key="s">{{s}}</li></ul><h3>不会覆盖的范围</h3><ul><li v-for="s in preview.excludes" :key="s">{{s}}</li></ul>
   <p>数据结构 {{preview.restore.backupSchema??'—'}} → {{preview.restore.programSchema??'—'}}；只允许向上迁移。</p>
   <p>备份 {{preview.restore.backupId}} · 有效至 {{date(preview.restore.expiresAt)}}</p>
   <p>校验结果：SHA256、密文解密与数据结构升级校验通过。</p>
   <el-button type="danger" :disabled="busy||!!writeError" @click="confirm=true">确认并排队恢复</el-button>
  </template>
 </DetailDrawer>
 <ConfirmAction v-model:visible="confirm" title="确认网页恢复" danger-level="danger" :confirm-phrase="preview?.confirmText" :description="'请输入：'+(preview?.confirmText||'')" impact-scope="全部业务数据回到备份状态；先创建快照，失败自动回滚。管理员密码与115令牌也回退，当前会话失效后请使用备份账号重新登录查看结果。" confirm-text="排队恢复" :loading="busy" @confirm="execute"/>
</section>
</template>

<style scoped>h3,p{overflow-wrap:anywhere}.restore-table-scroll{overflow:auto;max-height:360px}table{width:100%;border-collapse:collapse;text-align:left}th,td{padding:8px;border-bottom:1px solid var(--mt-border-color);overflow-wrap:anywhere}</style>
