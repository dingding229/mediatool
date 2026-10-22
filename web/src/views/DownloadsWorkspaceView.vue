<script setup lang="ts">
import TaskControl from '../workflows/TaskControl.vue';
import {usePageRefresh} from '../ui/page-refresh';
import { computed, ref } from 'vue';
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
import SourceRecoveryDialog from '../acquisition/SourceRecoveryDialog.vue';
import ManualWorkspace from '../acquisition/ManualWorkspace.vue';
import { useDownloads } from '../acquisition/useDownloads';
import { label, progress, size, qbLabel, seedingLabel } from '../acquisition/model';
import { date, errorText, stateFor } from '../workflows/readiness';
import '../acquisition/acquisition.css';
const route = useRoute(), tabs = [{ id: 'tasks', label: '下载任务' }, { id: 'uploads', label: '上传入库' }, { id: 'manual', label: '手动添加' }], tab = computed(() => tabs.some(t => t.id === route.query.tab) ? String(route.query.tab) : 'tasks');
const {pages,totals, tasks, transfers, batches, loading, errors, notice, message, busy, confirmOpen, confirmation, sessions, sessionBusy, sessionError, binding, sessionId, refresh, load, locked, ask, execute, inspectTransfer, reconcileCleanup, bind, askBinding } = useDownloads();
const source = ref<InstanceType<typeof SourceRecoveryDialog>>(), detail = ref<any>(null), detailsOpen = ref(false), cleanup = computed(() => confirmation.value?.action === 'cleanup');
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
        <el-button v-if="tab==='tasks'" type="primary" :loading="loading.tasks||loading.transfers" @click="refresh(true)">同步下载任务</el-button>
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
            <p>共 {{ loading.tasks||errors.tasks?'—':tasks.length }} 项 · 处理中 {{ loading.tasks||errors.tasks?'—':tasks.filter(t=>!['imported','failed','cancelled'].includes(t.state)).length }} 项</p>
            <p>下载进度与做种状态独立，原文件保留在下载器中。</p>
          </div>
          <RouterLink to="/search?tab=query">搜索并创建下载</RouterLink>
        </header>
        <StateView v-if="loading.tasks&&!tasks.length" state="loading" title="正在读取下载任务…"/>
        <StateView v-else-if="errors.tasks" :state="stateFor(errors.tasks)" title="下载任务暂不可用" :description="errorText(errors.tasks)" @retry="load('tasks')"/>
        <StateView v-else-if="!tasks.length" state="empty" title="尚无下载任务" description="先配置下载器和站点，再从资源搜索投递。"/>
        <article v-for="task in tasks" :key="task.id" class="acquire-task" :class="{highlighted:route.query.taskId===task.id}">
          <div>
            <h3>{{ task.title }}</h3>
            <p>{{ task.media?.title||'—' }} · {{ task.media?.year||'—' }}</p>
            <p>{{ label(task.state) }} · {{ qbLabel(task.qbState) }}</p>
            <el-progress v-if="task.progress!=null" :percentage="Math.max(0,Math.min(100,Math.round(task.progress*100)))"/>
            <p v-else>尚未同步</p>
            <p>↓ {{ task.syncedAt?size(task.downloadSpeed)+'/s':'—' }} / ↑ {{ task.syncedAt?size(task.uploadSpeed)+'/s':'—' }} · 做种连接 {{ task.seeds??'—' }} / 下载连接 {{ task.peers??'—' }}</p>
            <p>做种：{{ seedingLabel(task.seedingState) }} · {{ date(task.syncedAt) }}</p>
            <el-alert v-if="task.syncError" :title="'同步失败（保留上次进度）：'+task.syncError" type="warning" :closable="false"/>
            <p v-if="task.trackerError" role="alert">Tracker状态未同步：{{ task.trackerError }}</p>
            <p v-for="tracker in task.trackers?.filter(t=>t.status>=4&&t.message)||[]" :key="tracker.url" role="alert">Tracker：{{ tracker.message }}</p>
            <p>{{ task.message||'—' }}</p>
            <p v-if="task.cleanupState&&task.cleanupState!=='disabled'">清理：{{ label(task.cleanupState) }} · {{ task.cleanupMessage||'—' }}</p>
          </div>
          <div class="acquire-actions">
            <el-button @click="show(task)">查看详情</el-button>
            <TaskControl kind="downloads" :id="task.id" :state="task.state" @changed="load('tasks')"/>
            <el-button v-if="task.sourceRecoveryRequired" @click="source?.open(task.id)">核对来源</el-button>
            <el-button v-else-if="task.flow?.retryable" :disabled="!!locked('tasks',task.id,'retry')" @click="ask(task,'tasks','retry')">确认后重试</el-button>
            <el-button v-if="['uncertain','deleting'].includes(task.cleanupState||'')" @click="reconcileCleanup(task)">核对清理结果</el-button>
            <el-dropdown v-if="task.state==='imported'" trigger="click" popper-class="acquire-dropdown">
              <el-button>更多</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item :disabled="['succeeded','deleting','uncertain'].includes(task.cleanupState||'')||!!locked('tasks',task.id,'cleanup')" @click="ask(task,'tasks','cleanup')">清理本地与种子</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </article>
        <p>暂停、恢复或只移除任务，请在下载器里操作。这里的“清理”会同时删除本地文件。</p>
      <PagePagination label="下载任务分页" v-model:current-page="pages.tasks" :total="totals.tasks" :page-size="25" @current-change="load('tasks')"/>
      </section>
      <RouterLink to="/connections?tab=downloader">管理下载器</RouterLink>
    </template>
    <template v-else-if="tab==='uploads'">
      <section class="acquire-surface">
        <header class="acquire-section-heading">
          <div>
            <h2>115上传与入库</h2>
            <p>共 {{ loading.transfers||errors.transfers?'—':transfers.length }} 项 · 待跟进 {{ loading.transfers||errors.transfers?'—':transfers.filter(t=>!['imported','cancelled'].includes(t.state)).length }} 项</p>
            <p>先核对远端会话、文件和哈希，再确认输出与入库；保留本地做种源。</p>
          </div>
          <RouterLink to="/organize?tab=history&amp;kind=transfer">查看上传与入库记录</RouterLink>
        </header>
        <StateView v-if="loading.transfers&&!transfers.length" state="loading" title="正在读取上传任务…"/>
        <StateView v-else-if="errors.transfers" :state="stateFor(errors.transfers)" title="上传任务暂不可用" :description="errorText(errors.transfers)" @retry="load('transfers')"/>
        <StateView v-else-if="!transfers.length" state="empty" title="尚无上传任务" description="下载完成且文件服务可用后，会准备上传。"/>
        <article v-for="transfer in transfers" :key="transfer.id" class="acquire-task" :class="{highlighted:route.query.transferId===transfer.id}">
          <div>
            <h3>{{ transfer.destination||'—' }}</h3>
            <p>{{ label(transfer.state) }}</p>
            <el-progress v-if="transfer.processingProgress!=null" :percentage="Math.max(0,Math.min(100,transfer.processingProgress))"/>
            <p>{{ transfer.processingStage||'—' }}</p>
            <p>网络已传 {{ size(transfer.bytes) }} / {{ size(transfer.total) }}；哈希计算 {{ size(transfer.hashBytes) }}</p>
            <p v-if="transfer.transferMethod==='instant_or_reused'">秒传或复用；节省字节不计作网络传输</p>
            <p>{{ transfer.message||'—' }}</p>
          </div>
          <div class="acquire-actions">
            <el-button @click="show(transfer)">查看详情</el-button>
            <TaskControl kind="transfers" :id="transfer.id" :state="transfer.state" @changed="load('transfers')"/>
            <RouterLink v-if="['needs_output_reconciliation','output_resume_approved'].includes(transfer.state)" :to="{path:'/organize',query:{tab:'recovery',transferId:transfer.id}}">核对入库恢复</RouterLink>
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
          </div>
        </article>
      <PagePagination label="上传任务分页" v-model:current-page="pages.transfers" :total="totals.transfers" :page-size="25" @current-change="load('transfers')"/>
      </section>
      <section class="acquire-surface">
        <h2>需核对的批次</h2>
        <StateView v-if="errors.batches" :state="stateFor(errors.batches)" title="批次记录暂不可用" @retry="load('batches')"/>
        <p v-else-if="loading.batches">正在读取批次…</p>
        <p v-else-if="!batches.length">暂无待核对批次</p>
        <article v-for="batch in batches" :key="batch.id" class="acquire-task">
          <div>
            <h3>{{ batch.message||'文件批次待核对' }}</h3>
            <p>{{ batch.destination||batch.source||'—' }}</p>
          </div>
          <RouterLink :to="{path:'/organize',query:{tab:'history',kind:batch.kind,batchId:batch.batchId||batch.id}}">查看步骤并核对</RouterLink>
        </article>
      <PagePagination label="待核对批次分页" v-model:current-page="pages.batches" :total="totals.batches" :page-size="25" @current-change="load('batches')"/>
      </section>
    </template>
    <ManualWorkspace v-else/>
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
    <DetailDrawer v-model:visible="detailsOpen" title="任务详情" :technical-data="detail" class="acquire-detail">
      <h3>{{ detail?.title||detail?.destination||'—' }}</h3>
      <p>{{ label(detail?.state) }} · {{ detail?.message||'—' }}</p>
      <p>下载进度：{{ progress(detail?.progress) }}</p>
      <p>操作前请核对任务的最新状态。</p>
    </DetailDrawer>
    <SourceRecoveryDialog ref="source" @completed="refresh()"/>
  </div>
</template>
