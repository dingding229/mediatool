<script setup lang="ts">
import {joinMetadata} from '../presentation/metadata';

import {downloadDetailGroups} from '../acquisition/download-details';
import DescriptionGrid from '../ui/DescriptionGrid.vue';
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
import StatusBadge from '../components/StatusBadge.vue';
import DownloadRow from '../acquisition/DownloadRow.vue';
import {request} from '../api/client';
import {remainingTime,uploadPercent,uploadInProgress} from '../acquisition/download-presentation';
import '../acquisition/download-presentation.css';
import TaskControl from '../workflows/TaskControl.vue';
import {usePageRefresh} from '../ui/page-refresh';
import { computed, ref, onMounted, onUnmounted } from 'vue';
import { ElDropdown, ElDropdownMenu, ElDropdownItem } from 'element-plus';
import 'element-plus/es/components/dropdown/style/css'
import 'element-plus/es/components/dropdown-menu/style/css'
import 'element-plus/es/components/dropdown-item/style/css'
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import StateView from '../ui/StateView.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import OutputRecoveryDialog from '../workflows/OutputRecoveryDialog.vue';
import SourceRecoveryDialog from '../acquisition/SourceRecoveryDialog.vue';
import ManualWorkspace from '../acquisition/ManualWorkspace.vue';
import { useDownloads } from '../acquisition/useDownloads';
import { label, size } from '../acquisition/model';
import { errorText, stateFor } from '../workflows/readiness';
import '../acquisition/acquisition.css';
const route = useRoute(), tabs = [{ id: 'tasks', label: '下载任务' }, { id: 'uploads', label: '上传入库' }, { id: 'manual', label: '手动添加' }], tab = computed(() => tabs.some(t => t.id === route.query.tab) ? String(route.query.tab) : 'tasks');
const {sizes,pages,totals, tasks, transfers, batches, loading, errors, notice, message, busy, confirmOpen, confirmation, sessions, sessionBusy, sessionError, binding, sessionId, refresh, load, locked, ask, execute, inspectTransfer, reconcileCleanup, bind, askBinding } = useDownloads();
const output = ref<InstanceType<typeof OutputRecoveryDialog>>(), source = ref<InstanceType<typeof SourceRecoveryDialog>>(), detail = ref<any>(null), detailsOpen = ref(false), cleanup = computed(() => confirmation.value?.action === 'cleanup');
const manualView=ref<InstanceType<typeof ManualWorkspace>>(),syncing=ref(false);
async function syncDownloads(){if(syncing.value)return;syncing.value=true;try{await refresh(true);await manualView.value?.load()}finally{syncing.value=false}}
const connections=ref<{kind:string;name:string;settings?:{endpoint?:string}}[]>([]);
let disposed=false;
onMounted(()=>{void request<typeof connections.value>('/integrations').then(rows=>{if(!disposed)connections.value=Array.isArray(rows)?rows:[]}).catch(()=>{})});
onUnmounted(()=>{disposed=true});
const selectedTask=computed(()=>tasks.value.find(t=>t.id===detail.value?.id));
const detailGroups=computed(()=>downloadDetailGroups(selectedTask.value||detail.value,!!selectedTask.value,connections.value));
const actionName = computed(() => ({ pause: '暂停上传', resume: '恢复上传', cancel: '取消上传', restart: '核对后重建', bind: '绑定既有上传会话', retry: '重新排队', cleanup: '清理本地下载与种子' } as Record<string, string>)[confirmation.value?.action] || '确认操作');
function show(row: any) {
  detail.value = row;
  detailsOpen.value = true;
}
usePageRefresh(()=>refresh(),'tasks');
usePageRefresh(()=>refresh(),'uploads');
</script>
<template>
  <div class="acquire-workspace">
    <PageHeader title="下载" description="下载、做种、上传与入库分开跟进；100%传输不代表文件校验或播放完成。">
      <template #actions>
        <el-button type="primary" :loading="syncing||loading.tasks||loading.transfers" @click="syncDownloads">同步下载任务</el-button>
      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="tasks"/>
    <p v-if="message" role="status">{{ message }}</p>
    <AsyncResultBanner v-if="notice" :status="notice.phase" :message="message" @refresh="refresh()"/>
    <template v-if="tab==='tasks'">
      <section class="acquire-surface">
        <header class="acquire-section-heading">
          <div>
            <h2>下载任务</h2>
            <p>{{joinMetadata([joinMetadata(["共 ",(loading.tasks||errors.tasks?'—':tasks.length)," 项"], ''),joinMetadata(["处理中 ",(loading.tasks||errors.tasks?'—':tasks.filter(t=>!['imported','failed','cancelled'].includes(t.state)).length)," 项"], '')])}}</p>
            <p>下载进度与做种状态独立，原文件保留在下载器中。</p>
          </div>
          <RouterLink to="/search?tab=query">搜索并创建下载</RouterLink>
        </header>
        <div class="download-content-stage" :aria-busy="loading.tasks">
        <StateView v-if="loading.tasks&&!tasks.length" state="loading" title="正在读取下载任务…"/>
        <StateView v-else-if="errors.tasks&&!tasks.length" :state="stateFor(errors.tasks)" title="下载任务暂不可用" :description="errorText(errors.tasks)" @retry="load('tasks')"/>
        <StateView v-else-if="!tasks.length" state="empty" title="尚无下载任务" description="先配置下载器和站点，再从资源搜索投递。"/>
        <p v-if="errors.tasks&&tasks.length" role="status">刷新失败，保留上次下载数据；请在详情核对同步状态。</p>
        <DownloadRow v-for="task in tasks" :key="task.id" :title="task.title" :state="task.syncError?'failed':task.state" :state-label="task.syncError?'同步失败':label(task.state)" :percent="task.progress==null?null:task.progress*100" interactive :class="{highlighted:route.query.taskId===task.id}" @inspect="show(task)">
          <template #meta><span>速度 {{task.syncedAt&&!task.syncError?'↓ '+size(task.downloadSpeed)+'/s · ↑ '+size(task.uploadSpeed)+'/s':'—'}}</span><span v-if="remainingTime(task)!=='—'">剩余（估算）{{remainingTime(task)}}</span><span v-if="task.siteName">站点 {{task.siteName}}</span></template>
          <template #actions><el-button @click="show(task)">查看详情</el-button><el-button v-if="task.state==='imported'" :disabled="['succeeded','deleting','uncertain'].includes(task.cleanupState||'')||!!locked('tasks',task.id,'cleanup')" @click="ask(task,'tasks','cleanup')">清理本地与种子</el-button></template>
        </DownloadRow>
        </div>
        <div class="download-list-footer" :class="{'is-loading':loading.tasks}" :inert="loading.tasks" :aria-hidden="loading.tasks">
        <p>暂停、恢复或只移除任务，请在下载器里操作。“清理本地与种子”会同时删除本地文件。</p>
      <PagePagination label="下载任务分页" v-model:current-page="pages.tasks" :total="totals.tasks" v-model:page-size="sizes.tasks" @current-change="load('tasks')"/>
        </div>
      </section>
    </template>
    <template v-else-if="tab==='uploads'">
      <section class="acquire-surface">
        <header class="acquire-section-heading">
          <div>
            <h2>115上传与入库</h2>
            <p>{{joinMetadata([joinMetadata(["共 ",(loading.transfers||errors.transfers?'—':transfers.length)," 项"], ''),joinMetadata(["待跟进 ",(loading.transfers||errors.transfers?'—':transfers.filter(t=>!['imported','cancelled'].includes(t.state)).length)," 项"], '')])}}</p>
            <p>先核对远端会话、文件和哈希，再确认输出与入库；保留本地做种源。</p>
          </div>
          <RouterLink to="/organize?tab=history&amp;kind=transfer">查看上传与入库记录</RouterLink>
        </header>
        <StateView v-if="loading.transfers&&!transfers.length" state="loading" title="正在读取上传任务…"/>
        <StateView v-else-if="errors.transfers" :state="stateFor(errors.transfers)" title="上传任务暂不可用" :description="errorText(errors.transfers)" @retry="load('transfers')"/>
        <StateView v-else-if="!transfers.length" state="empty" title="尚无上传任务" description="下载完成且文件服务可用后，会准备上传。"/>
        <DownloadRow v-for="transfer in transfers" :key="transfer.id" :path="transfer.destination" :state="transfer.state" :state-label="label(transfer.state)" :percent="uploadPercent(transfer)" :class="{highlighted:route.query.transferId===transfer.id}">
          <template #meta><span v-if="transfer.transferMethod==='instant_or_reused'&&uploadPercent(transfer)===100">秒传或复用</span><span v-if="uploadInProgress(transfer.state)&&uploadPercent(transfer)!==100">网络已传 {{size(transfer.bytes)}} / {{size(transfer.total)}}；哈希计算 {{size(transfer.hashBytes)}}</span><span v-if="transfer.processingStage&&uploadPercent(transfer)!==100">{{transfer.processingStage}}</span><span v-if="transfer.message">{{transfer.message}}</span></template>
          <template #actions>
            <el-button @click="show(transfer)">查看详情</el-button>
            <TaskControl kind="transfers" :id="transfer.id" :state="transfer.state" @changed="load('transfers')"/>
            <el-button v-if="['needs_output_reconciliation','output_resume_approved'].includes(transfer.state)" @click="output?.open(transfer.id)">核对入库恢复</el-button>
            <template v-else>
              <el-button :disabled="busy" @click="inspectTransfer(transfer)">核对</el-button>
              <el-dropdown trigger="click" popper-class="acquire-dropdown">
                <el-button>上传控制</el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item :disabled="busy||!!locked('transfers',transfer.id,'pause')" @click="ask(transfer,'transfers','pause')">暂停</el-dropdown-item>
                    <el-dropdown-item :disabled="busy||!!locked('transfers',transfer.id,'resume')" @click="ask(transfer,'transfers','resume')">恢复</el-dropdown-item>
                    <el-dropdown-item v-if="['uncertain','starting'].includes(transfer.state)" @click="bind(transfer)">绑定会话</el-dropdown-item>
                    <el-dropdown-item :disabled="!transfer.flow?.retryable||!!locked('transfers',transfer.id,'retry')" @click="ask(transfer,'transfers','retry')">确认后重试</el-dropdown-item>
                    <el-dropdown-item @click="ask(transfer,'transfers','cancel')">取消上传</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </template>
          </template>
        </DownloadRow>
      <PagePagination label="上传任务分页" v-model:current-page="pages.transfers" :total="totals.transfers" v-model:page-size="sizes.transfers" @current-change="load('transfers')"/>
      </section>
      <section class="acquire-surface" :class="{'empty-batch-row':!batches.length&&!loading.batches&&!errors.batches}">
        <h2>需核对的批次</h2>
        <StateView v-if="errors.batches" :state="stateFor(errors.batches)" title="批次记录暂不可用" @retry="load('batches')"/>
        <p v-else-if="loading.batches">正在读取批次…</p>
        <p v-else-if="!batches.length">暂无需核对的批次</p>
        <article v-for="batch in batches" :key="batch.id" class="acquire-task">
          <div>
            <h3>{{ batch.message||'文件批次待核对' }}</h3>
            <MiddleEllipsis :path="batch.destination||batch.source"/>
          </div>
          <RouterLink :to="{path:'/organize',query:{tab:'history',kind:batch.kind,batchId:batch.batchId||batch.id}}">查看步骤并核对</RouterLink>
        </article>
      <PagePagination label="待核对批次分页" v-model:current-page="pages.batches" :total="totals.batches" v-model:page-size="sizes.batches" @current-change="load('batches')"/>
      </section>
    </template>
    <ManualWorkspace v-else ref="manualView"/>
    <ConfirmAction v-model:visible="confirmOpen" :title="actionName" :description="cleanup?'将删除此种子与对应本地下载文件。请确认上传、哈希校验及入库均已完成，并已满足站点 H&R / 做种要求。':confirmation?.action==='restart'?'远端未见匹配任务或文件。请先在 CloudDrive2 再次核对，再明确允许建立新会话。':confirmation?.action==='bind'?'将绑定你选中的现有上传会话，并先暂停它。':'将控制已保存的任务，不会另建下载任务。'" :impact-scope="cleanup?'删除不可逆；同时删除本地下载文件，不删除云端文件、STRM 或媒体库内容。':'不删除本地做种文件或云端已有数据；操作后需重新读取核对。'" :confirm-text="cleanup?'已满足做种要求，批准清理':'确认操作'" :danger-level="cleanup?'danger':'warning'" :loading="busy" class="acquire-dialog" @confirm="execute"/>
    <el-dialog v-model="binding" title="绑定既有上传会话" width="min(650px,94vw)" class="acquire-dialog">
      <div class="acquire-form">
        <p>只展示匹配目标的已有会话；确认绑定会先暂停，不会新建上传。</p>
        <StateView v-if="sessionBusy" state="loading" title="正在读取可绑定会话…"/>
        <StateView v-else-if="sessionError" :state="stateFor(sessionError)" title="会话读取失败" :description="errorText(sessionError)"/>
        <label v-for="(session,index) in sessions" :key="session.id" class="acquire-consent">
          <input v-model="sessionId" type="radio" :value="session.id"/>
          会话
          {{ index+1 }}
          ·
          {{ size(session.size) }}
        </label>
        <p v-if="!sessionBusy&&!sessionError&&!sessions.length">没有取得可匹配会话，请先核对文件服务。</p>
        <el-button :disabled="!sessionId" @click="askBinding">确认绑定并暂停</el-button>
      </div>
    </el-dialog>
    <DetailDrawer v-model:visible="detailsOpen" title="任务详情" size="large" class="acquire-detail download-detail">
      <h3><MiddleEllipsis :text="selectedTask?.title||detail?.title||detail?.destination" :copy="false"/></h3>
      <section v-for="group in detailGroups" :key="group.key" :aria-label="group.title"><h4>{{group.title}}</h4><DescriptionGrid :items="group.items" hide-single-label><template #state><StatusBadge :state="selectedTask?.state||detail?.state" :label="label(selectedTask?.state||detail?.state)"/></template></DescriptionGrid></section>
      <template v-for="task in selectedTask?[selectedTask]:[]" :key="task.id">
        <el-alert v-if="task.syncError" :title="'同步失败（保留上次进度）：'+task.syncError" type="warning" :closable="false"/>
        <p v-if="task.trackerError" role="alert">Tracker状态未同步：{{task.trackerError}}</p><p v-for="tracker in task.trackers?.filter(t=>t.status>=4&&t.message)||[]" :key="tracker.url" role="alert">Tracker：{{tracker.message}}</p>


      </template>
      <template v-if="selectedTask" #footer><template v-for="task in [selectedTask]" :key="task.id">
          <div class="acquire-actions download-detail-actions">
            <TaskControl kind="downloads" :id="task.id" :state="task.state" @changed="load('tasks')"/>
            <el-button v-if="task.sourceRecoveryRequired" @click="detailsOpen=false;source?.open(task.id)">核对来源</el-button>
            <el-button v-else-if="task.flow?.retryable" :disabled="!!locked('tasks',task.id,'retry')" @click="ask(task,'tasks','retry')">确认后重试</el-button>
            <el-button v-if="['uncertain','deleting'].includes(task.cleanupState||'')" @click="reconcileCleanup(task)">核对清理结果</el-button>
            <el-button v-if="task.state==='imported'" :disabled="['succeeded','deleting','uncertain'].includes(task.cleanupState||'')||!!locked('tasks',task.id,'cleanup')" @click="detailsOpen=false;ask(task,'tasks','cleanup')">清理本地与种子</el-button>
          </div>
      </template></template>
    </DetailDrawer>
    <OutputRecoveryDialog ref="output" @completed="refresh()"/>
    <SourceRecoveryDialog ref="source" @completed="refresh()"/>
  </div>
</template>
