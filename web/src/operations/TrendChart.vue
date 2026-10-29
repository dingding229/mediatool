<script setup lang="ts">
import { computed } from 'vue';
import { trendSegments } from './model';
import {formatBytes} from '../utils/format-bytes';
import { date } from '../workflows/readiness';
import {formatTrendValue,trendPlot} from './trend-chart';
const props = withDefaults(defineProps<{
  title: string;
  unit: string;
  rows: any[];
  metric: string;
  gapMs: number;
  stretch?: boolean;
}>(),{stretch:false}), plot=computed(()=>trendPlot(props.rows,props.metric,props.gapMs,props.unit)), values = computed(() => props.stretch?plot.value.values:props.rows.map(r => r[props.metric]).filter(v => typeof v === 'number' && Number.isFinite(v))), lines = computed(() => trendSegments(props.rows, props.metric, props.gapMs));
function display(value?:number){if(props.stretch)return formatTrendValue(value,props.unit);if(value==null)return '—';if(props.unit==='MiB')return formatBytes(value*1024**2);if(props.unit==='GiB')return formatBytes(value*1024**3);return value.toFixed(2)+(props.unit?' '+props.unit:'');}
</script>
<template>
  <article class="u6-card u6-trend">
    <h3>{{ title }}<template v-if="unit||!stretch"> · {{ unit||'—' }}</template></h3>
    <p v-if="!values.length">暂无有效采样</p>
    <template v-else>
      <p>最近 {{ display(values.at(-1)) }} · 最低 {{ display(Math.min(...values)) }} · 最高 {{ display(Math.max(...values)) }}</p>
      <div v-if="stretch" class="trend-frame">
        <div class="trend-y-axis" aria-label="纵轴刻度"><span v-for="tick in plot.yTicks" :key="tick.position" :style="{top:tick.position+'%'}">{{ tick.label }}</span></div>
        <div class="trend-plot">
          <svg viewBox="0 0 320 105" preserveAspectRatio="none" role="img" :aria-label="title+'趋势'">
            <line v-for="tick in plot.yTicks" :key="tick.position" class="trend-grid-line" x1="8" x2="312" :y1="tick.position*1.05" :y2="tick.position*1.05" vector-effect="non-scaling-stroke"/>
            <polyline v-for="(points,i) in plot.segments" :key="i" :points="points" vector-effect="non-scaling-stroke"/>
          </svg>
          <span v-for="(point,i) in plot.dots" :key="i" class="trend-dot" :style="{left:point.x+'%',top:point.y+'%'}" aria-label="孤立采样点"/>
        </div>
        <div class="trend-x-axis" aria-label="时间刻度"><span v-for="(tick,i) in plot.xTicks" :key="i" :style="{left:tick.position+'%'}" :class="{'is-first':i===0,'is-last':i===plot.xTicks.length-1}">{{ tick.label }}</span></div>
      </div>
      <svg v-else viewBox="0 0 320 105" preserveAspectRatio="xMidYMid meet" role="img" :aria-label="title+'趋势'">
        <polyline v-for="(points,i) in lines" :key="i" :points="points"/>
        <circle v-for="point in lines.filter(p=>!p.includes(' '))" :key="point" :cx="point.split(',')[0]" :cy="point.split(',')[1]" r="3"/>
      </svg>
      <p>{{ date(rows[0]?.at) }} — {{ date(rows.at(-1)?.at) }}</p>
    </template>
  </article>
</template>
<style scoped>
/* Own the drawing styles so reuse outside the lazily loaded operations page
   still renders a visible chart. */
.u6-trend svg{display:block;width:100%;height:130px}.u6-trend polyline{fill:none;stroke:var(--mt-color-primary);stroke-width:2}.u6-trend circle{fill:var(--mt-color-primary)}
.trend-frame{display:grid;grid-template-columns:6.5rem minmax(0,1fr);grid-template-rows:160px 30px;column-gap:8px;margin-block:12px}
.trend-y-axis,.trend-plot,.trend-x-axis{position:relative;min-width:0}
.trend-y-axis span{position:absolute;right:0;transform:translateY(-50%);white-space:nowrap}
.trend-y-axis,.trend-x-axis{color:var(--mt-text-muted);font-size:12px;font-variant-numeric:tabular-nums;line-height:1.5}
.trend-plot svg{height:100%}.trend-grid-line{stroke:var(--mt-text-muted);stroke-width:1;stroke-dasharray:3 5}
.trend-dot{position:absolute;width:6px;height:6px;border-radius:50%;background:var(--mt-color-primary);transform:translate(-50%,-50%);pointer-events:none}
.trend-x-axis{grid-column:2}.trend-x-axis span{position:absolute;top:0;transform:translateX(-50%);white-space:nowrap}
.trend-x-axis .is-first{transform:none}.trend-x-axis .is-last{transform:translateX(-100%)}
@media(max-width:700px){.trend-frame{grid-template-columns:5.5rem minmax(0,1fr);column-gap:4px}.trend-x-axis span{white-space:normal;width:4.5rem;text-align:center}.trend-x-axis .is-first{text-align:left}.trend-x-axis .is-last{text-align:right}}
</style>
