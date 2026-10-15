<script setup lang="ts">
import { computed } from "vue";
const props = defineProps<{
  title: string;
  unit: string;
  rows: Record<string, unknown>[];
  metric: string;
  gapMs?: number;
}>();
const values = computed(() =>
  props.rows.flatMap((r, i) =>
    typeof r[props.metric] === "number" && Number.isFinite(r[props.metric])
      ? [{ i, value: r[props.metric] as number, at: Date.parse(String(r.at)) }]
      : [],
  ),
);
const range = computed(() => {
  const v = values.value.map((x) => x.value);
  return {
    min: Math.min(0, ...v),
    max: Math.max(0, ...v),
    ceiling: Math.max(1, ...v),
  };
});
const lines = computed(() => {
  const groups: string[][] = [];
  let last = -2,
    lastTime = 0;
  const start = Date.parse(String(props.rows[0]?.at)),
    end = Date.parse(String(props.rows.at(-1)?.at));
  for (const v of values.value) {
    if (
      v.i !== last + 1 ||
      (lastTime > 0 && v.at - lastTime > (props.gapMs ?? 60000) * 1.5)
    )
      groups.push([]);
    const x = 8 + (end > start ? (v.at - start) / (end - start) : 0) * 304;
    const y =
      92 -
      ((v.value - range.value.min) / (range.value.ceiling - range.value.min)) *
        80;
    groups.at(-1)!.push(`${x},${y}`);
    last = v.i;
    lastTime = v.at;
  }
  return groups.map((g) => g.join(" "));
});
const timeLabel = (row: Record<string, unknown> | undefined) => {
  const at = Date.parse(String(row?.at));
  return Number.isFinite(at)
    ? new Date(at).toLocaleString("zh-CN", {
        timeZone: "Asia/Shanghai",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";
};
</script>
<template>
  <article class="metric-trend">
    <h3>
      {{ title }} <small>{{ unit }}</small>
    </h3>
    <p v-if="!values.length" class="muted">暂无有效采样</p>
    <template v-else
      ><svg
        viewBox="0 0 320 105"
        role="img"
        :aria-label="`${title}趋势，${values.length}个有效采样，最大${range.max.toFixed(2)}${unit}`"
      >
        <line x1="8" y1="92" x2="312" y2="92" />
        <polyline
          v-for="(points, index) in lines"
          :key="index"
          :points="points"
        />
        <circle
          v-for="point in lines.filter((line) => !line.includes(' '))"
          :key="point"
          :cx="Number(point.split(',')[0])"
          :cy="Number(point.split(',')[1])"
          r="3"
        />
      </svg>
      <div class="trend-time">
        <time>{{ timeLabel(rows[0]) }}</time
        ><time>{{ timeLabel(rows.at(-1)) }}</time>
      </div>
      <p>
        最近 {{ values.at(-1)?.value.toFixed(2) }} {{ unit }} · 最大
        {{ range.max.toFixed(2) }} {{ unit }}
      </p></template
    >
  </article>
</template>
<style scoped>
.metric-trend {
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  min-width: 0;
}
.metric-trend h3 {
  margin: 0;
  font-size: 14px;
}
.trend-time {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  color: var(--muted);
  font-size: 11px;
}
.metric-trend small,
.metric-trend p {
  font-size: 12px;
  color: var(--muted);
}
svg {
  width: 100%;
  height: 105px;
  display: block;
}
line {
  stroke: var(--border);
}
polyline {
  fill: none;
  stroke: var(--el-color-primary);
  stroke-width: 2;
  stroke-linejoin: round;
}
circle {
  fill: var(--el-color-primary);
}
</style>
