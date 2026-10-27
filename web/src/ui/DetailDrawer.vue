<script setup lang="ts">
import { computed, ref } from "vue";

const props = withDefaults(
  defineProps<{
    title: string;
    technicalData?: Record<string, any> | unknown;
    size?: "small" | "default" | "large";
  }>(),
  { size: "default" },
);

const visible = defineModel<boolean>("visible", { default: false });
const showTechnical = ref(false);
const dialogWidth = computed(
  () => ({ small: "560px", default: "800px", large: "1000px" })[props.size],
);
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    :width="dialogWidth"
    align-center
    :close-on-click-modal="false"
    :close-on-press-escape="true"
    :show-close="true"
    class="detail-drawer detail-dialog"
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
          <span>{{ showTechnical ? "收起 ▲" : "展开 ▼" }}</span>
        </button>

        <div v-if="showTechnical" class="technical-content">
          <pre
            class="technical-json"
          ><code>{{ JSON.stringify(technicalData, null, 2) }}</code></pre>
        </div>
      </div>
    </div>

    <template v-if="$slots.footer" #footer>
      <div class="detail-footer">
        <slot name="footer" />
      </div>
    </template>
  </el-dialog>
</template>

<style>
/* Dialog content is teleported: scope these rules by our unique class, not a parent scope attribute. */
.el-dialog.detail-dialog {
  max-width: calc(100vw - 32px);
  max-height: 85vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.el-dialog.detail-dialog.is-align-center { margin: auto; }
.detail-dialog > .el-dialog__header,
.detail-dialog > .el-dialog__footer { flex: 0 0 auto; }
.detail-dialog .el-dialog__headerbtn { min-width: 44px; min-height: 44px; }
.detail-dialog > .el-dialog__body {
  min-height: 0;
  flex: 1 1 auto;
  overflow: auto;
}
.detail-dialog .detail-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.detail-dialog .detail-body {
  min-height: min(360px, 60vh);
  font-size: 14px;
  line-height: 1.6;
}
.detail-dialog .technical-section {
  margin-top: 24px;
  border-top: 1px dashed var(--border);
  padding-top: 16px;
}
.detail-dialog .technical-toggle {
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
.detail-dialog .technical-toggle:hover { color: var(--text); border-color: var(--border-strong); }
.detail-dialog .technical-content { margin-top: 12px; }
.detail-dialog .technical-json {
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
@media (max-width: 599.98px) {
  .el-dialog.detail-dialog {
    width: 100vw;
    max-width: 100vw;
    height: 100vh;
    height: 100dvh;
    max-height: 100vh;
    max-height: 100dvh;
    border-radius: 0;
  }
  .el-dialog.detail-dialog.is-align-center { margin: 0; }
  .detail-dialog > .el-dialog__body,
  .detail-dialog > .el-dialog__footer { padding-inline: 16px; }
}
</style>
