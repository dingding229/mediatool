<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import InstanceDetail from '../plugin-workspace/InstanceDetail.vue';
const route = useRoute(), id = computed(() => String(route.params.id));
</script>
<template>
  <InstanceDetail :key="id"/>
</template>
