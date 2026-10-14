<script setup lang="ts">
import { computed } from 'vue'
const props=defineProps<{state:string;label?:string}>()
const labels:Record<string,string>={preview:'待批准',blocked:'冲突阻止',paused:'已暂停',cancelled:'已取消',needs_reconcile:'需读回核对',visible:'逐路径可见',applied:'已写入核对',partial:'部分完成',expired:'预览过期',accessible:'可读取',cached:'缓存可用',ready:'连接正常',configured:'已配置 · 未验证',not_configured:'未配置',not_implemented:'待实现',unavailable:'不可用',disabled:'未启用',planned:'待开发',foundation:'框架可用',pending:'等待执行',running:'执行中',retry:'等待重试',succeeded:'已完成',failed:'失败',needs_refresh:'需要重新评估'}
const tone=computed(()=>['ready','foundation','succeeded','visible','applied','accessible'].includes(props.state)?'good':['unavailable','failed','needs_refresh'].includes(props.state)?'bad':['partial','retry','configured'].includes(props.state)?'caution':['running'].includes(props.state)?'active':'neutral')
</script>
<template><span class="status-badge" :class="tone"><span class="status-dot" aria-hidden="true"></span>{{label??labels[state]??state}}</span></template>
