<script setup lang="ts">
import { ref } from 'vue'

defineProps<{
  title: string
  technicalData?: Record<string, any> | unknown
}>()

const visible = defineModel<boolean>('visible', { default: false })
const showTechnical = ref(false)
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="title"
    size="min(600px, 94vw)"
    direction="rtl"
    class="detail-drawer"
  >
    <div class="detail-body">
      <slot />

      <div v-if="technicalData" class="technical-section">
        <button
          type="button"
          class="technical-toggle"
          :aria-expanded="showTechnical"
          @click="showTechnical = !showTechnical"
        >
          <span>技术细节与原始记录</span>
          <span>{{ showTechnical ? '收起 ▲' : '展开 ▼' }}</span>
        </button>

        <div v-if="showTechnical" class="technical-content">
          <pre class="technical-json"><code>{{ JSON.stringify(technicalData, null, 2) }}</code></pre>
        </div>
      </div>
    </div>

    <template #footer>
      <slot name="footer">
        <el-button @click="visible = false">关闭</el-button>
      </slot>
    </template>
  </el-drawer>
</template>

<style scoped>
.detail-body {
  font-size: 14px;
  line-height: 1.6;
}
.technical-section {
  margin-top: 24px;
  border-top: 1px dashed var(--border);
  padding-top: 16px;
}
.technical-toggle {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  background: var(--mt-bg-canvas);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--muted);
  font-size: 13px;
  cursor: pointer;
}
.technical-toggle:hover {
  color: var(--text);
  border-color: var(--border-strong);
}
.technical-content {
  margin-top: 12px;
}
.technical-json {
  background: var(--mt-bg-canvas);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 12px;
  max-height: 360px;
  overflow: auto;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
