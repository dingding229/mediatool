<script setup lang="ts">
import { computed } from 'vue'

export type AsyncStatus = 'queued' | 'reconciling' | 'locked'

const props = withDefaults(defineProps<{
  status: AsyncStatus
  message?: string
  ticketId?: string
  canRefresh?: boolean
}>(), {
  canRefresh: true
})

const emit = defineEmits<{
  (e: 'refresh'): void
}>()

const defaultMessages: Record<AsyncStatus, string> = {
  queued: '操作已排队，尚未确认完成。',
  reconciling: '上一操作结果正在向服务端核对，不会重复发起请求…',
  locked: '系统已为您锁定表单，正在等待操作完成…'
}

const displayMessage = computed(() => props.message || defaultMessages[props.status])
const bannerType = computed(() => props.status === 'reconciling' ? 'warning' : 'info')
</script>

<template>
  <el-alert
    :type="bannerType"
    :closable="false"
    show-icon
    class="async-result-banner"
  >
    <template #title>
      <div class="banner-title-box">
        <span>{{ displayMessage }}</span>
        <details v-if="ticketId"><summary>技术详情</summary><code>{{ ticketId }}</code></details>
        <el-button v-if="canRefresh" class="reconcile-button" @click="emit('refresh')">核对结果</el-button>

      </div>
    </template>
  </el-alert>
</template>

<style scoped>
.async-result-banner {
  margin-bottom: 16px;
}
.banner-title-box {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.reconcile-button { min-height:44px; }
.banner-ticket {
  font-size: 12px;
  color: var(--mt-text-muted);
}
.banner-ticket code {
  background: var(--mt-border-color);
  padding: 1px 4px;
  border-radius: 4px;
}
</style>
