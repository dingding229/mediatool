<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { request, post } from "../api/client";
import { ElMessageBox } from "element-plus";
interface Asset {
  id: string;
  title: string;
  mediaType: string;
  year: string;
  source: string;
  poster?: string;
  outputs: { kind: string; path: string; owner?: string }[];
  cloudState?: string;
  artifactsState?: string;
}
interface Row {
  ordinal: number;
  assetId: string;
  path: string;
  state: string;
  message: string;
  digest: string;
}
interface Batch {
  id: string;
  state: string;
  digest: string;
  expiresAt: string;
  items: Row[];
}
const route = useRoute(),
  router = useRouter(),
  paths = ref(""),
  preview = ref<
    {
      path: string;
      url?: string;
      content?: string;
      valid: boolean;
      message?: string;
    }[]
  >([]),
  assets = ref<Asset[]>([]),
  selected = ref<string[]>([]),
  assetPickerOpen = ref(false),
  selectionLimit = ref(100),
  batch = ref<Batch>(),
  busy = ref(false),
  error = ref(""),
  q = ref("");
const labels: Record<string, string> = {
  preview: "等待批准",
  running: "执行中",
  succeeded: "修复完成",
  partial: "部分完成 / 含跳过项",
  ready: "原文完整",
  missing: "待恢复缺失文件",
  staged: "已登记恢复意图",
  repaired: "已修复",
  needs_reconcile: "任务中断 / 需明确恢复",
  blocked: "跳过 / 需核对",
};
async function batchPreview() {
  busy.value = true;
  error.value = "";
  try {
    preview.value = await post("/strm/batch-preview", {
      paths: paths.value.split(/\r?\n/).filter((s) => s.trim()),
    });
  } catch (e) {
    error.value = e instanceof Error ? e.message : "批量预览失败";
  } finally {
    busy.value = false;
  }
}
const selectedCount = computed(() => selected.value.length);
const visibleAssetIds = computed(() => assets.value.map((asset) => asset.id));
const allVisibleSelected = computed(
  () =>
    visibleAssetIds.value.length > 0 &&
    visibleAssetIds.value.every((id) => selected.value.includes(id)),
);
function toggleVisibleAssets() {
  if (allVisibleSelected.value) {
    selected.value = selected.value.filter(
      (id) => !visibleAssetIds.value.includes(id),
    );
    return;
  }
  const next = [...selected.value];
  for (const id of visibleAssetIds.value) {
    if (next.length >= selectionLimit.value) break;
    if (!next.includes(id)) next.push(id);
  }
  selected.value = next;
}
function onSelectionLimitChange(value: number | undefined) {
  const limit = Math.min(100, Math.max(1, Number(value) || 1));
  selectionLimit.value = limit;
  if (selected.value.length > limit) selected.value = selected.value.slice(0, limit);
}
async function loadAssets() {
  busy.value = true;
  error.value = "";
  try {
    assets.value = (
      await request<{ items: Asset[] }>(
        `/library/outputs?kind=strm&page=1&pageSize=100&q=${encodeURIComponent(q.value)}`,
      )
    ).items;
    selected.value = [];
  } catch (e) {
    error.value = e instanceof Error ? e.message : "资产读取失败";
  } finally {
    busy.value = false;
  }
}
async function chooseRepairAssets() {
  assetPickerOpen.value = true;
  if (!assets.value.length) await loadAssets();
}
async function check() {
  if (selected.value.length < 1 || selected.value.length > 100) {
    error.value = "请先选择1–100个托管资产";
    assetPickerOpen.value = true;
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    batch.value = await post("/strm/repairs/preview", {
      assets: selected.value.slice(0, 100),
    });
    await router.replace({
      query: { ...route.query, repairId: batch.value!.id },
    });
  } catch (e) {
    error.value = e instanceof Error ? e.message : "检查失败";
  } finally {
    busy.value = false;
  }
}
async function refreshBatch() {
  const id = batch.value?.id || String(route.query.repairId || "");
  if (!id) return;
  busy.value = true;
  error.value = "";
  try {
    batch.value = await request(`/strm/repairs/${id}`);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "批次读取失败";
  } finally {
    busy.value = false;
  }
}
async function approve() {
  if (!batch.value) return;
  try {
    await ElMessageBox.confirm(
      "仅恢复可证明原文的缺失STRM；不覆盖已修改文件、不旋转文件Key、不处理NFO。",
      "批准全局STRM修复",
      {
        confirmButtonText: "明确批准修复",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
    busy.value = true;
    error.value = "";
    batch.value = await post(`/strm/repairs/${batch.value.id}/approve`, {
      digest: batch.value.digest,
      confirm: true,
    });
  } catch (e) {
    if (e === "cancel" || e === "close") return;
    error.value =
      e instanceof Error
        ? e.message
        : "结果尚未确认，请刷新现有批次状态，不重新创建或盲目重放";
  } finally {
    busy.value = false;
  }
}
async function resume() {
  if (!batch.value) return;
  try {
    await ElMessageBox.confirm(
      "核对并恢复该已批准批次，沿用原始修复意图；不会覆盖手工文件或生成新Key。",
      "恢复STRM修复任务",
      {
        confirmButtonText: "明确恢复任务",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
    busy.value = true;
    batch.value = await post(`/strm/repairs/${batch.value.id}/retry`, {
      digest: batch.value.digest,
      confirm: true,
    });
  } catch (e) {
    if (e === "cancel" || e === "close") return;
    error.value = e instanceof Error ? e.message : "请先刷新批次核对状态";
  } finally {
    busy.value = false;
  }
}
onMounted(() => {
  if (route.query.repairId) void refreshBatch();
});
</script>
<template>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <section class="panel tool-section">
    <div class="panel-title">
      <div>
        <h2>批量链接预览</h2>
        <p class="field-hint">
          每行一个实际云盘完整路径，最多100条；只预览，不写入服务器或声称可播放。
        </p>
      </div>
    </div>
    <label for="strm-batch-paths">云盘路径列表</label
    ><el-input
      id="strm-batch-paths"
      v-model="paths"
      type="textarea"
      :rows="5"
      placeholder="/115/媒体/电影一.mkv&#10;/115/媒体/电影二.mkv"
    /><el-button
      type="primary"
      :loading="busy"
      :disabled="!paths.trim()"
      @click="batchPreview"
      >批量预览链接</el-button
    >
    <div v-for="(row, i) in preview" :key="i" class="preview-row">
      <strong>{{ row.path }}</strong
      ><span>{{ row.valid ? "已生成固定入口" : row.message }}</span
      ><el-input
        v-if="row.valid"
        :model-value="row.content"
        type="textarea"
        :rows="2"
        readonly
        :aria-label="`第${i + 1}条STRM预览`"
      />
    </div>
  </section>
  <section class="panel tool-section">
    <div class="panel-title">
      <div>
        <h2>全局修复STRM</h2>
        <p class="field-hint">
          检查托管输出，保持原始正文、文件级Key和媒体文件；手工改动和证据不足的文件不会覆盖。
        </p>
      </div>
      <el-button :loading="busy" @click="refreshBatch">刷新修复批次</el-button>
    </div>
    <p class="field-hint">
      从托管资产中选择本批需要检查的项目，每批必须选择1–100个资产。只恢复可信原文，手工改动和归属冲突会跳过。
    </p>
    <el-button
      v-if="!assetPickerOpen && !batch"
      :disabled="busy"
      :loading="busy"
      @click="chooseRepairAssets"
      >全局检查并预览修复</el-button>
    >
    <section v-if="assetPickerOpen && !batch" class="repair-selector">
      <div class="repair-selector-toolbar">
        <div>
          <h3>选择本批托管资产</h3>
          <p class="field-hint">
            已选择 {{ selectedCount }} / {{ selectionLimit }}；本批最多100个，至少选择1个。
          </p>
        </div>
        <div class="repair-selector-actions">
          <el-input
            v-model="q"
            aria-label="筛选托管资产"
            placeholder="搜索标题或路径"
            clearable
            @keyup.enter="loadAssets"
          />
          <el-input-number
            v-model="selectionLimit"
            aria-label="本批选择上限"
            :min="1"
            :max="100"
            controls-position="right"
            @change="onSelectionLimitChange"
          />
          <el-button @click="loadAssets" :loading="busy">刷新资产</el-button>
        </div>
      </div>
      <div class="repair-selector-summary">
        <span>当前加载 {{ assets.length }} 个托管资产</span>
        <div>
          <el-button link @click="toggleVisibleAssets">
            {{ allVisibleSelected ? "取消全选当前列表" : "全选当前列表" }}
          </el-button>
          <el-button link @click="selected = []" :disabled="!selected.length">
            清空选择
          </el-button>
        </div>
      </div>
      <div v-if="assets.length" class="repair-assets">
        <label v-for="asset in assets" :key="asset.id" class="repair-asset">
          <input
            v-model="selected"
            type="checkbox"
            :value="asset.id"
            :disabled="!selected.includes(asset.id) && selected.length >= selectionLimit"
          />
          <img v-if="asset.poster" :src="asset.poster" :alt="`${asset.title || '未命名媒体'}海报`" loading="lazy" />
          <span class="repair-asset-copy">
            <strong>{{ asset.title || "未命名媒体" }}</strong>
            <small>{{ asset.year || "年份未知" }} · {{ asset.mediaType === "series" ? "剧集" : "电影" }}</small>
            <code>{{ asset.source }}</code>
            <small>{{ asset.outputs?.length || 0 }} 个托管输出</small>
          </span>
        </label>
      </div>
      <el-empty v-else description="暂无托管资产，请先完成文件与入库" :image-size="72" />
      <div class="repair-selector-footer">
        <el-button @click="assetPickerOpen = false">取消</el-button>
        <el-button
          type="primary"
          :loading="busy"
          :disabled="selectedCount < 1 || selectedCount > 100"
          @click="check"
          >检查已选 {{ selectedCount }} 个资产</el-button
        >
      </div>
    </section>
    <section v-if="batch" class="repair-batch">
      <div class="panel-title">
        <div>
          <h3>{{ labels[batch.state] || batch.state }}</h3>
          <p class="field-hint">
            批次 {{ batch.id }} · 预览有效至
            {{ new Date(batch.expiresAt).toLocaleString("zh-CN") }}
          </p>
        </div>
        <el-button :loading="busy" @click="refreshBatch"
          >刷新批次状态</el-button
        >
      </div>
      <ul>
        <li v-for="row in batch.items" :key="row.ordinal">
          <el-tag
            :type="
              row.state === 'blocked'
                ? 'warning'
                : row.state === 'repaired' || row.state === 'ready'
                  ? 'success'
                  : 'info'
            "
            >{{ labels[row.state] || row.state }}</el-tag
          ><code>{{ row.path || row.assetId }}</code>
          <p>{{ row.message }}</p>
        </li>
      </ul>
      <el-button
        v-if="batch.state === 'preview'"
        type="primary"
        :loading="busy"
        :disabled="!batch.items.some((r) => r.state === 'missing')"
        @click="approve"
        >批准批量修复</el-button
      ><el-button
        v-if="batch.state === 'needs_reconcile'"
        :loading="busy"
        @click="resume"
        >核对后恢复此批次</el-button
      >
      <p class="field-hint">
        没有可安全修复项时不会执行。请求结果未知时只刷新此批次；服务器恢复意图持久化，不自动重新上传源媒体。
      </p>
    </section>
  </section>
</template>
<style scoped>
.tool-section {
  margin-top: 24px;
}
.tool-section > label {
  display: block;
  margin-bottom: 12px;
}
.tool-section > .el-button {
  margin-top: 16px;
  min-height: 44px;
}
.preview-row {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 20px;
  border-top: 1px solid var(--border);
  padding-top: 20px;
  overflow-wrap: anywhere;
}

.repair-selector {
  margin-top: 20px;
  padding: 20px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--el-fill-color-lighter);
}
.repair-selector-toolbar,
.repair-selector-summary,
.repair-selector-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.repair-selector-toolbar h3 {
  margin: 0 0 6px;
}
.repair-selector-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.repair-selector-actions .el-input {
  width: 220px;
}
.repair-selector-actions .el-input-number {
  width: 150px;
}
.repair-selector-summary {
  margin: 16px 0 12px;
  padding: 10px 0;
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  color: var(--muted);
  font-size: 13px;
}
.repair-assets {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  max-height: 520px;
  overflow: auto;
}
.repair-asset {
  display: grid;
  grid-template-columns: 18px 48px minmax(0, 1fr);
  align-items: start;
  gap: 10px;
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface, #fff);
  cursor: pointer;
}
.repair-asset:has(input:checked) {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.repair-asset input {
  width: 16px;
  height: 16px;
  margin-top: 14px;
  accent-color: var(--el-color-primary);
}
.repair-asset img,
.repair-asset::before {
  width: 48px;
  height: 68px;
  border-radius: 4px;
  object-fit: cover;
  background: var(--el-fill-color-light);
}
.repair-asset-copy {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.repair-asset-copy strong,
.repair-asset-copy code {
  overflow-wrap: anywhere;
}
.repair-asset-copy small {
  color: var(--muted);
  line-height: 1.5;
}
.repair-selector-footer {
  justify-content: flex-end;
  margin-top: 16px;
}
.repair-filter {
  display: flex;
  gap: 12px;
  margin: 16px 0;
}
.repair-batch {
  margin-top: 24px;
  border-top: 1px solid var(--border);
  padding-top: 24px;
}
.repair-batch ul {
  list-style: none;
  padding: 0;
}
.repair-batch li {
  padding: 16px 0;
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.repair-batch li code {
  overflow-wrap: anywhere;
  min-width: 0;
  flex: 1;
}
.repair-batch li p {
  flex-basis: 100%;
  color: var(--muted);
  margin: 0;
  line-height: 1.7;
}
.repair-batch .panel-title {
  gap: 16px;
  flex-wrap: wrap;
}
@media (max-width: 680px) {
  .repair-assets {
    grid-template-columns: 1fr;
  }
  .repair-batch li code {
    flex-basis: 100%;
  }
}
</style>
