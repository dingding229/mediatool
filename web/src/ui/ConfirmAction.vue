<script setup lang="ts">
import { ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  impactScope?: string
  dangerLevel?: 'warning' | 'danger'
  confirmText?: string
  cancelText?: string
  confirmPhrase?: string
  loading?: boolean
}>(), {
  dangerLevel: 'warning',
  confirmText: '确认执行',
  cancelText: '取消'
})

const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const inputPhrase = ref('')

watch(visible, val => {
  if (val) inputPhrase.value = ''
})

function onConfirm() {
  if (props.confirmPhrase && inputPhrase.value !== props.confirmPhrase) return
  emit('confirm')
}

function onCancel() {
  visible.value = false
  emit('cancel')
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="min(560px, 94vw)"
    :close-on-click-modal="!loading"
    :close-on-press-escape="!loading"
    :show-close="!loading"
    class="confirm-action-dialog"
  >
    <div class="confirm-content">
      <p v-if="description" class="confirm-desc">{{ description }}</p>

      <div v-if="impactScope" class="impact-scope-box" :class="`impact-${dangerLevel}`">
        <strong class="impact-title">⚠️ 操作影响范围：</strong>
        <p class="impact-text">{{ impactScope }}</p>
      </div>

      <div v-if="confirmPhrase" class="confirm-phrase-box">
        <label class="phrase-label">
          为防止误操作，请输入确认短语 <code>{{ confirmPhrase }}</code>：
        </label>
        <el-input
          v-model="inputPhrase"
          placeholder="请输入上方确认短语"
          :disabled="loading"
          autocomplete="off"
        />
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button :disabled="loading" @click="onCancel">{{ cancelText }}</el-button>
        <el-button
          :type="dangerLevel === 'danger' ? 'danger' : 'primary'"
          :loading="loading"
          :disabled="!!confirmPhrase && inputPhrase !== confirmPhrase"
          @click="onConfirm"
        >
          {{ confirmText }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.confirm-content {
  font-size: 14px;
  line-height: 1.6;
}
.confirm-desc {
  color: var(--text);
  margin-bottom: 16px;
}
.impact-scope-box {
  padding: 12px 16px;
  border-radius: var(--radius-sm);
  margin-bottom: 16px;
}
.impact-warning {
  background: var(--el-color-warning-light-9, #fdf6ec);
  border: 1px solid var(--warning);
  color: var(--warning);
}
.impact-danger {
  background: var(--el-color-danger-light-9, #fef0f0);
  border: 1px solid var(--danger);
  color: var(--danger);
}
.impact-title {
  display: block;
  font-size: 13px;
  margin-bottom: 4px;
}
.impact-text {
  font-size: 13px;
  margin: 0;
  overflow-wrap: anywhere;
}
.confirm-phrase-box {
  margin-top: 16px;
}
.phrase-label {
  display: block;
  font-size: 13px;
  color: var(--muted);
  margin-bottom: 8px;
}
.phrase-label code {
  font-weight: 600;
  color: var(--text);
  background: var(--border);
  padding: 2px 6px;
  border-radius: 4px;
}
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
