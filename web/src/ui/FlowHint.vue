<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  nextAction?: string
  reason?: string
  clickable?: boolean
}>()

const emit = defineEmits<{
  (e: 'action', nextAction: string): void
}>()

const actionLabels: Record<string, string> = {
  reconcile_upload: '去核对上传对账',
  reconcile_transfer: '去核对传输结果',
  verify_player_302: '在播放器中验证播放',
  authorize_115: '去完成 115 扫码授权',
  enable_connection: '去启用该服务连接',
  configure_credentials: '去完善服务账号密码',
  test_connection: '测试连接连通性',
  inspect_connection: '查看连接故障原因',
  review_plugin_permissions: '审核插件新增权限',
  retry_stage: '重试当前阶段'
}

const displayAction = computed(() => {
  if (!props.nextAction) return ''
  return actionLabels[props.nextAction] || (/^[a-z][a-z0-9_.:-]*$/i.test(props.nextAction)?'查看处理建议':props.nextAction)
})

function onClick() {
  if (props.clickable && props.nextAction) {
    emit('action', props.nextAction)
  }
}
</script>

<template>
  <div v-if="nextAction || reason" class="flow-hint">
    <span v-if="reason" class="flow-reason">{{ reason }}</span>
    <button
      v-if="nextAction"
      type="button"
      class="flow-action-btn"
      :class="{ 'is-clickable': clickable }"
      :disabled="!clickable"
      @click="onClick"
    >
      {{ displayAction }} →
    </button>
  </div>
</template>

<style scoped>
.flow-hint {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  line-height: 1.5;
}
.flow-reason {
  color: var(--mt-text-muted);
}
.flow-action-btn {
  background: transparent;
  border: 0;
  min-height:44px;
  padding: 0;
  color: var(--mt-color-primary);
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.flow-action-btn.is-clickable {
  cursor: pointer;
  text-decoration: underline;
}
.flow-action-btn:disabled {
  text-decoration: none;
  cursor: default;
}
</style>
