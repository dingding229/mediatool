<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import { request } from "../api/client";
import {
  Plus,
  Edit,
  Delete,
  ArrowUp,
  ArrowDown,
} from "@element-plus/icons-vue";
import {
  genres,
  genreChoices,
  countryChoices,
} from "../classification/options";
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
    genres: [] as number[],
    countries: [] as string[],
    languages: "",
  });
const genreNames = Object.fromEntries(genres.map((g) => [g.value, g.label]));
const countryQuery = ref("");
const genreOptions = computed(() =>
  genreChoices(
    isGroup.value
      ? form.value.mediaType
      : record.value?.value.groups[groupIndex.value]?.mediaType || "movie",
    form.value.genres,
  ),
);
const countryOptions = computed(() =>
  countryChoices(form.value.countries).filter((c) =>
    `${c.label} ${c.value}`
      .toLowerCase()
      .includes(countryQuery.value.toLowerCase()),
  ),
);
const selectedCountryOptions = computed(() =>
  countryChoices(form.value.countries).filter((c) =>
    form.value.countries.includes(c.value),
  ),
);
async function load() {
  busy.value = true;
  error.value = "";
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
    genres: [...(x?.genreIds ?? [])],
    countries: [...((x as Rule)?.countries ?? [])],
    languages: (x as Rule)?.languages?.join(",") ?? "",
  };
  countryQuery.value = "";
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
    genres: [] as number[],
    countries: [] as string[],
    languages: "",
  };
}
function apply() {
  if (!record.value) return;
  if (!form.value.name.trim()) {
    ElMessage.warning("请填写分类名称");
    return;
  }
  const x = {
    id: `rule-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`,
    name: form.value.name,
    directory: form.value.directory || form.value.name,
    enabled: form.value.enabled,
    genreIds: [...form.value.genres],
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
      countries: [...form.value.countries],
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
  <div class="classification-grid" v-loading="busy">
    <section
      v-for="(g, gi) in record?.value.groups ?? []"
      :key="g.id"
      class="panel category-group"
    >
      <header>
        <div class="group-heading">
          <span class="order">{{ gi + 1 }}</span>
          <h2>{{ g.name }}</h2>
          <el-tag size="small">{{
            g.mediaType === "series" ? "剧集" : "电影"
          }}</el-tag
          ><el-tag v-if="!g.enabled" type="info" size="small">已停用</el-tag
          ><span class="muted">{{ g.rules.length }} 条规则</span>
        </div>
        <div class="rule-actions">
          <el-button
            :disabled="gi === 0"
            :aria-label="`向上移动${g.name}`"
            @click="move(gi, -1, -1)"
            ><ArrowUp /></el-button
          ><el-button
            :disabled="gi === (record?.value.groups.length ?? 0) - 1"
            :aria-label="`向下移动${g.name}`"
            @click="move(gi, -1, 1)"
            ><ArrowDown /></el-button
          ><el-button :aria-label="`编辑${g.name}`" @click="edit(gi)"
            ><Edit />编辑组</el-button
          ><el-button :aria-label="`删除${g.name}`" @click="remove(gi)"
            ><Delete /></el-button
          ><el-button :aria-label="`为${g.name}添加规则`" @click="addRule(gi)"
            ><Plus />添加规则</el-button
          >
        </div>
        <div class="group-meta">
          <code>{{ g.directory }}/</code
          ><span v-for="id in g.genreIds" :key="id">{{
            genreNames[id] ?? id
          }}</span>
        </div>
      </header>
      <article v-for="(r, ri) in g.rules" :key="r.id" class="category-rule">
        <div class="rule-title">
          <span class="order">{{ ri + 1 }}</span
          ><strong>{{ r.name }}</strong
          ><el-tag v-if="!r.enabled" type="info" size="small">已停用</el-tag>
        </div>
        <div class="rule-conditions">
          <span v-for="id in r.genreIds" :key="id">{{
            genreNames[id] ?? id
          }}</span
          ><span v-for="c in r.countries" :key="c"
            >{{ countryChoices([c]).find((x) => x.value === c)?.label }} ·
            {{ c }}</span
          ><span v-for="l in r.languages" :key="l">语言 {{ l }}</span
          ><span
            v-if="
              !r.genreIds.length && !r.countries.length && !r.languages.length
            "
            class="muted"
            >兜底 / 所有题材</span
          ><code>{{ g.directory }}/{{ r.directory }}</code>
        </div>
        <div class="rule-actions">
          <el-button :aria-label="`编辑${r.name}`" @click="edit(gi, ri)"
            ><Edit />编辑</el-button
          ><el-button
            :disabled="ri === 0"
            :aria-label="`向上移动${r.name}`"
            @click="move(gi, ri, -1)"
            ><ArrowUp /></el-button
          ><el-button
            :disabled="ri === g.rules.length - 1"
            :aria-label="`向下移动${r.name}`"
            @click="move(gi, ri, 1)"
            ><ArrowDown /></el-button
          ><el-button :aria-label="`删除${r.name}`" @click="remove(gi, ri)"
            ><Delete
          /></el-button>
        </div>
      </article>
      <p v-if="!g.rules.length" class="muted">
        尚无子规则，可添加规则设置分类目录。
      </p>
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
      ><el-form-item label="题材（多选，留空不限）"
        ><div class="choice-grid">
          <label v-for="option in genreOptions" :key="option.value"
            ><input
              v-model="form.genres"
              type="checkbox"
              :value="option.value"
            />{{ option.label }} <small>{{ option.value }}</small></label
          >
        </div></el-form-item
      >
      <template v-if="!isGroup"
        ><el-form-item label="国家 / 地区（多选，留空不限）"
          ><div class="country-picker">
            <el-input
              v-model="countryQuery"
              aria-label="筛选国家地区"
              placeholder="搜索中文名称或地区代码"
              clearable
            />
            <p class="selected-countries">
              已选：{{
                selectedCountryOptions
                  .map((c) => `${c.label} (${c.value})`)
                  .join("、") || "不限"
              }}
            </p>
            <div class="choice-grid country-options">
              <label v-for="option in countryOptions" :key="option.value"
                ><input
                  v-model="form.countries"
                  type="checkbox"
                  :value="option.value"
                />{{ option.label }} <small>{{ option.value }}</small></label
              >
            </div>
          </div></el-form-item
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
  gap: 16px;
}
.category-group {
  min-width: 0;
}
.category-group header {
  display: flex;
  align-items: center;
  gap: 8px 16px;
  flex-wrap: wrap;
  padding-bottom: 12px;
}
.group-heading,
.rule-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.group-heading {
  flex: 1;
  flex-wrap: wrap;
}
.group-heading h2 {
  margin: 0;
  font-size: 17px;
}
.group-meta {
  display: flex;
  gap: 12px;
  flex-basis: 100%;
  font-size: 12px;
  color: var(--muted);
}
.order {
  color: var(--el-color-primary);
  font-variant-numeric: tabular-nums;
  min-width: 20px;
}
.category-rule {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  border-top: 1px solid var(--border);
  padding: 10px 0;
}
.rule-title {
  flex-wrap: wrap;
  font-size: 14px;
}
.rule-conditions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  font-size: 12px;
}
.rule-conditions code {
  flex-basis: 100%;
  overflow-wrap: anywhere;
  color: var(--muted);
}
.rule-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.rule-actions .el-button {
  margin: 0;
  padding: 7px 9px;
}
.rule-actions svg,
.heading-actions svg {
  width: 15px;
}
.choice-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 4px;
  width: 100%;
}
.choice-grid label {
  display: flex;
  align-items: center;
  gap: 5px;
  min-height: 36px;
  font-size: 13px;
  cursor: pointer;
}
.choice-grid small {
  color: var(--muted);
}
.choice-grid input {
  accent-color: var(--el-color-primary);
  width: 16px;
  height: 16px;
}
.country-picker {
  width: 100%;
}
.country-options {
  max-height: 240px;
  overflow: auto;
}
.selected-countries {
  font-size: 12px;
  line-height: 1.6;
  margin: 8px 0;
  overflow-wrap: anywhere;
}
@media (max-width: 1050px) {
  .category-rule {
    grid-template-columns: 120px minmax(0, 1fr);
  }
  .category-rule > .rule-actions {
    grid-column: 2;
  }
}
@media (max-width: 600px) {
  .category-rule {
    grid-template-columns: 1fr;
    gap: 8px;
    padding: 12px 0;
  }
  .category-rule > .rule-actions {
    grid-column: 1;
  }
  .rule-actions .el-button {
    min-height: 44px;
    min-width: 44px;
  }
  .category-group header > .rule-actions {
    flex-basis: 100%;
  }
  .choice-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .choice-grid label {
    min-height: 44px;
  }
  .group-meta {
    overflow-wrap: anywhere;
  }
}
</style>
