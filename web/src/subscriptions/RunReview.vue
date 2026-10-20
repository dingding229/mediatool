<script setup lang="ts">
import { computed, ref } from 'vue'
import StateView from '../ui/StateView.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { outcomes, stateFor, stateTitle, timestamp } from './model'
import type { Run } from './model'
const props = defineProps<{ run: Run | null; loading: boolean; error: Error | null; createdAt?: string; compact?: boolean }>()
const emit = defineEmits<{ refresh: []; detail: [] }>()
const filter = ref('all')
const decisions = computed(() => (props.run?.decisions ?? []).filter(d => filter.value === 'all' || d.action === filter.value))
const submitted = computed(() => props.run?.decisions?.filter(d => d.action === 'download_queued').length ?? 0)
</script>
<template>
  <section class="run-review" aria-label="运行结果">
    <StateView v-if="loading" state="loading" title="正在读取这次运行…" />
    <StateView v-else-if="error" :state="stateFor(error)" :title="stateTitle(error, '运行结果')" :description="error.message" retry-text="重新读取结果" @retry="emit('refresh')" />
    <StateView v-else-if="!run" state="empty" title="选择一次运行，查看它做了什么" description="左侧记录可切换不同运行；预演只检查规则，不创建下载。" />
    <template v-else>
      <header class="run-review-heading">
        <div><p>{{ run.preview ? '仅规则预演 · 不创建下载' : '正式运行 · 实际资源决策' }}</p><h2>{{ run.preview ? '这次规则会选中哪些资源？' : '这次跟进发生了什么？' }}</h2><span>{{ timestamp(createdAt) }}</span></div>
        <StatusBadge :state="run.state" />
      </header>
      <el-alert v-if="run.state === 'needs_refresh'" title="订阅、站点或资源已变化，请刷新列表后重新核对规则。已有下载不会重复创建。" type="warning" :closable="false" />
      <el-alert v-if="run.state === 'failed'" title="这次运行未完成，请检查决策说明或技术详情，再决定下一步。" type="error" :closable="false" />
      <StateView v-if="['pending','running'].includes(run.state)" state="loading" title="任务正在排队或执行，尚未完成…" />
      <template v-else>
        <div class="run-outcome-summary"><span><strong>{{ run.decisions?.length ?? 0 }}</strong> 条候选决策</span><span><strong>{{ submitted }}</strong> 条已加入下载队列</span><el-button v-if="!compact" @click="emit('detail')">完整运行详情</el-button></div>
        <div class="run-filters" aria-label="决策筛选">
          <button v-for="f in [{id:'all',label:'全部决策'},{id:'would_download',label:'预演通过'},{id:'needs_confirmation',label:'需确认'},{id:'skipped',label:'已跳过'}]" :key="f.id" type="button" :aria-pressed="filter === f.id" @click="filter = f.id">
            {{ f.label }}
          </button>
        </div>
        <StateView v-if="!decisions.length" state="empty" :title="filter === 'all' ? '本次没有候选资源决策' : '没有这类决策'" />
        <article v-for="decision in decisions" :key="decision.id" class="run-decision">
          <header><h3>{{ decision.title || '—' }}</h3><span>{{ outcomes[decision.action] || '待核对' }}</span></header>
          <p>{{ decision.reasons?.join('；') || '—' }}</p>
          <dl v-if="decision.checks?.length">
            <div v-for="check in decision.checks" :key="check.key">
              <dt><el-tag :type="check.state === 'rejected' ? 'danger' : check.state === 'matched' ? 'success' : 'info'">{{ check.label }} · {{ check.state === 'rejected' ? '拒绝' : check.state === 'matched' ? '通过' : '不限' }}</el-tag></dt>
              <dd>{{ check.message || '—' }}<small v-if="check.actual || check.expected">要求：{{ check.expected || '—' }} · 识别：{{ check.actual || '—' }}</small></dd>
            </div>
          </dl>
          <p v-if="decision.episodes?.length">覆盖集：{{ decision.episodes.join('、') }}</p>
        </article>
      </template>
    </template>
  </section>
</template>
