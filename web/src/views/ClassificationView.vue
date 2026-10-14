<script setup lang="ts">
import { ref, onMounted } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import { request, post } from "../api/client";
import {
  Plus,
  Edit,
  Delete,
  ArrowUp,
  ArrowDown,
} from "@element-plus/icons-vue";
interface Rule {
  id: string;
  name: string;
  directory: string;
  enabled: boolean;
  genreIds: number[];
  countries: string[];
  languages: string[];
}
interface Group {
  id: string;
  name: string;
  directory: string;
  mediaType: string;
  genreIds: number[];
  enabled: boolean;
  rules: Rule[];
}
const record = ref<{ value: { groups: Group[] }; revision: number }>(),
  busy = ref(false),
  error = ref(""),
  dialog = ref(false),
  groupIndex = ref(-1),
  ruleIndex = ref(-1),
  isGroup = ref(false),
  form = ref({
    name: "",
    directory: "",
    mediaType: "movie",
    enabled: true,
    genres: "",
    countries: "",
    languages: "",
  });
const genreNames: Record<number, string> = {
  16: "动画",
  99: "纪录片",
  10402: "音乐",
  10762: "儿童",
  10764: "真人秀",
  10767: "脱口秀",
};
async function load() {
  busy.value = true;
  try {
    record.value = await request("/classification");
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
function edit(g: number, r = -1) {
  groupIndex.value = g;
  ruleIndex.value = r;
  isGroup.value = r === -1;
  const x =
    r === -1
      ? record.value?.value.groups[g]
      : record.value?.value.groups[g]?.rules[r];
  form.value = {
    name: x?.name ?? "",
    directory: x?.directory ?? "",
    mediaType: (x as Group)?.mediaType ?? "movie",
    enabled: x?.enabled ?? true,
    genres: x?.genreIds?.join(",") ?? "",
    countries: (x as Rule)?.countries?.join(",") ?? "",
    languages: (x as Rule)?.languages?.join(",") ?? "",
  };
  dialog.value = true;
}
function addRule(g: number) {
  edit(g, -2);
  isGroup.value = false;
  form.value = {
    name: "",
    directory: "",
    mediaType: "movie",
    enabled: true,
    genres: "",
    countries: "",
    languages: "",
  };
}
function apply() {
  if (!record.value) return;
  const x = {
    id: `rule-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`,
    name: form.value.name,
    directory: form.value.directory || form.value.name,
    enabled: form.value.enabled,
    genreIds: form.value.genres
      .split(/[\s,]+/)
      .filter(Boolean)
      .map(Number),
  };
  if (isGroup.value) {
    const g = { ...x, mediaType: form.value.mediaType, rules: [] };
    if (groupIndex.value < 0) record.value.value.groups.push(g);
    else {
      const old = record.value.value.groups[groupIndex.value]!;
      record.value.value.groups[groupIndex.value] = {
        ...g,
        id: old.id,
        rules: old.rules,
      };
    }
  } else {
    const rows = record.value.value.groups[groupIndex.value]!.rules;
    const r = {
      ...x,
      countries: form.value.countries
        .toUpperCase()
        .split(/[\s,]+/)
        .filter(Boolean),
      languages: form.value.languages
        .toLowerCase()
        .split(/[\s,]+/)
        .filter(Boolean),
    };
    if (ruleIndex.value < 0) rows.push(r);
    else rows[ruleIndex.value] = { ...r, id: rows[ruleIndex.value]!.id };
  }
  dialog.value = false;
}
function move(g: number, r: number, delta: number) {
  if (!record.value) return;
  const rows =
    r < 0 ? record.value.value.groups : record.value.value.groups[g]!.rules;
  const index = r < 0 ? g : r;
  if (index + delta < 0 || index + delta >= rows.length) return;
  const a = rows[index]!;
  rows.splice(index, 1);
  rows.splice(index + delta, 0, a as Group & Rule);
}
async function remove(g: number, r = -1) {
  try {
    await ElMessageBox.confirm(
      "仅删除分类规则，不移动或删除已有媒体。保存后新布局使用修改后的分类。",
      "删除分类",
      { type: "warning" },
    );
    if (r < 0) record.value?.value.groups.splice(g, 1);
    else record.value?.value.groups[g]?.rules.splice(r, 1);
  } catch {}
}
async function save() {
  if (!record.value) return;
  busy.value = true;
  error.value = "";
  try {
    record.value = await request("/classification", {
      method: "PUT",
      body: JSON.stringify(record.value),
    });
    ElMessage.success("目录分类已保存，仅影响新预览及新上传");
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ORGANIZE / 分类规则</span>
      <h1>目录分类</h1>
      <p>
        有序分类组与子规则。类型和不同字段同时满足，同字段多值任一满足；首次匹配优先。
      </p>
    </div>
    <div class="heading-actions">
      <el-button @click="load">重新读取</el-button
      ><el-button @click="edit(-1)"><Plus />添加分类组</el-button
      ><el-button type="primary" :loading="busy" @click="save"
        >保存规则</el-button
      >
    </div>
  </section>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <p class="field-hint">
    模板路径前加分类目录，国家缺失不猜测。修改规则不会自动整理或迁移已有文件。
  </p>
  <div class="classification-grid">
    <section
      v-for="(g, gi) in record?.value.groups"
      :key="g.id"
      class="panel category-group"
    >
      <header>
        <div>
          <h2>{{ g.name }}</h2>
          <small
            >{{ g.mediaType === "movie" ? "电影" : "剧集" }} ·
            {{ g.directory }} · {{ g.rules.length }}个子分类</small
          >
        </div>
        <div class="rule-actions">
          <el-button :aria-label="`向上移动${g.name}`" @click="move(gi, -1, -1)"
            ><ArrowUp /></el-button
          ><el-button :aria-label="`向下移动${g.name}`" @click="move(gi, -1, 1)"
            ><ArrowDown /></el-button
          ><el-button :aria-label="`编辑${g.name}`" @click="edit(gi)"
            ><Edit /></el-button
          ><el-button :aria-label="`删除${g.name}`" @click="remove(gi)"
            ><Delete /></el-button
          ><el-button :aria-label="`为${g.name}添加规则`" @click="addRule(gi)"
            ><Plus
          /></el-button>
        </div>
      </header>
      <el-tag v-if="!g.enabled" type="info">分类组已停用</el-tag>
      <article v-for="(r, ri) in g.rules" :key="r.id" class="category-rule">
        <div class="rule-title">
          <strong
            ><span>{{ ri + 1 }}</span
            >{{ r.name }}</strong
          ><el-tag v-if="!r.enabled" type="info">已停用</el-tag>
        </div>
        <p>
          <el-tag v-for="id in r.genreIds" :key="id" size="small">{{
            genreNames[id] ?? id
          }}</el-tag
          ><el-tag v-for="c in r.countries" :key="c" size="small" type="info">{{
            c
          }}</el-tag
          ><el-tag
            v-for="l in r.languages"
            :key="l"
            size="small"
            type="success"
            >{{ l }}</el-tag
          ><span
            v-if="
              !(r.genreIds?.length??0) && !(r.countries?.length??0) && !(r.languages?.length??0)
            "
            class="muted"
            >兜底 / 所有题材</span
          >
        </p>
        <code>{{ g.directory }}/{{ r.directory }}</code>
        <div class="rule-actions">
          <el-button @click="edit(gi, ri)"><Edit />编辑</el-button
          ><el-button
            :aria-label="`向上移动${r.name}`"
            @click="move(gi, ri, -1)"
            ><ArrowUp /></el-button
          ><el-button :aria-label="`向下移动${r.name}`" @click="move(gi, ri, 1)"
            ><ArrowDown /></el-button
          ><el-button :aria-label="`删除${r.name}`" @click="remove(gi, ri)"
            ><Delete
          /></el-button>
        </div>
      </article>
    </section>
  </div>
  <el-dialog
    v-model="dialog"
    :title="isGroup ? '分类组设置' : '子分类规则'"
    width="min(620px,94vw)"
    ><el-form label-position="top"
      ><el-form-item label="显示名称"
        ><el-input v-model="form.name" aria-label="分类名称" /></el-form-item
      ><el-form-item label="相对目录"
        ><el-input
          v-model="form.directory"
          aria-label="分类目录"
          placeholder="留空使用名称" /></el-form-item
      ><el-form-item v-if="isGroup" label="媒体类型"
        ><el-select v-model="form.mediaType"
          ><el-option label="电影" value="movie" /><el-option
            label="剧集"
            value="series" /></el-select></el-form-item
      ><el-form-item label="题材ID（逗号分隔）"
        ><el-input v-model="form.genres" /><small
          >16动画、99纪录片、10402音乐、10762儿童、10764真人秀、10767脱口秀</small
        ></el-form-item
      ><template v-if="!isGroup"
        ><el-form-item label="国家/地区代码"
          ><el-input
            v-model="form.countries"
            placeholder="CN,HK,TW,JP,US" /></el-form-item
        ><el-form-item label="原语言代码"
          ><el-input
            v-model="form.languages"
            placeholder="zh,cn,ja,en" /></el-form-item></template
      ><el-form-item label="启用规则"
        ><el-switch v-model="form.enabled" /></el-form-item></el-form
    ><template #footer
      ><el-button @click="dialog = false">取消</el-button
      ><el-button type="primary" @click="apply"
        >应用到待保存规则</el-button
      ></template
    ></el-dialog
  >
</template>
<style scoped>
.classification-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
}
.category-group {
  min-width: 0;
}
.category-group header {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 20px;
}
.category-group h2 {
  margin: 0 0 8px;
}
.category-rule {
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 16px;
  margin: 12px 0;
}
.rule-title {
  display: flex;
  justify-content: space-between;
}
.rule-title span {
  margin-right: 8px;
  color: var(--el-color-primary);
}
.category-rule p {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.category-rule code {
  overflow-wrap: anywhere;
  display: block;
  margin: 12px 0;
}
.rule-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.rule-actions .el-button {
  margin: 0;
  min-height: 44px;
}
.rule-actions svg {
  width: 16px;
}
.heading-actions svg {
  width: 16px;
}
@media (max-width: 1200px) {
  .classification-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 760px) {
  .classification-grid {
    grid-template-columns: 1fr;
  }
}
</style>
