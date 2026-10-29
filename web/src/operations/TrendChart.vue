<script setup lang="ts">
import { computed } from 'vue';
import { trendSegments } from './model';
import {formatBytes} from '../utils/format-bytes';
import { date } from '../workflows/readiness';
const props = withDefaults(defineProps<{
  title: string;
  unit: string;
  rows: any[];
  metric: string;
  gapMs: number;
  stretch?: boolean;
}>(),{stretch:false}), values = computed(() => props.rows.map(r => r[props.metric]).filter(v => typeof v === 'number' && Number.isFinite(v))), lines = computed(() => trendSegments(props.rows, props.metric, props.gapMs));
function display(value?:number){if(value==null)return '—';if(props.unit==='MiB')return formatBytes(value*1024**2);if(props.unit==='GiB')return formatBytes(value*1024**3);return value.toFixed(2)+(props.unit?' '+props.unit:'');}
</script>
<template>
  <article class="u6-card u6-trend">
    <h3>{{ title }} · {{ unit||'—' }}</h3>
    <p v-if="!values.length">暂无有效采样</p>
    <template v-else>
      <p>最近 {{ display(values.at(-1)) }} · 最低 {{ display(Math.min(...values)) }} · 最高 {{ display(Math.max(...values)) }}</p>
      <svg viewBox="0 0 320 105" :preserveAspectRatio="stretch?'none':'xMidYMid meet'" role="img" :aria-label="title+'趋势'">
        <polyline v-for="(points,i) in lines" :key="i" :points="points" :vector-effect="stretch?'non-scaling-stroke':undefined"/>
        <template v-for="point in lines.filter(p=>!p.includes(' '))" :key="point">
          <line v-if="stretch" :x1="point.split(',')[0]" :x2="point.split(',')[0]" :y1="point.split(',')[1]" :y2="point.split(',')[1]" vector-effect="non-scaling-stroke" stroke-linecap="round" stroke-width="6"/>
          <circle v-else :cx="point.split(',')[0]" :cy="point.split(',')[1]" r="3"/>
        </template>
      </svg>
      <p>{{ date(rows[0]?.at) }} — {{ date(rows.at(-1)?.at) }}</p>
    </template>
  </article>
</template>
<style scoped>
/* Own the drawing styles so reuse outside the lazily loaded operations page
   still renders a visible chart. */
.u6-trend svg{display:block;width:100%;height:130px}.u6-trend polyline{fill:none;stroke:var(--mt-color-primary);stroke-width:2}.u6-trend circle{fill:var(--mt-color-primary)}
.u6-trend line{stroke:var(--mt-color-primary)}
</style>
