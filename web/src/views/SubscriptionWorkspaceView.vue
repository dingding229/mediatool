<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { computed } from 'vue'
import { ElDropdown, ElDropdownMenu, ElDropdownItem, ElIcon } from 'element-plus'
import 'element-plus/es/components/dropdown/style/css'
import 'element-plus/es/components/dropdown-menu/style/css'
import 'element-plus/es/components/dropdown-item/style/css'
import 'element-plus/es/components/icon/style/css'
import { Plus, Search, VideoPlay, ArrowRight } from '@element-plus/icons-vue'
import PageHeader from '../ui/PageHeader.vue'
import PageTabs from '../ui/PageTabs.vue'
import StateView from '../ui/StateView.vue'
import ConfirmAction from '../ui/ConfirmAction.vue'
import DetailDrawer from '../ui/DetailDrawer.vue'
import AsyncResultBanner from '../ui/AsyncResultBanner.vue'
import FlowHint from '../ui/FlowHint.vue'
import StatusBadge from '../components/StatusBadge.vue'
import SubscriptionComposer from '../subscriptions/SubscriptionComposer.vue'
import RunReview from '../subscriptions/RunReview.vue'
import type { Subscription } from '../subscriptions/model'
import { discovery, quality, stateFor, stateTitle, timestamp } from '../subscriptions/model'
import { useSubscriptionWorkspace } from '../subscriptions/useSubscriptionWorkspace'
import '../subscriptions/subscriptions.css'
const { tab, scope, records, accounts, loading, listError, accountError, search, filter, visible, selected, detailOpen, inspect,
  history, historyLoading, historyError, historyId, activeRunId, run, runLoading, runError, runDetailOpen, readRun, chooseRun, showHistory, loadHistory,
  pending, evaluate, reconcile, refresh, changeScope, compose, draft, editorOpen, saving, saveError, save, reloadEditor,
  previewDraft, previewReady, previewRun, previewError, toggle, mutation, askDelete, deleteOpen, deleteTarget, deleting, confirmDelete } = useSubscriptionWorkspace()
function moreAction(command: string | number | object, record: Subscription) {
  if (command === 'history') void showHistory(record.id)
  else if (command === 'preview') void evaluate(record, true)
  else if (command === 'toggle') void toggle(record)
  else if (command === 'archive') askDelete(record)
}
const tabs = [{ id: 'list', label: '订阅列表' }, { id: 'history', label: '运行记录' }]
const currentRecord = computed(() => records.value.find(s => s.id === historyId.value))
const runTime = computed(() => history.value.find(r => r.id === activeRunId.value)?.createdAt)
const siteNames = (ids: string[]) => ids.map(id => accounts.value.find(a => a.id === id)?.name || '站点暂不可用').join('、') || '—'
usePageRefresh(refresh,'*');
</script>
<template>
  <div class="mt-subscriptions">
    <PageHeader title="订阅" description="选择想看的影视，设置筛选条件，自动跟进新资源。">
      <template #actions><el-button type="primary" :icon="Plus" @click="compose()">新建订阅</el-button></template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="list" />
    <AsyncResultBanner v-if="pending" :status="pending.status" :can-refresh="pending.status !== 'locked'" :message="pending.status === 'locked' ? '正在提交运行请求，已锁定重复提交；尚未确认排队。' : pending.status === 'queued' ? (pending.preview ? '规则预演已排队，不创建下载。请等待运行结果。' : '运行已排队，尚未完成；只有已启用的订阅才会自动下载。') : '上次运行回执尚未确认。已锁定重复提交，请读取状态或核对运行记录。'" @refresh="reconcile" />
    <FlowHint v-if="pending?.status === 'reconciling'" reason="没有运行编号时不会猜测任务归属，也不会重新发起运行。" next-action="查看运行记录" clickable @action="showHistory(pending.subscriptionId)" />
    <section class="subscription-list-section subscription-workbench" aria-label="订阅工作区">
      <header class="subscription-commandbar">
        <div><h2>{{ tab === 'list' ? '我的跟进清单' : '跟进记录' }}</h2><p v-if="tab === 'history'">记录不会因订阅归档而丢失。选择一次运行，逐条核对决策。</p></div>
        <label class="subscription-scope">查看范围<el-select :model-value="scope" aria-label="订阅范围" @change="changeScope"><el-option label="当前" value="current" /><el-option label="已归档" value="history" /></el-select></label>
      </header>
      <StateView v-if="loading" state="loading" title="正在读取订阅…" />
      <StateView v-else-if="listError" :state="stateFor(listError)" :title="stateTitle(listError, '订阅列表')" :description="listError.message" @retry="refresh" />
      <template v-else-if="tab === 'list'">
        <div class="subscription-findbar">
          <el-input v-model="search" :prefix-icon="Search" clearable aria-label="搜索订阅" placeholder="搜索影视名称" />
          <div class="subscription-state-filters" aria-label="订阅状态">
            <button v-for="f in [{id:'all',label:'全部'},{id:'enabled',label:'已启用'},{id:'paused',label:'已停用'}]" :key="f.id" type="button" :aria-pressed="filter === f.id" @click="filter = f.id">{{ f.label }}</button>
          </div>
          <span>{{ visible.length }} 条订阅</span>
        </div>
        <StateView v-if="!visible.length" state="empty" :title="records.length ? '没有符合筛选条件的订阅' : scope === 'history' ? '暂无已归档订阅' : '还没有订阅，从一部想看的影视开始'" description="点击右上角新建订阅，选影视、设规则、核对后保存。新订阅默认停用。" />
        <div v-else class="subscription-ledger">
          <div class="subscription-columns" aria-hidden="true"><span>跟进对象</span><span>站点与质量要求</span><span>最近结果 / 下一次</span><span>操作</span></div>
          <article v-for="record in visible" :key="record.id" class="subscription-row">
            <div class="subscription-subject">
              <span class="subscription-media-symbol"><el-icon :size="22"><VideoPlay /></el-icon></span>
              <div>
                <button class="subscription-title" @click="inspect(record)">{{ record.title || '—' }}<el-icon><ArrowRight /></el-icon></button>
                <p>{{ record.media.mediaType === 'movie' ? '电影' : '剧集' }} · {{ record.media.year || '—' }}<span v-if="record.media.mediaType === 'series'"> · 第 {{ record.rules.season }} 季</span></p>
                <StatusBadge :state="record.enabled ? 'running' : 'disabled'" :label="record.enabled ? '已启用' : '已停用'" />
              </div>
            </div>
            <div class="subscription-rule-summary"><strong>{{ quality(record.rules) }}</strong><p>{{ siteNames(record.rules.accountIds) }}</p><small>{{ discovery[record.source] || '—' }} · {{ record.rules.freeOnly ? '仅免费资源' : '不限促销' }}</small></div>
            <div class="subscription-schedule"><StatusBadge v-if="record.lastState" :state="record.lastState" /><span v-else>尚未运行</span><p>{{ timestamp(record.nextRunAt) }}</p></div>
            <div class="subscription-row-actions">
              <template v-if="scope === 'current'">
                <el-button type="primary" :disabled="!!pending" @click="evaluate(record, false)">立即运行</el-button>
                <el-button :disabled="!!pending" @click="compose(record)">编辑</el-button>
              </template>
              <ElDropdown trigger="click" @command="command => moreAction(command, record)">
                <el-button :aria-label="`${record.title}更多操作`">更多</el-button>
                <template #dropdown><ElDropdownMenu class="subscription-more-menu">
                  <ElDropdownItem command="history">运行记录</ElDropdownItem>
                  <template v-if="scope === 'current'">
                    <ElDropdownItem command="preview" :disabled="!!pending">预演</ElDropdownItem>
                    <ElDropdownItem command="toggle" :disabled="!!pending || !!mutation">{{ record.enabled ? '停用' : '启用' }}</ElDropdownItem>
                    <ElDropdownItem command="archive" divided :disabled="!!pending">归档</ElDropdownItem>
                  </template>
                </ElDropdownMenu></template>
              </ElDropdown>
            </div>
          </article>
        </div>
        <footer class="subscription-list-footer"><FlowHint reason="停用只停止后续自动下载；归档会保留下载、文件和运行历史。" /></footer>
      </template>
      <template v-else>
        <div class="history-subject-picker">
          <label for="history-subscription">查看哪条订阅</label>
          <el-select id="history-subscription" :model-value="historyId" aria-label="选择订阅查看运行记录" placeholder="选择订阅" @change="showHistory"><el-option v-for="record in records" :key="record.id" :label="record.title" :value="record.id" /></el-select>
          <span>{{ currentRecord?.title || '—' }}</span>
        </div>
        <StateView v-if="historyLoading" state="loading" title="正在读取运行记录…" />
        <StateView v-else-if="historyError" :state="stateFor(historyError)" :title="stateTitle(historyError, '运行记录')" :description="historyError.message" @retry="loadHistory" />
        <div v-else class="subscription-history-workspace">
          <nav class="run-timeline" aria-label="运行时间线">
            <header><h3>运行记录</h3><span>{{ history.length }} 次</span></header>
            <p v-if="!history.length">暂无运行记录；预演或运行后会显示在这里。</p>
            <button v-for="item in history" :key="item.id" type="button" :aria-pressed="activeRunId === item.id" @click="chooseRun(item.id)">
              <span class="timeline-dot" aria-hidden="true" />
              <span><strong>{{ item.preview ? '规则预演' : '正式运行' }}</strong><small>{{ timestamp(item.createdAt) }}</small><StatusBadge :state="item.state" /></span>
            </button>
          </nav>
          <RunReview :run="run" :loading="runLoading" :error="runError" :created-at="runTime" @refresh="readRun(activeRunId)" @detail="runDetailOpen = true" />
        </div>
      </template>
    </section>
    <SubscriptionComposer v-model:visible="editorOpen" v-model:draft="draft" :accounts="accounts" :account-error="accountError" :saving="saving" :error="saveError" :preview-ready="previewReady" :preview-run="previewRun" :preview-error="previewError" :pending="pending" @preview="previewDraft" @reconcile="reconcile" @save="save" @reload="reloadEditor" @retry-accounts="refresh" />
    <ConfirmAction v-model:visible="deleteOpen" class="subscription-confirm" title="归档订阅" :description="`将停止「${deleteTarget?.title || '—'}」的后续自动订阅。`" impact-scope="仅归档此订阅；保留规则、运行记录及下载关联，不删除已有下载、云盘文件或做种源文件。" danger-level="danger" confirm-text="确认归档订阅" :loading="deleting" @confirm="confirmDelete" />
    <DetailDrawer :key="selected?.id" v-model:visible="detailOpen" title="订阅详情" class="subscription-detail" :technical-data="selected">
      <template v-if="selected">
        <div class="subscription-detail-target"><span>跟进对象</span><h2>{{ selected.title || '—' }}</h2><StatusBadge :state="selected.enabled ? 'running' : 'disabled'" :label="selected.enabled ? '已启用' : '已停用'" /></div>
        <dl class="subscription-detail-facts">
          <div><dt>监控站点</dt><dd>{{ siteNames(selected.rules.accountIds) }}</dd></div>
          <div><dt>质量要求</dt><dd>{{ quality(selected.rules) }}</dd></div>
          <div><dt>搜索与筛选</dt><dd>{{ selected.rules.keyword || '—' }}<small>包含：{{ selected.rules.include?.join('、') || '—' }}<br />排除：{{ selected.rules.exclude?.join('、') || '—' }}</small></dd></div>
          <div><dt>最近运行说明</dt><dd>{{ selected.lastMessage || '—' }}</dd></div>
          <div><dt>下次运行</dt><dd>{{ timestamp(selected.nextRunAt) }}</dd></div>
        </dl>
        <div class="subscription-detail-actions">
          <el-button @click="showHistory(selected.id)">查看运行记录</el-button>
          <template v-if="scope === 'current'">
            <el-button :disabled="!!pending" @click="compose(selected)">编辑规则</el-button>
            <el-button :loading="mutation === selected.id" :disabled="!!pending || !!mutation" @click="toggle(selected)">{{ selected.enabled ? '停用' : '启用' }}</el-button>
            <el-button type="danger" plain :disabled="!!pending" @click="askDelete(selected)">归档订阅</el-button>
          </template>
        </div>
      </template>
    </DetailDrawer>
    <DetailDrawer :key="activeRunId" v-model:visible="runDetailOpen" title="运行详情" class="subscription-detail" :technical-data="run">
      <p>以下记录由服务端返回。排队不等于执行完成；下载决策也不等于媒体已入库。</p>
      <StatusBadge v-if="run" :state="run.state" />
      <p>候选决策：{{ run?.decisions?.length ?? '—' }} 条</p>
    </DetailDrawer>
  </div>
</template>
