<script setup lang="ts">
import { computed,onMounted,ref } from 'vue'
import { Refresh,ArrowRight } from '@element-plus/icons-vue'
import { useSystemStore } from '../stores/system'
import { request } from '../api/client'
import type { Job } from '../api/types'
import StatusBadge from '../components/StatusBadge.vue'
const system=useSystemStore(),jobs=ref<Job[]|null>(null),jobsError=ref('')
const implemented=computed(()=>system.modules.filter(m=>m.stage!=='planned').length)
async function refresh(){await system.refresh();try{jobs.value=await request<Job[]>('/jobs');jobsError.value=''}catch(e){jobs.value=null;jobsError.value=e instanceof Error?e.message:'任务状态不可用'}}
onMounted(refresh)
</script>
<template>
 <section class="page-heading"><div><h1>媒体链路总览</h1><p>先连接服务，再让每一次入库都有迹可循。</p></div><el-button :loading="system.loading" @click="refresh"><Refresh aria-hidden="true"/>刷新状态</el-button></section>
 <el-alert v-if="system.error" :title="system.error" type="error" :closable="false" show-icon class="section-gap"/>
 <section class="pipeline-panel"><div class="panel-title"><h2>资源到播放的路径</h2><span class="phase-label">{{system.status?.phase??'正在读取服务状态'}}</span></div><div class="pipeline"><div v-for="(step,index) in ['PT 订阅与搜索','本地下载与做种','CloudDrive2 → 115','STRM 固定链接','Emby / 302 播放']" :key="step" class="pipeline-step"><span class="pipeline-index">{{index+1}}</span><strong>{{step}}</strong><small>{{index===3?'固定解析已实现':index===2?'gRPC 已实现 · 待配置':index===4?'静态流拦截 · 待实测':'业务链路待接入'}}</small><ArrowRight v-if="index<4" class="pipeline-arrow" aria-hidden="true"/></div></div><p class="pipeline-footnote">115 直链不写入 STRM。播放时请求固定解析入口，再获取有效的临时地址。</p></section>
 <div class="dashboard-grid">
  <section class="panel"><div class="panel-title"><h2>依赖服务</h2><RouterLink to="/settings" class="inline-link">连接配置 <ArrowRight aria-hidden="true"/></RouterLink></div><div v-if="system.status" class="dependency-list"><div v-for="dep in system.status.dependencies" :key="dep.id" class="dependency-row"><div class="dependency-name"><span class="service-monogram" aria-hidden="true">{{dep.id==='postgres'?'PG':dep.id==='redis'?'R':dep.id==='emby'?'E':'CD'}}</span><div><strong>{{dep.name}}</strong><p>{{dep.message}}</p></div></div><StatusBadge :state="dep.state"/></div></div><el-skeleton v-else :rows="4" animated/><div class="panel-bottom">外部服务“已配置”不等于“已通过连接验证”。</div></section>
  <section class="panel recent-jobs"><div class="panel-title"><h2>最近任务</h2><RouterLink to="/jobs" class="inline-link">查看队列 <ArrowRight aria-hidden="true"/></RouterLink></div><div v-if="jobs===null" class="empty-state compact"><span class="empty-ring" aria-hidden="true"></span><h3>持久化任务尚不可用</h3><p>{{jobsError||'正在查询数据库'}}</p><RouterLink to="/settings" class="inline-link">检查数据库配置</RouterLink></div><div v-else-if="jobs.length===0" class="empty-state compact"><span class="empty-ring" aria-hidden="true"></span><h3>队列中还没有任务</h3><p>数据库已可用。可以到任务队列创建一次系统检查，验证 Worker。</p><RouterLink to="/jobs" class="inline-link">运行系统检查</RouterLink></div><div v-else class="job-list"><div v-for="job in jobs.slice(0,5)" :key="job.id" class="job-row"><div><strong>{{job.kind==='system.check'?'系统检查':job.kind}}</strong><small>{{new Date(job.createdAt).toLocaleString('zh-CN')}}</small></div><StatusBadge :state="job.state"/></div></div></section>
 </div>
 <section class="panel module-progress"><div class="panel-title"><h2>模块建设进度</h2><span class="muted">{{implemented}} / {{system.modules.length}} 个模块已有基础实现</span></div><div class="module-table"><RouterLink v-for="module in system.modules" :key="module.id" :to="`/${module.id}`" class="module-progress-row"><strong>{{module.name}}</strong><span class="module-summary">{{module.summary}}</span><span class="milestone">{{module.milestone}}</span><StatusBadge :state="module.stage"/></RouterLink></div></section>
</template>
