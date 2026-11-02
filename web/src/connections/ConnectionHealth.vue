<script setup lang="ts">
import {computed} from 'vue';
import {healthState,checkLabel,hasCheckTime} from './health';
import StatusBadge from '../components/StatusBadge.vue';
import {beijingTime} from '../presentation/time';
const props=withDefaults(defineProps<{state:string;checkedAt?:string;message?:string}>(),{message:''});
const effective=computed(()=>healthState(props.state,props.checkedAt,props.message));
</script>
<template>
<div class="connection-health" aria-label="连接检测结果" aria-live="polite">
 <StatusBadge :state="effective" :label="checkLabel(effective)"/>
 <p>最近检测：{{hasCheckTime(checkedAt)?beijingTime(checkedAt):'尚未检测'}}</p>
 <p v-if="effective!=='ready'&&effective!=='disabled'">{{message||'尚无有效检测记录，已安排检测'}}</p>
</div>
</template>
<style scoped>
.connection-health p{font-size:12px;color:var(--mt-text-muted);line-height:1.6;margin:4px 0;overflow-wrap:anywhere}
</style>
