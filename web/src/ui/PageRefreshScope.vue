<script setup lang="ts">
import { providePageRefresh } from './page-refresh';
const props = defineProps<{
  defaultTab?: string;
}>();
providePageRefresh(props.defaultTab);
</script>
<template>
  <slot/>
</template>
