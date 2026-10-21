<script setup lang="ts">
import RefreshButton from './RefreshButton.vue';
import {useRefreshControl} from './page-refresh';
const refresh=useRefreshControl();
defineProps<{
  title: string
  kicker?: string
  description?: string
}>()
</script>

<template>
  <header class="page-heading">
    <div class="page-heading-main">
      <span v-if="kicker" class="page-kicker">{{ kicker }}</span>
      <h1>{{ title }}</h1>
      <p v-if="description" class="page-description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions||refresh" class="heading-actions">
      <slot name="actions" />
      <RefreshButton v-if="refresh&&refresh.active.value.length" :loading="refresh.busy.value" @refresh="refresh.refresh"/>
      <p v-if="refresh?.error.value" role="status">{{refresh.error.value}}</p>
    </div>
  </header>
</template>

<style scoped>
.page-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}
.page-heading-main {
  min-width: 0;
  flex: 1;
}
.page-kicker {
  display: block;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--mt-text-muted);
  margin-bottom: 4px;
}
.page-description {
  font-size: 13px;
  color: var(--mt-text-muted);
  margin-top: 4px;
}
.heading-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
