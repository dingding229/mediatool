<script setup lang="ts" generic="T extends string|number">
import { computed, ref, watch, nextTick } from 'vue';
import { filterChoices, type Choice } from '../classification/options';
const props = defineProps<{
  options: Choice<T>[];
  label: string;
}>(), selected = defineModel<T[]>({ required: true }), query = ref('');
const picker = ref<HTMLElement>();
watch(selected, async () => {
  if (typeof document === 'undefined')
    return;
  const input = document.activeElement;
  if (!(input instanceof HTMLInputElement) || !picker.value?.contains(input))
    return;
  await nextTick();
  if (input.isConnected)
    input.focus();
});
const visible = computed(() => filterChoices(props.options, selected.value, query.value));
const summary = computed(() => props.options.filter(x => selected.value.includes(x.value)).map(x => `${x.label} (${x.value})`).join('、'));
</script>
<template>
  <section ref="picker" class="rule-picker" :aria-label="label">
    <label :for="`rule-${label}`">{{label}}</label>
    <el-input :id="`rule-${label}`" v-model="query" :aria-label="`搜索${label}`" clearable :placeholder="`搜索${label}或代码`"/>
    <p role="status" aria-live="polite">已选：{{summary||'不限'}}</p>
    <div class="rule-options" role="group" :aria-label="`${label}选项`">
      <label v-for="item in visible" :key="item.value" class="rule-choice">
        <input v-model="selected" type="checkbox" :value="item.value"/>
        <span>
          {{item.label}}
          <small>{{item.value}}</small>
        </span>
      </label>
    </div>
    <p v-if="!visible.length">没有匹配项，请更换关键词。</p>
  </section>
</template>
