<script setup lang="ts">
import StateView from '../ui/StateView.vue';
import { sourceState } from '../media/model';
import { errorText } from './readiness';
defineProps<{
  loading?: boolean;
  stale?: boolean;
  error?: unknown;
  empty?: boolean;
  title?: string;
  emptyTitle?: string;
}>();
defineEmits<{
  reload: [
  ];
}>();
</script>
<template>
  <p v-if="stale && error" class="f3-stale-warning" role="status"><span>{{errorText(error)}}</span>；刷新失败，保留上次数据。<button @click="$emit('reload')">重试</button></p>
  <StateView v-else-if="loading" state="loading" :title="title||'正在读取…'"/>
  <StateView v-else-if="error" :state="sourceState(error)" title="暂时无法读取" :description="errorText(error)" retry-text="重新读取" @retry="$emit('reload')"/>
  <StateView v-else-if="empty" state="empty" :title="emptyTitle||'暂无记录'"/>
</template>

<style scoped>.f3-stale-warning{position:absolute;inset:8px 12px auto auto;z-index:2;max-width:75%;font-size:12px;background:var(--panel-bg,#fff);padding:4px 8px;border:1px solid var(--border);border-radius:6px}.f3-stale-warning button{color:var(--el-color-primary);border:0;background:none;min-height:32px}</style>
