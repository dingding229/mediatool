<script setup lang="ts">
import { Check, View, Refresh, Download, Lock, Unlock, Delete, Close } from '@element-plus/icons-vue';
import StatusBadge from '../components/StatusBadge.vue';
import {alertPresentation} from './alerts';
import {usePageNumber,usePageSize} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import { formatBytes as bytes } from './model';
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { savePrivate } from '../organize/model';
import { date, errorText } from '../workflows/readiness';
import { stateLabel, canDeleteBackup, canPinBackup, ticketLabel, requestStep, copyOriginal } from './model';
import StateView from '../ui/StateView.vue';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import PreviewApprove from '../ui/PreviewApprove.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import {useRoute} from 'vue-router';
import WebRestorePanel from './WebRestorePanel.vue';
import PolicyPanel from './PolicyPanel.vue';
const recordPages=reactive({backups:usePageNumber('backupsPage'),drills:usePageNumber('drillsPage'),requests:usePageNumber('maintenanceRequestsPage'),alerts:usePageNumber('alertsPage')});
const recordSizes=reactive({backups:usePageSize(25,'backupsPage'),drills:usePageSize(25,'drillsPage'),requests:usePageSize(25,'maintenanceRequestsPage'),alerts:usePageSize(25,'alertsPage')});
function recordRows(kind:keyof typeof recordPages){return (data.data?.[kind]||[]).slice((recordPages[kind]-1)*recordSizes[kind],recordPages[kind]*recordSizes[kind])}
const data = reactive(useResource<any>(() => request('/ops'))), op = reactive(useOperation('maintenance')), section = ref(useRoute().query.restore?'recovery':'backups'), confirm = ref(false), action = ref<any>(null), detail = ref<any>(null), detailOpen = ref(false), restoreID = ref(''), rollbackID = ref(''), preview = ref<any>(null), previewLoading = ref(false), previewError = ref<unknown>(null), previewDialog = ref(false), approval = ref(false), copied = ref(''), commandCopy = ref('');
const alertRows=computed(()=>recordRows('alerts').map((a:any)=>({...a,display:alertPresentation(a)})));
async function load() {
  await data.load();
  if (preview.value && !data.error) {
    const current = data.data?.requests?.find((r: any) => r.id === preview.value.request.id);
    if (current)
      preview.value = { ...preview.value, request: current };
  }
  if (!op.pending || data.error || !data.data)
    return;
  const p = JSON.parse(op.pending.key), d = data.data, b = d.backups?.find((b: any) => b.id === (p.receiptId || p.id)), r = d.requests?.find((r: any) => r.id === p.id), dr = d.drills?.find((r: any) => r.id === (p.receiptId || p.id));
  if (p.kind === 'pin' && b?.pinned === p.pinned || p.kind === 'delete' && b?.state === 'deleted' || p.kind === 'backup' && b && ['ready', 'failed', 'interrupted'].includes(b.state) || p.kind === 'drill' && dr && ['verified', 'failed', 'interrupted'].includes(dr.state) || p.kind === 'ack' && d.alerts?.some((a: any) => a.code === p.id && a.acknowledged_at) || p.kind === 'cancel' && r?.state === 'cancelled' || p.kind === 'approve' && r && ['approved', 'executing', 'completed'].includes(r.state) || p.kind === 'verify' && r?.verifiedAt || p.kind === 'resume' && !d.mode.paused && d.mode.revision !== p.revision || p.kind === 'cleanup' && op.pending.status === 200)
    op.clear();
}
function ask(kind: string, row: any = {}) {
  action.value = { kind, row: JSON.parse(JSON.stringify(row)) };
  confirm.value = true;
}
const actionTitle = computed(() => ({ backup: '创建系统备份', drill: '恢复演练', pin: '更改备份固定状态', delete: '删除本程序备份', cleanup: '清理演练临时库', ack: '确认告警', cancel: '取消维护单', verify: '执行后核验', resume: '解除维护' } as Record<string, string>)[action.value?.kind] || '确认操作');
const impact = computed(() => action.value?.kind === 'backup' ? '包含数据库和托管输出，不含主密钥、115原媒体或外部服务的数据卷。' : action.value?.kind === 'drill' ? '仅创建隔离克隆库验证，不恢复到正式数据库，不启动业务执行器；仅清理本次创建的克隆库。' : action.value?.kind === 'delete' ? '只删除选中的备份包，不删除媒体；固定、最近两份成功备份及使用中的备份不能删除。' : action.value?.kind === 'cleanup' ? '只清理有本次演练创建证明的临时库；使用中或来源不明的库不会删除。' : action.value?.kind === 'verify' ? '只读核验执行结果，不会再次恢复、不会解除维护，也不代表播放器验收。' : action.value?.kind === 'resume' ? '先核对恢复、文件与授权；明确解除后才恢复业务，不通用重放旧任务。' : '仅更新此项记录；确认告警不等于问题已解决。');
async function apply() {
  const { kind, row: r } = action.value, p: any = { kind, id: r.id || r.code, revision: data.data?.mode.revision, pinned: !r.pinned };
  const out = await op.run(JSON.stringify(p), () => kind === 'backup' ? post('/ops/backups', { confirm: true }) : kind === 'drill' ? post(`/ops/backups/${r.id}/drill`, { confirm: true }) : kind === 'pin' ? post(`/ops/backups/${r.id}/pin`, { pinned: !r.pinned, confirm: true }) : kind === 'delete' ? post(`/ops/backups/${r.id}/delete`, { confirm: true }) : kind === 'cleanup' ? post(`/ops/drills/${r.id}/cleanup`, { confirm: true }) : kind === 'ack' ? post(`/ops/alerts/${encodeURIComponent(r.code)}/ack`, { confirm: true }) : kind === 'cancel' ? post(`/ops/requests/${r.id}/cancel`, { confirm: true }) : kind === 'verify' ? post(`/ops/requests/${r.id}/verify`, { confirm: true }) : post('/ops/resume', { confirm: true, text: '解除维护并恢复业务', revision: p.revision }));
  confirm.value = false;
  if (out?.id && op.pending) {
    op.pending.key = JSON.stringify({ ...p, receiptId: out.id });
    savePrivate('media-tool:u5:maintenance', op.pending);
  }
  await load();
}
async function showPreview(kind: string, id: string) {
  if (!id || op.pending)
    return;
  preview.value = null;
  previewError.value = null;
  previewDialog.value = true;
  previewLoading.value = true;
  try {
    preview.value = await post('/ops/requests/preview', { kind, id });
  }
  catch (e) {
    previewError.value = e;
    await load();
  }
  finally {
    previewLoading.value = false;
  }
}
async function approve() {
  if (!preview.value || preview.value.request.state !== 'preview' || op.pending)
    return;
  const p = preview.value;
  await op.run(JSON.stringify({ kind: 'approve', id: p.request.id }), () => post(`/ops/requests/${p.request.id}/approve`, { confirm: true, digest: p.request.digest, text: p.confirmText }));
  approval.value = false;
  await load();
}
function inspect(value: any) {
  detail.value = value;
  detailOpen.value = true;
}
useRefresh(load, {interval:30000, active:()=>!op.busy&&!confirm.value&&!approval.value});
defineExpose({ load, apply, ask, showPreview, approve, data, op, preview, section, inspect });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface ops-panel-header">
      <header class="media-section-heading">
        <h2>备份与告警</h2>
      </header>
      <ResourceState :stale="data.loaded" :error="data.error" @reload="load"/>
      <div class="f5-maint-summary">
      <p v-if="data.data&&!data.data.toolsReady">备份工具暂不可用，无法创建备份或演练；已有记录仍可查看。</p>
      <p v-if="data.data">当前备份 {{ bytes(data.data.totalBytes) }} · {{ data.data.policy?.value.backupEnabled?'下次 '+(data.data.policy?.nextBackupAt===null?'计划已结束':date(data.data.policy?.nextBackupAt)):'已停用' }}</p>
      <div v-if="data.data?.mode.paused">
        <p>业务暂停：{{ data.data.mode.reason }}</p>
        <el-button :disabled="!!op.pending" @click="ask('resume')">核对后解除维护</el-button>
      </div>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" message="维护请求尚未核验完成；请先查看当前结果，不重复执行。" @refresh="load"/>
      </div>
      <nav class="media-actions mt-segments" aria-label="备份与告警功能">
        <el-button v-for="(name,key) in {backups:'备份管理',recovery:'恢复与回滚',alerts:'运维告警',policy:'维护策略'}" :key="key" :aria-pressed="section===key" @click="section=String(key)">{{ name }}</el-button>
      </nav>
      <p v-if="section==='alerts'" class="ops-alert-warning" role="note">确认告警不等于问题已解决</p>
    </section>
    <div class="ops-content-stage" :aria-busy="data.loading||data.refreshing">
      <StateView v-if="!data.loaded&&!data.error" state="loading" title="正在读取备份与告警…"/>
      <template v-else>
    <section v-if="section==='backups'" class="media-surface ops-backups-panel">
      <header class="media-section-heading"><h3>系统备份</h3><el-button :disabled="!!op.pending||!!data.error||data.loading||!data.data?.toolsReady" @click="ask('backup')">创建系统备份</el-button></header>
      <p>保留最近两份成功备份，固定及维护单使用中的备份不参与自动清理；最多显示最近200条。</p>
      <div class="f5-backups-region">
      <ResourceState :empty="!data.loading&&!data.error&&!!data.data&&!data.data.backups?.length" empty-title="尚无系统备份"/>
      <div v-if="data.data?.backups?.length" class="mt-list-columns ops-backup-columns" aria-hidden="true"><span>备份时间 / 状态</span><span>大小 / 固定 / 说明</span><span>操作</span></div>
      <article v-for="b in recordRows('backups')" :key="b.id" class="mt-list-row ops-backup-row">
        <div class="ops-backup-name"><h3>{{ date(b.createdAt) }}</h3><p>{{ stateLabel(b.state) }}</p></div>
        <div class="ops-record-meta"><p>{{ bytes(b.bytes) }} · {{ b.pinned?'已固定':'未固定' }}</p><p>{{ b.message||'—' }}</p></div>
        <div class="media-actions mt-list-actions">
          <el-button size="small" aria-label="备份详情" @click="inspect(b)"><el-icon class="mt-list-action-icon"><View/></el-icon><span class="mt-list-action-text">备份详情</span></el-button>
          <el-button size="small" aria-label="恢复演练" v-if="b.state==='ready'" :disabled="!!op.pending||!!data.error||data.loading||!data.data?.toolsReady" @click="ask('drill',b)"><el-icon class="mt-list-action-icon"><Refresh/></el-icon><span class="mt-list-action-text">恢复演练</span></el-button>
          <a class="mt-link-button ops-maint-link" aria-label="下载备份" v-if="b.state==='ready'" :href="`/api/v1/ops/backups/${b.id}/download`"><el-icon class="mt-list-action-icon"><Download/></el-icon><span class="mt-list-action-text">下载备份</span></a>
          <el-button size="small" :aria-label="b.pinned?'取消固定':'固定备份'" v-if="canPinBackup(b.state)" :disabled="!!op.pending" @click="ask('pin',b)"><el-icon class="mt-list-action-icon"><Unlock v-if="b.pinned"/><Lock v-else/></el-icon><span class="mt-list-action-text">{{ b.pinned?'取消固定':'固定备份' }}</span></el-button>
          <el-button size="small" aria-label="删除备份" v-if="canDeleteBackup(b.state)" :disabled="!!op.pending||b.pinned" @click="ask('delete',b)"><el-icon class="mt-list-action-icon"><Delete/></el-icon><span class="mt-list-action-text">删除备份</span></el-button>
        </div>
      </article>


      </div>
<PagePagination label="系统备份分页" v-model:current-page="recordPages.backups" v-model:page-size="recordSizes.backups" :total="data.data?.backups?.length"/>
<h3 class="ops-drill-heading">恢复演练记录</h3>
      <ResourceState :empty="!data.loading&&!data.error&&!!data.data&&!data.data.drills?.length" empty-title="暂无恢复演练记录"/>
      <div v-if="data.data?.drills?.length" class="mt-list-columns ops-drill-columns" aria-hidden="true"><span>状态 / 时间</span><span>说明</span><span>操作</span></div>
      <article v-for="d in recordRows('drills')" :key="d.id" class="mt-list-row ops-drill-row">
        <div><h3>{{ stateLabel(d.state) }}</h3><p>{{ date(d.createdAt) }}</p></div>
        <p class="ops-record-meta">{{ d.message||'—' }}</p>
        <div class="media-actions mt-list-actions">
          <el-button size="small" aria-label="演练详情" @click="inspect(d)"><el-icon class="mt-list-action-icon"><View/></el-icon><span class="mt-list-action-text">演练详情</span></el-button>
          <el-button size="small" aria-label="清理临时库" v-if="['failed','interrupted'].includes(d.state)" :disabled="!!op.pending" @click="ask('cleanup',d)"><el-icon class="mt-list-action-icon"><Delete/></el-icon><span class="mt-list-action-text">清理临时库</span></el-button>
        </div>
      </article>
<PagePagination label="恢复演练分页" v-model:current-page="recordPages.drills" v-model:page-size="recordSizes.drills" :total="data.data?.drills?.length"/>

    </section>
    <section v-else-if="section==='recovery'" class="media-surface">
      <h3>受控恢复与回滚</h3>
      <WebRestorePanel :backups="data.data?.backups||[]" :backup-error="data.error" :backups-loading="data.loading"/>
      <section class="ops-cli-maintenance" aria-label="命令行维护与维护单">
      <h3>命令行维护</h3>
      <p>以下保留原命令行审批流程；网页恢复请使用上方的预览与确认。</p>
      <div class="u5-filters ops-cli-filters">
        <label>
          选择备份
          <el-select v-model="restoreID" aria-label="选择待恢复备份" placeholder="请选择">
            <el-option value="" label="请选择"/>
            <el-option v-for="b in data.data?.backups.filter((b:any)=>b.state==='ready')||[]" :key="b.id" :value="b.id" :label="date(b.createdAt)+' · '+bytes(b.bytes)"/>
          </el-select>
        </label>
        <el-button :disabled="!restoreID||!!op.pending" @click="showPreview('restore',restoreID)">预览恢复</el-button>
        <label>
          选择已登记版本
          <el-select v-model="rollbackID" aria-label="选择回滚版本" placeholder="请选择">
            <el-option value="" label="请选择"/>
            <el-option v-for="r in data.data?.releases.filter((r:any)=>!r.current&&r.verified)||[]" :key="r.revision" :value="r.revision" :label="r.revision.slice(0,12)+' · 数据结构 '+r.schema"/>
          </el-select>
        </label>
        <el-button :disabled="!rollbackID||!!op.pending" @click="showPreview('rollback',rollbackID)">预览回滚</el-button>
      </div>
      <el-button @click="inspect({releases:data.data?.releases,register:'media-toolctl --root /root/docker/mediatool register dingding229/mediatool:sha-完整SHA'})">查看登记与版本技术详情</el-button>
      <h3>维护单</h3>
      <div v-if="data.data?.requests?.length" class="mt-list-columns ops-request-columns" aria-hidden="true"><span>类型 / 状态</span><span>有效期 / 说明</span><span>操作</span></div>
      <article v-for="r in recordRows('requests')" :key="r.id" class="mt-list-row ops-request-row">
        <div><h3>{{ r.kind==='restore'?'备份恢复':'版本回滚' }}</h3><p>{{ ticketLabel(r) }}</p></div>
        <div class="ops-record-meta"><p>有效至 {{ date(r.expiresAt) }}</p><p>{{ r.message||'—' }}</p></div>
        <div class="media-actions mt-list-actions">
          <el-button size="small" aria-label="维护详情与执行命令" @click="inspect({...r,command:r.state==='approved'&&r.flow?.reason!=='approval_expired'?'media-toolctl --root /root/docker/mediatool execute '+r.id:undefined})"><el-icon class="mt-list-action-icon"><View/></el-icon><span class="mt-list-action-text">维护详情与执行命令</span></el-button>
          <el-button size="small" aria-label="取消维护单" v-if="['preview','approved'].includes(r.state)" :disabled="!!op.pending" @click="ask('cancel',r)"><el-icon class="mt-list-action-icon"><Close/></el-icon><span class="mt-list-action-text">取消维护单</span></el-button>
          <el-button size="small" aria-label="执行后核验" v-if="r.state==='completed'&&!r.verifiedAt" :disabled="!!op.pending" @click="ask('verify',r)"><el-icon class="mt-list-action-icon"><Check/></el-icon><span class="mt-list-action-text">执行后核验</span></el-button>
        </div>
      </article>
<PagePagination label="维护单分页" v-model:current-page="recordPages.requests" v-model:page-size="recordSizes.requests" :total="data.data?.requests?.length"/>

      <ResourceState :empty="!data.loading&&!data.error&&!!data.data&&!data.data.requests?.length" empty-title="暂无维护单"/>
      </section>
    </section>
    <section v-else-if="section==='alerts'" class="media-surface ops-alerts-panel">
      <header class="media-section-heading"><h3>阈值告警</h3><RouterLink to="/connections?tab=notifications">查看通知投递记录</RouterLink></header>
      <div v-if="data.data?.alerts?.length" class="mt-list-columns ops-alert-columns" aria-hidden="true"><span>告警 / 状态</span><span>当前值</span><span>连续次数 / 最近发生</span><span>操作</span></div>
      <article v-for="a in alertRows" :key="a.code" class="mt-list-row ops-alert-row">
        <div class="ops-alert-name"><h3>{{ a.display.name }}</h3><StatusBadge :state="a.display.badgeState" :label="a.display.badgeLabel"/><p v-if="a.display.description">{{ a.display.description }}</p></div>
        <div class="ops-alert-value"><p><span class="ops-mobile-label">当前 </span>{{ a.display.value }}</p><small v-if="a.display.threshold" class="ops-alert-threshold">{{ a.display.threshold }}</small></div>
        <div class="ops-alert-time"><p>连续 {{ a.hits??'—' }} 次</p><p>{{ date(a.last_seen_at) }}</p></div>
        <div class="media-actions mt-list-actions">
          <el-button size="small" aria-label="确认告警" v-if="a.state==='active'&&!a.acknowledged_at" :disabled="!!op.pending" @click="ask('ack',a)"><el-icon class="mt-list-action-icon"><Check/></el-icon><span class="mt-list-action-text">确认告警</span></el-button>
          <span v-else-if="a.acknowledged_at">已确认</span>
        </div>
      </article>
<PagePagination label="告警记录分页" v-model:current-page="recordPages.alerts" v-model:page-size="recordSizes.alerts" :total="data.data?.alerts?.length"/>

      <ResourceState :empty="!data.loading&&!data.error&&!!data.data&&!data.data.alerts?.length" empty-title="暂无告警记录"/>
    </section>
    <PolicyPanel v-else/>
      </template>
    </div>
    <ConfirmAction v-model:visible="confirm" :title="actionTitle" :description="'请核对：'+actionTitle" :impact-scope="impact" :confirm-phrase="action?.kind==='resume'?'解除维护并恢复业务':undefined" confirm-text="确认继续" :loading="op.busy" class="media-dialog" @confirm="apply"/>
    <el-dialog v-model="previewDialog" title="恢复与回滚预览" width="min(920px,94vw)" class="media-dialog u5-dialog">
      <ResourceState :loading="previewLoading" :error="previewError" @reload="load"/>
      <PreviewApprove :current-step="requestStep(preview?.request)" :steps="[{title:'选择范围'},{title:'核对预览'},{title:'审批后由服务器执行'},{title:'执行后核验'}]">
        <template v-if="preview">
          <p>仅审批，不立即执行恢复或回滚。恢复会回退全部业务表，服务器执行前会新建快照并校验备份密文。</p>
          <p>有效至 {{ date(preview.request.expiresAt) }} · {{ ticketLabel(preview.request) }}</p>
          <el-button @click="inspect(preview)">查看预览与执行命令</el-button>
          <el-button :disabled="!!op.pending||preview.request.state!=='preview'" @click="approval=true">审批维护单（不立即执行）</el-button>
        </template>
      </PreviewApprove>
    </el-dialog>
    <ConfirmAction v-model:visible="approval" title="确认维护审批" description="确认预览范围后，批准管理员在服务器执行受控恢复或回滚。" :impact-scope="preview?.request.kind==='restore'?'审批不会立即执行；执行时走app内共享恢复器，成功后恢复调度，旧任务仍需核对。':'审批不会立即执行；镜像回滚执行后必须再核验，核验通过也不自动解除维护。'" :confirm-phrase="preview?.confirmText" confirm-text="审批维护单（不立即执行）" :loading="op.busy" class="media-dialog" @confirm="approve"/>
    <DetailDrawer v-model:visible="detailOpen" title="维护技术详情" :technical-data="detail" size="large" class="media-detail">
      <label v-if="detail?.command" class="u5-field">
        服务器受控执行命令
        <textarea :value="detail.command" rows="4" readonly aria-label="宿主受控执行命令"/>
      </label>
      <el-button v-if="detail?.command" @click="copyOriginal(detail.command,s=>commandCopy=s).then(s=>copied=s)">复制执行命令</el-button>
      <p>{{ copied }}</p>
      <textarea v-if="commandCopy" :value="commandCopy" readonly aria-label="待复制执行命令"/>
    </DetailDrawer>
  </div>
</template>
