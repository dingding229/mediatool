<script setup lang="ts">
interface Selection {
  nfo?: boolean;
  poster?: boolean;
  backdrop?: boolean;
  logo?: boolean;
  thumb?: boolean;
}
export interface Policy {
  movie: Selection;
  series: Selection;
  season: Selection;
  episode: Selection;
}
const props = defineProps<{ modelValue: Policy }>(),
  emit = defineEmits<{ "update:modelValue": [Policy] }>();
const groups: [keyof Policy, string, (keyof Selection)[]][] = [
  ["movie", "电影刮削项", ["nfo", "poster", "backdrop", "logo"]],
  ["series", "剧集刮削项", ["nfo", "poster", "backdrop", "logo"]],
  ["season", "季刮削项", ["nfo", "poster"]],
  ["episode", "集刮削项", ["nfo", "thumb"]],
];
const labels: Record<string, string> = {
  nfo: "NFO（元数据）",
  poster: "poster（海报）",
  backdrop: "backdrop（背景）",
  logo: "logo",
  thumb: "thumb（缩略图）",
};
function update(level: keyof Policy, key: keyof Selection, value: boolean) {
  emit("update:modelValue", {
    ...props.modelValue,
    [level]: { ...props.modelValue[level], [key]: value },
  });
}
</script>
<template>
  <fieldset class="scraping-options">
    <legend>刮削输出设置</legend>
    <p class="field-hint">
      选项控制下一次明确确认的入库/身份修正。关闭不会自动删除旧文件；图像下载失败会阻止本次写入，TMDB未提供图像会显示未生成原因。
    </p>
    <section
      v-for="[level, title, keys] in groups"
      :key="level"
      class="scraping-group"
    >
      <h3>{{ title }}</h3>
      <div class="scraping-checks">
        <el-checkbox
          v-for="key in keys"
          :key="key"
          :model-value="!!modelValue[level]?.[key]"
          :aria-label="`${title} ${labels[key]}`"
          @change="update(level, key, !!$event)"
          >{{ labels[key] }}</el-checkbox
        >
      </div>
    </section>
  </fieldset>
</template>
<style scoped>
.scraping-options {
  border: 0;
  padding: 0;
  margin: 0 0 24px;
  min-width: 0;
}
.scraping-options legend {
  font-weight: 650;
  font-size: 14px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border);
  width: 100%;
}
.scraping-group {
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 20px;
  margin-top: 16px;
}
.scraping-group h3 {
  font-size: 14px;
  margin-bottom: 12px;
}
.scraping-checks {
  display: flex;
  gap: 8px 24px;
  flex-wrap: wrap;
}
.scraping-checks .el-checkbox {
  margin-right: 0;
  min-height: 44px;
}
</style>
