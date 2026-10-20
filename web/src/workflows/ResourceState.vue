<script setup lang="ts">
import StateView from '../ui/StateView.vue';
import { sourceState } from '../media/model';
import { errorText } from './readiness';
defineProps<{
  loading?: boolean;
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
  <StateView v-if="loading" state="loading" :title="title||'正在读取…'"/>
  <StateView v-else-if="error" :state="sourceState(error)" title="暂时无法读取" :description="errorText(error)" retry-text="重新读取" @retry="$emit('reload')"/>
  <StateView v-else-if="empty" state="empty" :title="emptyTitle||'暂无记录'"/>
</template>
