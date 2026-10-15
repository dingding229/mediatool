<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import { request } from "../api/client";
import ClassificationChoices from "../components/ClassificationChoices.vue";
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
  languageChoices,
  languageLabel,
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
    languages: [] as string[],
  });
const genreNames = Object.fromEntries(genres.map((g) => [g.value, g.label]));
const genreOptions = computed(() =>
  genreChoices(
    isGroup.value
      ? form.value.mediaType
      : record.value?.value.groups[groupIndex.value]?.mediaType || "movie",
    form.value.genres,
  ),
);
const countryOptions = computed(() => countryChoices(form.value.countries));
const languageOptions = computed(() => languageChoices(form.value.languages));
const countryLabels = Object.fromEntries(
  countryChoices([]).map((c) => [c.value, c.label]),
);
const ruleCount = computed(
  () =>
    record.value?.value.groups.reduce((sum, g) => sum + g.rules.length, 0) ?? 0,
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
    languages: [...((x as Rule)?.languages ?? [])],
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
    genres: [] as number[],
    countries: [] as string[],
    languages: [] as string[],
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
      languages: [...form.value.languages],
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
  <section class="panel classification-list-panel" v-loading="busy">
    <div class="panel-title">
      <h2>分类规则列表</h2>
      <span v-if="record" class="muted"
        >{{ record.value.groups.length }} 个分类组 ·
        {{ ruleCount }} 条子规则</span
      >
    </div>
    <p class="list-hint">
      按列表顺序匹配，组内首条命中规则生效。窄屏可左右滑动查看完整列表。
    </p>
    <div
      class="classification-table-scroll"
      tabindex="0"
      role="region"
      aria-label="目录分类列表横向滚动区"
    >
      <table class="classification-table" aria-label="目录分类规则列表">
        <colgroup>
          <col class="name-col" />
          <col class="type-col" />
          <col class="conditions-col" />
          <col class="directory-col" />
          <col class="status-col" />
          <col class="actions-col" />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">分类名称</th>
            <th scope="col">媒体类型</th>
            <th scope="col">匹配条件</th>
            <th scope="col">目标目录</th>
            <th scope="col">状态</th>
            <th scope="col" class="actions-heading">操作</th>
          </tr>
        </thead>
        <tbody v-for="(g, gi) in record?.value.groups ?? []" :key="g.id">
          <tr class="classification-group-row">
            <th
              scope="rowgroup"
              :id="`classification-group-${g.id}`"
              :aria-label="g.name"
            >
              <div class="classification-name">
                <span class="order">{{ gi + 1 }}</span
                ><strong>{{ g.name }}</strong>
              </div>
              <small>分类组 · {{ g.rules.length }} 条子规则</small>
            </th>
            <td>{{ g.mediaType === "series" ? "剧集" : "电影" }}</td>
            <td>
              <div class="condition-values">
                <span v-for="id in g.genreIds" :key="id">{{
                  genreNames[id] ?? id
                }}</span
                ><span v-if="!g.genreIds.length" class="muted">全部题材</span>
              </div>
            </td>
            <td>
              <code>{{ g.directory }}/</code>
            </td>
            <td>
              <el-tag size="small" :type="g.enabled ? 'success' : 'info'">{{
                g.enabled ? "已启用" : "已停用"
              }}</el-tag>
            </td>
            <td>
              <div class="list-actions">
                <el-button
                  :aria-label="`为${g.name}添加规则`"
                  title="新增子规则"
                  @click="addRule(gi)"
                  ><Plus /></el-button
                ><el-button
                  :aria-label="`编辑${g.name}`"
                  title="编辑分类组"
                  @click="edit(gi)"
                  ><Edit />编辑</el-button
                ><el-button
                  :disabled="gi === 0"
                  :aria-label="`向上移动${g.name}`"
                  title="上移分类组"
                  @click="move(gi, -1, -1)"
                  ><ArrowUp /></el-button
                ><el-button
                  :disabled="gi === (record?.value.groups.length ?? 0) - 1"
                  :aria-label="`向下移动${g.name}`"
                  title="下移分类组"
                  @click="move(gi, -1, 1)"
                  ><ArrowDown /></el-button
                ><el-button
                  :aria-label="`删除${g.name}`"
                  title="删除分类组"
                  @click="remove(gi)"
                  ><Delete
                /></el-button>
              </div>
            </td>
          </tr>
          <tr
            v-for="(r, ri) in g.rules"
            :key="r.id"
            class="classification-rule-row"
          >
            <th
              scope="row"
              :headers="`classification-group-${g.id}`"
              :aria-label="r.name"
            >
              <div class="classification-name child-name">
                <span class="order">{{ gi + 1 }}.{{ ri + 1 }}</span
                ><strong>{{ r.name }}</strong>
              </div>
            </th>
            <td class="muted">
              {{ g.mediaType === "series" ? "剧集" : "电影" }}
            </td>
            <td>
              <div class="condition-values">
                <span v-for="id in r.genreIds" :key="id">{{
                  genreNames[id] ?? id
                }}</span
                ><span v-for="c in r.countries" :key="c"
                  >{{ countryLabels[c] ?? "已保存地区" }} · {{ c }}</span
                ><span v-for="l in r.languages" :key="l">{{ languageLabel(l) }} · {{ l }}</span
                ><span
                  v-if="
                    !r.genreIds.length &&
                    !r.countries.length &&
                    !r.languages.length
                  "
                  class="muted"
                  >兜底 / 所有题材</span
                >
              </div>
            </td>
            <td>
              <code>{{ g.directory }}/{{ r.directory }}</code>
            </td>
            <td>
              <el-tag
                size="small"
                :type="r.enabled && g.enabled ? 'success' : 'info'"
                >{{
                  !g.enabled ? "组已停用" : r.enabled ? "已启用" : "已停用"
                }}</el-tag
              ><small v-if="!g.enabled"
                >规则{{ r.enabled ? "启用" : "停用" }}</small
              >
            </td>
            <td>
              <div class="list-actions">
                <el-button
                  :aria-label="`编辑${r.name}`"
                  title="编辑子规则"
                  @click="edit(gi, ri)"
                  ><Edit />编辑</el-button
                ><el-button
                  :disabled="ri === 0"
                  :aria-label="`向上移动${r.name}`"
                  title="上移子规则"
                  @click="move(gi, ri, -1)"
                  ><ArrowUp /></el-button
                ><el-button
                  :disabled="ri === g.rules.length - 1"
                  :aria-label="`向下移动${r.name}`"
                  title="下移子规则"
                  @click="move(gi, ri, 1)"
                  ><ArrowDown /></el-button
                ><el-button
                  :aria-label="`删除${r.name}`"
                  title="删除子规则"
                  @click="remove(gi, ri)"
                  ><Delete
                /></el-button>
              </div>
            </td>
          </tr>
          <tr v-if="!g.rules.length" class="classification-empty-group">
            <td colspan="6">此分类组暂无子规则，可使用右侧新增按钮添加。</td>
          </tr>
        </tbody>
        <tbody v-if="record && !record.value.groups.length && !busy">
          <tr>
            <td colspan="6">
              <el-empty description="尚无分类规则，请添加分类组" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
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
      ><el-form-item label="题材（多选，留空不限）">
        <ClassificationChoices
          v-model="form.genres"
          :key="`${groupIndex}:${ruleIndex}:genres:${dialog}`"
          :options="genreOptions"
          label="题材"
          search-label="筛选题材"
          placeholder="搜索题材名称或代码"
        />
      </el-form-item>
      <template v-if="!isGroup">
        <el-form-item label="国家 / 地区（多选，留空不限）">
          <ClassificationChoices
            v-model="form.countries"
            :key="`${groupIndex}:${ruleIndex}:countries:${dialog}`"
            :options="countryOptions"
            label="国家地区"
            search-label="筛选国家地区"
            placeholder="搜索中文名称或地区代码"
          />
        </el-form-item>
        <el-form-item label="原始语言（多选，留空不限）">
          <ClassificationChoices
            v-model="form.languages"
            :key="`${groupIndex}:${ruleIndex}:languages:${dialog}`"
            :options="languageOptions"
            label="原始语言"
            search-label="筛选原始语言"
            placeholder="搜索中文名称或语言代码"
          />
        </el-form-item>
      </template
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
.classification-list-panel {
  padding: 20px;
  min-width: 0;
}
.classification-list-panel .panel-title {
  margin-bottom: 8px;
}
.list-hint {
  font-size: 12px;
  color: var(--muted);
  margin: 0 0 16px;
}
.classification-table-scroll {
  overflow-x: auto;
  max-width: 100%;
  border: 1px solid var(--border);
  border-radius: 8px;
}
.classification-table {
  width: 100%;
  min-width: 1060px;
  border-collapse: collapse;
  table-layout: fixed;
  text-align: left;
  font-size: 13px;
}
.name-col {
  width: 172px;
}
.type-col {
  width: 76px;
}
.conditions-col {
  width: auto;
}
.directory-col {
  width: 215px;
}
.status-col {
  width: 90px;
}
.actions-col {
  width: 234px;
}
.classification-table th,
.classification-table td {
  padding: 10px 12px;
  border-bottom: 1px solid var(--border);
  vertical-align: middle;
  overflow-wrap: anywhere;
}
.classification-table thead th {
  background: var(--el-fill-color-light);
  color: var(--muted);
  font-weight: 500;
  font-size: 12px;
  padding-top: 12px;
  padding-bottom: 12px;
}
.classification-table tbody:last-child tr:last-child > * {
  border-bottom: 0;
}
.classification-table tbody th {
  font-weight: normal;
}
.classification-group-row > * {
  background: var(--el-fill-color-light);
}
.classification-rule-row:hover > * {
  background: var(--el-color-primary-light-9);
}
.classification-name {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 1.6;
}
.classification-name strong {
  font-weight: 600;
}
.child-name {
  padding-left: 12px;
}
.child-name:before {
  content: "";
  width: 8px;
  height: 8px;
  border-left: 1px solid var(--border-strong);
  border-bottom: 1px solid var(--border-strong);
  flex-shrink: 0;
}
.order {
  color: var(--muted);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
.classification-table small {
  display: block;
  font-size: 11px;
  color: var(--muted);
  margin-top: 4px;
}
.classification-group-row th > small {
  padding-left: 20px;
}
.condition-values {
  display: flex;
  flex-wrap: wrap;
  gap: 3px 10px;
  line-height: 1.7;
}
.classification-table code {
  font-size: 12px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}
.actions-heading {
  text-align: right;
}
.list-actions {
  display: flex;
  gap: 4px;
  justify-content: flex-end;
  align-items: center;
  flex-wrap: nowrap;
}
.list-actions .el-button {
  margin: 0;
  padding: 6px 8px;
  min-width: 32px;
  min-height: 32px;
}
.list-actions svg,
.heading-actions svg {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}
.classification-empty-group td {
  font-size: 12px;
  color: var(--muted);
  padding-left: 32px;
}
@media (max-width: 760px) {
  .classification-list-panel {
    padding: 16px;
  }
  .classification-table {
    min-width: 1100px;
  }
  .actions-col {
    width: 272px;
  }
  .list-actions .el-button {
    min-width: 44px;
    min-height: 44px;
  }
}
</style>
