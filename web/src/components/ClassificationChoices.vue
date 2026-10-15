<script setup lang="ts" generic="T extends string | number">
import { computed, nextTick, ref, watch } from "vue";
import { Search } from "@element-plus/icons-vue";
import { filterChoices, type Choice } from "../classification/options";

const props = defineProps<{
  options: Choice<T>[];
  label: string;
  searchLabel: string;
  placeholder: string;
}>();
const selected = defineModel<T[]>({ required: true });
const query = ref("");
const picker = ref<HTMLElement>();
const visible = computed(() => filterChoices(props.options, selected.value, query.value));
const summary = computed(() =>
  filterChoices(props.options, selected.value)
    .filter((option) => selected.value.includes(option.value))
    .map((option) => `${option.label} (${option.value})`)
    .join("、"),
);
// Capture focus before Vue moves keyed nodes. A change handler is too late:
// the native v-model listener may already have scheduled the reordering.
watch(selected, async () => {
  const input = document.activeElement;
  if (!(input instanceof HTMLInputElement) || input.type !== "checkbox" || !picker.value?.contains(input)) return;
  await nextTick();
  if (input.isConnected) input.focus();
});
</script>

<template>
  <div ref="picker" class="classification-choices" :data-choice="label">
    <el-input
      v-model="query"
      :aria-label="searchLabel"
      :placeholder="placeholder"
      :prefix-icon="Search"
      clearable
    />
    <p class="selection-summary" aria-live="polite">
      已选{{ selected.length ? ` ${selected.length} 项` : "" }}：{{ summary || "不限" }}
    </p>
    <div class="choice-options" role="group" :aria-label="`${label}选项`" tabindex="0">
      <label
        v-for="option in visible"
        :key="option.value"
        class="choice-option"
        :class="{ 'is-selected': selected.includes(option.value) }"
      >
        <input v-model="selected" type="checkbox" :value="option.value" />
        <span class="choice-label">{{ option.label }} <small>{{ option.value }}</small></span>
      </label>
      <p v-if="!visible.length" class="choice-empty" role="status">
        没有匹配的{{ label }}，请更换关键词或清空搜索。
      </p>
    </div>
  </div>
</template>

<style scoped>
.classification-choices {
  width: 100%;
  min-width: 0;
}
.selection-summary {
  margin: 8px 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.6;
  white-space: normal;
  overflow-wrap: anywhere;
}
.choice-options {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-items: stretch;
  gap: 4px 8px;
  max-height: 240px;
  overflow-y: auto;
  scrollbar-gutter: stable;
  padding: 2px;
  border-radius: 4px;
}
.choice-option {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr);
  align-items: start;
  gap: 7px;
  min-width: 0;
  min-height: 44px;
  box-sizing: border-box;
  padding: 8px 6px;
  border-radius: 4px;
  font-size: 13px;
  line-height: 1.5;
  cursor: pointer;
}
.choice-option:hover,
.choice-option.is-selected {
  background: var(--el-color-primary-light-9);
}
.choice-option:focus-within,
.choice-options:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: -2px;
}
.choice-option input {
  accent-color: var(--el-color-primary);
  width: 16px;
  height: 16px;
  margin: 2px 0 0;
}
.choice-label {
  min-width: 0;
  white-space: normal;
  overflow-wrap: anywhere;
}
.choice-label small {
  display: inline-block;
  color: var(--muted);
  font-size: 12px;
  white-space: nowrap;
}
.choice-empty {
  grid-column: 1 / -1;
  margin: 8px 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.6;
}
@media (max-width: 760px) {
  .choice-options {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
