<script setup lang="ts">
import { computed } from 'vue'
import { useSystemStore } from '../stores/system'
import { Connection } from '@element-plus/icons-vue'
import StatusBadge from '../components/StatusBadge.vue'
const props=defineProps<{moduleId:string}>(),system=useSystemStore()
const module=computed(()=>system.modules.find(m=>m.id===props.moduleId))
</script>
<template><section class="page-heading"><div><h1>{{module?.name??'业务模块'}}</h1><p>{{module?.summary??'正在读取模块定义'}}</p></div><StatusBadge :state="module?.stage??'planned'"/></section><section class="module-detail panel"><div class="module-detail-intro"><Connection aria-hidden="true"/><h2>{{module?.stage==='partial'?'基础已建立，业务链路待完善':'接口边界已预留'}}</h2><p>当前为工程框架，不展示模拟记录，也不会将尚未接入的服务标记为可用。</p><span class="phase-label">实施阶段：{{module?.milestone??'待确认'}}</span></div><div class="module-detail-scope"><h3>该模块的开发范围</h3><ul><li v-for="feature in module?.features??[]" :key="feature">{{feature}}</li></ul><p class="muted">详细实施计划位于项目文档 docs/DEVELOPMENT_PLAN.md。</p></div></section><div v-if="moduleId==='emby'" class="note-panel"><strong>当前 Emby 能力边界</strong><p>已实现最小静态流拦截：使用调用者身份查询 Emby 权限，映射本程序生成的 STRM，再返回 302 或受控代理。转码、HLS 和未识别媒体仍走原 Emby 链路；真实客户端兼容矩阵尚未验证。</p></div></template>
