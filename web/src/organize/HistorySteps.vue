<script setup lang="ts">
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';import StatusBadge from '../components/StatusBadge.vue';import {historySteps,present} from './presentation';import {label} from './model';
defineProps<{detail:any}>();
</script>
<template><ol class="history-steps" aria-label="整理步骤列表"><li v-for="step in historySteps(detail)" :key="step.number"><span class="history-step-number">{{step.number}}</span><strong>{{step.action}}</strong><StatusBadge :state="step.state" :label="label(step.state)"/><div class="history-step-paths"><MiddleEllipsis v-if="present(step.source)" :path="step.source" full/><span v-if="present(step.source)&&present(step.target)" aria-hidden="true">→</span><MiddleEllipsis v-if="present(step.target)" :path="step.target" full/></div></li></ol></template>
