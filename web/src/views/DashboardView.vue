<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Refresh, ArrowRight, Connection, Operation, Warning, Search, FolderOpened, Download, Cloudy, Link, VideoPlay } from '@element-plus/icons-vue'
import { useSystemStore } from '../stores/system'
import { request } from '../api/client'
import type { Job } from '../api/types'
import StatusBadge from '../components/StatusBadge.vue'
const system = useSystemStore(), jobs = ref<Job[] | null>(null), jobsError = ref(''), busy = ref(false), updatedAt = ref('')
const connected = computed(() => system.status?.dependencies.filter(dep => dep.state === 'ready').length)
const activeJobs = computed(() => jobs.value?.filter(job => ['pending','running','retry'].includes(job.state)).length)
const failedJobs = computed(() => jobs.value?.filter(job => job.state === 'failed').length)
const jobNames: Record<string,string> = { 'system.check': '系统检查', 'pt.search': '站点搜索', 'strm.generate': '生成 STRM' }
const steps = [
  { title: '发现资源', detail: '订阅与站点搜索', to: '/search', icon: Search },
  { title: '下载与做种', detail: '跟进下载与传输', to: '/downloads', icon: Download },
  { title: '云盘整理', detail: '扫描、识别与归档', to: '/cloudStorage', icon: Cloudy },
  { title: '生成 STRM', detail: '保存固定解析链接', to: '/strm', icon: Link },
  { title: '媒体库播放', detail: '播放时解析 115 直链', to: '/mediaLibrary', icon: VideoPlay },
]
async function refresh() {
  busy.value = true
  try {
    await system.refresh()
    try { jobs.value = await request<Job[]>('/jobs'); jobsError.value = '' } catch (e) { jobs.value = null; jobsError.value = e instanceof Error ? e.message : '任务状态不可用' }
    updatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } finally { busy.value = false }
}
onMounted(refresh)
</script>
<template>
  <section class="page-heading"><div><span class="page-kicker">WORKSPACE / 总览</span><h1>媒体链路总览</h1><p>连接资源、云盘与媒体库，让每一次入库都有迹可循。</p></div><div class="heading-actions"><span v-if="updatedAt" class="snapshot-time">上次查询 {{ updatedAt }}</span><el-button :loading="busy" @click="refresh"><Refresh aria-hidden="true" />刷新状态</el-button></div></section>
  <el-alert v-if="system.error" :title="system.error" type="error" :closable="false" show-icon />
  <section class="metrics-grid" aria-label="运行状态快照">
    <article class="metric-card"><Connection aria-hidden="true" /><span class="metric-label">已连接依赖</span><strong>{{ connected ?? '—' }}<small> / {{ system.status?.dependencies.length ?? '—' }}</small></strong><p>本次查询的依赖服务状态</p></article>
    <article class="metric-card"><Operation aria-hidden="true" /><span class="metric-label">队列快照</span><strong>{{ jobs?.length ?? '—' }}<small> 项</small></strong><p>最近返回的任务，非全量统计</p></article>
    <article class="metric-card"><Download aria-hidden="true" /><span class="metric-label">待处理 / 执行中</span><strong>{{ activeJobs ?? '—' }}<small> 项</small></strong><p>含等待执行与重试的任务</p></article>
    <article class="metric-card" :class="{ 'needs-attention': failedJobs }"><Warning aria-hidden="true" /><span class="metric-label">失败任务</span><strong>{{ failedJobs ?? '—' }}<small> 项</small></strong><p>可前往任务队列查看原因</p></article>
  </section>
  <section class="workspace-hero"><div class="hero-copy"><span class="hero-eyebrow">YOUR MEDIA, CONNECTED</span><h2>从发现好片，<br />到<span>轻松入库。</span></h2><p>搜索与订阅发现资源，整理文件并生成固定链接。<br class="desktop-break" />Emby 播放时获取 115 临时地址，媒体不经过本程序中转。</p><div class="hero-actions"><RouterLink to="/search" class="action-link primary"><Search aria-hidden="true" />搜索资源<ArrowRight aria-hidden="true" /></RouterLink><RouterLink to="/files" class="action-link secondary"><FolderOpened aria-hidden="true" />整理文件</RouterLink></div></div><div class="hero-flow" aria-label="播放链路说明"><div class="flow-label">固定入口 · 动态解析</div><div class="flow-source"><Link aria-hidden="true" /><span>STRM 固定链接<small>原始地址保持不变</small></span></div><div class="flow-connector"><span></span><small>播放请求 · 实际客户端 UA</small><ArrowRight aria-hidden="true" /></div><div class="flow-target"><VideoPlay aria-hidden="true" /><span>115 官方 CDN<small>302 跳转至临时播放地址</small></span><span class="flow-code">302</span></div></div></section>
  <div class="dashboard-grid">
    <section class="panel"><div class="panel-title"><div><span class="section-kicker">CONNECTIONS</span><h2>依赖服务</h2></div><RouterLink to="/integrations" class="inline-link">连接状态<ArrowRight aria-hidden="true" /></RouterLink></div><div v-if="system.status" class="dependency-list"><div v-for="dep in system.status.dependencies" :key="dep.id" class="dependency-row"><div class="dependency-name"><span class="service-monogram" aria-hidden="true">{{ dep.id === 'postgres' ? 'PG' : dep.id === 'redis' ? 'R' : dep.id === 'emby' ? 'E' : 'CD' }}</span><div><strong>{{ dep.name }}</strong><p>{{ dep.message }}</p></div></div><StatusBadge :state="dep.state" /></div></div><el-skeleton v-else-if="busy" :rows="4" animated /><p v-else class="field-hint">依赖状态不可用，请刷新重试。</p><div class="panel-bottom">连接配置在各功能模块管理；此处仅显示本次检查结果。</div></section>
    <section class="panel recent-jobs"><div class="panel-title"><div><span class="section-kicker">ACTIVITY</span><h2>最近任务</h2></div><RouterLink to="/jobs" class="inline-link">查看队列<ArrowRight aria-hidden="true" /></RouterLink></div><el-skeleton v-if="busy && jobs === null" :rows="4" animated /><div v-else-if="jobs === null" class="empty-state compact"><Operation class="empty-icon" aria-hidden="true" /><h3>任务暂时不可用</h3><p>{{ jobsError }}</p><el-button @click="refresh">重新查询</el-button></div><div v-else-if="jobs.length === 0" class="empty-state compact"><Operation class="empty-icon" aria-hidden="true" /><h3>还没有任务记录</h3><p>开始搜索资源，或运行一次系统检查，任务进度将在这里显示。</p><RouterLink to="/jobs" class="inline-link">运行系统检查<ArrowRight aria-hidden="true" /></RouterLink></div><div v-else class="job-list"><div v-for="job in jobs.slice(0,5)" :key="job.id" class="job-row"><div><strong>{{ jobNames[job.kind] ?? job.kind }}</strong><small>{{ new Date(job.createdAt).toLocaleString('zh-CN') }}</small></div><StatusBadge :state="job.state" /></div></div></section>
  </div>
  <section class="panel pipeline-panel"><div class="panel-title"><div><span class="section-kicker">WORKFLOW</span><h2>资源到播放的路径</h2></div><span class="muted">按需开始，无需按顺序操作</span></div><div class="workflow-grid"><RouterLink v-for="(step,index) in steps" :key="step.to" :to="step.to" class="workflow-step"><span class="workflow-number">0{{ index+1 }}</span><component :is="step.icon" aria-hidden="true" /><h3>{{ step.title }}</h3><p>{{ step.detail }}</p><ArrowRight class="workflow-arrow" aria-hidden="true" /></RouterLink></div></section>
</template>
