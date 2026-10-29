<script setup lang="ts">
import StatusBadge from '../components/StatusBadge.vue';
import {beijingTime} from '../presentation/time';
withDefaults(defineProps<{state:string;checkedAt?:string;message?:string}>(),{message:''});
const label=(state:string)=>state==='ready'?'连接成功':state==='disabled'?'已停用':'连接失败';
</script>
<template>
<div class="connection-health" aria-label="连接检测结果" aria-live="polite">
 <StatusBadge :state="state" :label="label(state)"/>
 <p>最近检测：{{checkedAt?beijingTime(checkedAt):'—'}}</p>
 <p v-if="state!=='ready'&&state!=='disabled'">{{message||'尚无有效检测记录，已安排检测'}}</p>
</div>
</template>
<style scoped>
.connection-health p{font-size:12px;color:var(--mt-text-muted);line-height:1.6;margin:4px 0;overflow-wrap:anywhere}
</style>
