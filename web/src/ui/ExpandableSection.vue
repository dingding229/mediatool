<script setup lang="ts">
import {ref} from 'vue';
import {ArrowRight} from '@element-plus/icons-vue';
withDefaults(defineProps<{title:string;open?:boolean}>(),{open:false});
const expanded=ref(false);
</script>
<template>
 <details class="mt-expandable" :open="open" @toggle="expanded=($event.target as HTMLDetailsElement).open">
  <summary><el-icon :class="{'is-expanded':expanded}"><ArrowRight/></el-icon><span>{{title}}</span></summary>
  <div class="mt-expandable-content"><slot/></div>
 </details>
</template>
<style scoped>
.mt-expandable{min-width:0}.mt-expandable>summary{list-style:none;display:flex;align-items:center;gap:8px;min-height:44px;font-size:13px;font-weight:400;color:var(--mt-text-muted);cursor:pointer}.mt-expandable>summary::-webkit-details-marker{display:none}.mt-expandable>summary:focus-visible{outline:2px solid var(--el-color-primary);outline-offset:2px}.mt-expandable .el-icon{flex:none;transition:transform .15s}.is-expanded{transform:rotate(90deg)}.mt-expandable-content{min-width:0;overflow-wrap:anywhere}
</style>
