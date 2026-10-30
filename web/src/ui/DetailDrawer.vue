<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    title: string;
    size?: "small" | "default" | "large";
  }>(),
  { size: "default" },
);

const visible = defineModel<boolean>("visible", { default: false });
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
  width: 100%;
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.detail-dialog .detail-body {
  font-size: 14px;
  line-height: 1.6;
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
