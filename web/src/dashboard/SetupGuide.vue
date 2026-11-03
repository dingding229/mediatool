<script setup lang="ts">
defineProps<{complete:boolean;steps:{label:string;done:boolean;note?:string;path:string;tab:string}[];uncertain?:boolean}>();
defineEmits<{dismiss:[]}>();
</script>
<template>
 <section class="setup-guide u2-surface" aria-label="首次使用引导">
  <div v-if="complete" class="setup-guide-complete" role="status"><span>初始配置已全部完成</span><el-button text @click="$emit('dismiss')">收起</el-button></div>
  <template v-else>
   <header><div><h2>完成配置，开始自动跟进</h2><p>{{steps.filter(s=>s.done).length}} / {{steps.length}} 项已完成。</p></div></header>
   <ol><li v-for="(step,index) in steps" :key="step.label" :class="{'is-incomplete':!step.done}"><span>{{step.done?'✓':index+1}}</span><div><strong>{{step.label}}</strong><small>{{step.note||(step.done?'已完成':'待完成或尚未取得有效数据')}}</small><RouterLink :to="{path:step.path,query:{tab:step.tab}}">{{step.done?'查看配置':'去配置'}}</RouterLink></div></li></ol>
   <p v-if="uncertain" role="alert">部分配置读取失败，完成度暂不确定；可刷新状态后再核对。</p>
  </template>
 </section>
</template>
<style scoped>.setup-guide-complete{display:flex;align-items:center;justify-content:space-between;gap:12px;font-size:13px;color:var(--mt-text-muted)}.setup-guide-complete .el-button{margin:0}</style>

<style scoped>
.setup-guide header{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:20px}.setup-guide p,.setup-guide small{font-size:12px;color:var(--mt-text-muted)}.setup-guide ol{list-style:none;padding:0;margin:0;display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.setup-guide li{display:flex;gap:12px;min-width:0}.setup-guide li>span{display:grid;place-items:center;width:28px;height:28px;flex-shrink:0;border-radius:50%;background:var(--mt-color-primary-light);color:var(--mt-color-primary)}.setup-guide small{display:block;margin-top:6px}.setup-guide a{display:inline-flex;align-items:center;min-height:44px;color:var(--mt-color-primary);font-size:13px}.setup-guide li.is-incomplete{border-left:3px solid var(--el-color-warning);padding-left:12px}@media(max-width:1100px){.setup-guide ol{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:600px){.setup-guide ol{grid-template-columns:1fr}}
</style>
