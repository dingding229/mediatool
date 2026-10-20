<script setup lang="ts">
export type StepStatus = 'preview' | 'review' | 'execute' | 'verify'

withDefaults(defineProps<{
  currentStep: number // 1: 预览, 2: 核对, 3: 执行, 4: 核验
  steps?: Array<{ title: string; description?: string }>
}>(), {
  steps: () => [
    { title: '生成预览', description: '计算变更内容' },
    { title: '人工核对', description: '确认影响范围' },
    { title: '执行操作', description: '后台受控写入' },
    { title: '对账核验', description: '验证最终状态' }
  ]
})
</script>

<template>
  <div class="preview-approve-workflow panel">
    <div class="steps-bar">
      <div
        v-for="(step, index) in steps"
        :key="index"
        class="step-item"
        :class="{
          'is-active': currentStep === index + 1,
          'is-done': currentStep > index + 1
        }"
      >
        <div class="step-badge">{{ currentStep > index + 1 ? '✓' : index + 1 }}</div>
        <div class="step-text">
          <strong class="step-title">{{ step.title }}</strong>
          <span v-if="step.description" class="step-desc">{{ step.description }}</span>
        </div>
        <div v-if="index < steps.length - 1" class="step-divider" aria-hidden="true" />
      </div>
    </div>

    <div class="step-content">
      <slot :current-step="currentStep" />
    </div>

    <div v-if="$slots.footer" class="step-footer">
      <slot name="footer" :current-step="currentStep" />
    </div>
  </div>
</template>

<style scoped>
.preview-approve-workflow {
  padding: 24px;
}
.steps-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
  gap: 16px;
}
.step-item {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 140px;
}
.step-badge {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--border);
  color: var(--muted);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
  flex-shrink: 0;
}
.step-text {
  display: flex;
  flex-direction: column;
}
.step-title {
  font-size: 13px;
  color: var(--muted);
}
.step-desc {
  font-size: 11px;
  color: var(--muted);
}
.step-item.is-active .step-badge {
  background: var(--mt-color-primary);
  color: #fff;
}
.step-item.is-active .step-title {
  color: var(--mt-color-primary);
  font-weight: 600;
}
.step-item.is-done .step-badge {
  background: var(--mt-color-primary-light);
  color: var(--mt-color-primary);
}
.step-item.is-done .step-title {
  color: var(--text);
}
.step-divider {
  flex: 1;
  height: 2px;
  background: var(--border);
  margin: 0 8px;
  min-width: 20px;
}
.step-content {
  min-height: 120px;
}
.step-footer {
  margin-top: 24px;
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  border-top: 1px solid var(--border);
  padding-top: 16px;
}
</style>
