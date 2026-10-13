<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Grid, List } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { request, post } from "../api/client";
interface Site {
  id: string;
  name: string;
  domain: string;
  version: string;
  iconUrl: string;
  loaded: boolean;
}
const layout = ref(
  localStorage.getItem("media-tool:template-layout") === "list"
    ? "list"
    : "grid",
);
function setLayout(value: string) {
  layout.value = value;
  localStorage.setItem("media-tool:template-layout", value);
}
const templates = ref<Site[]>([]);
const selected = ref<string[]>([]);
const loading = ref(true);
const saving = ref(false);
const error = ref("");
const iconErrors = ref(new Set<string>());
const available = computed(() =>
  templates.value.filter((site) => !site.loaded),
);
const allSelected = computed(
  () =>
    available.value.length > 0 &&
    selected.value.length === available.value.length,
);
const partlySelected = computed(
  () => selected.value.length > 0 && !allSelected.value,
);
function selectAll(value: boolean) {
  selected.value = value ? available.value.map((site) => site.id) : [];
}
async function load() {
  try {
    templates.value = (
      await request<{ templates: Site[] }>("/pt/templates")
    ).templates;
  } catch (e) {
    error.value = String(e);
  } finally {
    loading.value = false;
  }
}
async function loadSelected() {
  if (!selected.value.length) return;
  saving.value = true;
  error.value = "";
  try {
    templates.value = (
      await post<{ templates: Site[] }>("/pt/templates/load", {
        ids: selected.value,
      })
    ).templates;
    selected.value = [];
    ElMessage.success("所选站点模板已加载，可在PT站点中添加账号");
  } catch (e) {
    error.value = String(e);
  } finally {
    saving.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ACQUIRE / 模板目录</span>
      <h1>站点模板</h1>
      <p>
        勾选需要的站点模板后加载，再到 PT
        站点添加账号。已添加账号的模板将自动保留。
      </p>
    </div>
    <el-button
      type="primary"
      :disabled="!selected.length"
      :loading="saving"
      @click="loadSelected"
      >加载所选模板{{
        selected.length ? `（${selected.length}）` : ""
      }}</el-button
    >
  </section>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <el-skeleton v-if="loading" :rows="3" animated />
  <template v-else>
    <div class="template-toolbar">
      <div class="template-layout">
        <el-button
          :icon="Grid"
          :type="layout === 'grid' ? 'primary' : 'default'"
          aria-label="网格展示模板"
          :aria-pressed="layout === 'grid'"
          @click="setLayout('grid')"
        /><el-button
          :icon="List"
          :type="layout === 'list' ? 'primary' : 'default'"
          aria-label="列表展示模板"
          :aria-pressed="layout === 'list'"
          @click="setLayout('list')"
        />
      </div>
      <el-checkbox
        :model-value="allSelected"
        :indeterminate="partlySelected"
        :disabled="saving || !available.length"
        @change="selectAll(Boolean($event))"
        >全选可加载模板</el-checkbox
      ><el-button
        text
        :disabled="saving || !selected.length"
        @click="selectAll(false)"
        >取消全选</el-button
      >
    </div>
    <p class="field-hint">
      {{ templates.filter((site) => site.loaded).length }} 个已加载 ·
      {{ available.length }} 个可选择
    </p>
    <el-checkbox-group
      v-model="selected"
      aria-label="选择站点模板"
      class="template-grid"
      :class="{ 'template-list': layout === 'list' }"
    >
      <article
        v-for="site in templates"
        :key="site.id"
        class="panel template-card"
        :class="{ chosen: selected.includes(site.id) }"
      >
        <header>
          <img
            v-if="!iconErrors.has(site.id)"
            :src="site.iconUrl"
            :alt="`${site.name}图标`"
            width="40"
            height="40"
            loading="lazy"
            referrerpolicy="no-referrer"
            @error="iconErrors.add(site.id)"
          />
          <span v-else class="site-fallback" aria-hidden="true">{{
            site.name.slice(0, 2)
          }}</span>
          <div>
            <h2>{{ site.name }}</h2>
            <span class="field-hint">v{{ site.version }}</span>
          </div>
        </header>
        <a
          :href="site.domain"
          target="_blank"
          rel="noopener noreferrer"
          class="site-url"
          >{{ site.domain }}</a
        >
        <el-checkbox :value="site.id" :disabled="site.loaded || saving">{{
          site.loaded ? "已加载" : `选择 ${site.name}`
        }}</el-checkbox>
      </article>
    </el-checkbox-group>
  </template>
</template>
<style scoped>
.template-layout {
  display: flex;
  gap: 8px;
  margin-left: auto;
  order: 2;
}
.template-layout .el-button {
  margin: 0;
  min-width: 44px;
  min-height: 44px;
}
.template-list {
  grid-template-columns: 1fr !important;
}
.template-list .template-card {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(180px, 1fr) 140px;
  align-items: center;
}
@media (max-width: 680px) {
  .template-list .template-card {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .template-list .site-url {
    grid-column: 1/-1;
    grid-row: 2;
  }
}

.template-toolbar {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.template-toolbar :deep(.el-checkbox) {
  min-height: 44px;
}
.template-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 16px;
}
.template-card {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.template-card.chosen {
  border-color: var(--accent);
}
.template-card header {
  display: flex;
  gap: 14px;
  align-items: center;
}
.template-card h2 {
  margin: 0;
  font-size: 18px;
}
.template-card img,
.site-fallback {
  flex-shrink: 0;
  border-radius: 8px;
}
.site-fallback {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  font-size: 14px;
  font-weight: 600;
  background: var(--accent-soft);
  color: var(--accent);
}
.site-url {
  color: var(--accent);
  overflow-wrap: anywhere;
  font-size: 13px;
}
.template-card :deep(.el-checkbox) {
  min-height: 44px;
}
@media (max-width: 600px) {
  .template-toolbar {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 8px;
  }
  .template-toolbar :deep(.el-checkbox) {
    min-height: 44px;
  }
  .template-grid {
    grid-template-columns: 1fr;
  }
  .page-heading {
    align-items: stretch;
    gap: 16px;
  }
}
</style>
