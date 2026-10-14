<script setup lang="ts">
import { ElMessageBox } from "element-plus";
import { ref, onMounted } from "vue";
import ScrapingSettings, { type Policy } from "./ScrapingSettings.vue";
import { request } from "../api/client";
interface Settings {
  cleanupAfterImport: boolean;
  trustedProxyCidrs: string[];
  scraping: Policy;
  movieTemplate: string;
  seriesTemplate: string;
  searchCacheSeconds: number;
  tmdbCacheSeconds: number;
  subscriptionCron: string;
  embyUserId: string;
}
const proxyText = ref("");
const namingPreview = ref<{ movie: string; series: string } | null>(null),
  previewing = ref(false);
async function previewNames() {
  if (!record.value) return;
  previewing.value = true;
  error.value = "";
  try {
    namingPreview.value = await request("/settings/naming-preview", {
      method: "POST",
      body: JSON.stringify({
        movieTemplate: record.value.value.movieTemplate,
        seriesTemplate: record.value.value.seriesTemplate,
      }),
    });
  } catch (e) {
    error.value = e instanceof Error ? e.message : "模板预览失败";
  } finally {
    previewing.value = false;
  }
}
const record = ref<{ value: Settings; revision: number } | null>(null),
  busy = ref(false),
  error = ref(""),
  message = ref("");
async function load() {
  try {
    record.value = await request("/settings/application");
    proxyText.value = (record.value?.value.trustedProxyCidrs ?? []).join("\n");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "设置读取失败";
  }
}
async function save() {
  if (!record.value) return;
  record.value.value.trustedProxyCidrs = proxyText.value
    .split(/[\n,]/)
    .map((x) => x.trim())
    .filter(Boolean);
  if (record.value.value.cleanupAfterImport) {
    try {
      await ElMessageBox.confirm(
        "新任务全部上传校验并入库后允许自动清理本地文件和qB种子。H&R未知或有要求时仍须逐项人工确认完成做种；不会自动清理历史任务。",
        "启用上传后清理",
        {
          type: "warning",
          confirmButtonText: "确认启用",
          cancelButtonText: "取消",
        },
      );
    } catch {
      return;
    }
  }
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    record.value = await request("/settings/application", {
      method: "PUT",
      body: JSON.stringify(record.value),
    });
    message.value =
      "模板、订阅Cron与缓存有效期已保存，新的整理预览默认使用模板。不会自动重命名现有文件。";
  } catch (e) {
    error.value = e instanceof Error ? e.message : "保存失败";
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="panel section-gap">
    <div class="panel-title">
      <div>
        <span class="section-kicker">DEFAULTS</span>
        <h2>媒体命名、订阅与缓存</h2>
      </div>
      <span class="muted">命名预览、全局订阅调度与TMDB查询</span>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-alert
      v-if="message"
      :title="message"
      type="success"
      :closable="false"
    />
    <form v-if="record" class="settings-defaults" @submit.prevent="save">
      <fieldset>
        <legend>上传后清理</legend>
        <el-checkbox v-model="record.value.cleanupAfterImport"
          >新任务全部云端校验并入库后清理本地文件及qB种子</el-checkbox
        >
        <p class="field-hint">
          默认关闭。开启后冻结到新任务；H&R未知/有要求、共用文件、缺少云端哈希或任务归属会阻止删除。旧任务须在下载页面单独批准；失败/结果未知不会盲目重放。
        </p>
      </fieldset>
      <fieldset>
        <legend>播放器访客 IP</legend>
        <label for="trusted-proxies">可信反代 IP / CIDR（每行一个）</label
        ><el-input
          id="trusted-proxies"
          v-model="proxyText"
          type="textarea"
          :rows="3"
          placeholder="例如 172.18.0.2/32；留空只记录直连地址"
        />
        <p class="field-hint">
          仅信任实际反代连接来源，从右向左解析
          X-Forwarded-For。不要填写所有私网或公网网段；错误信任会使访客IP可伪造。找不到公网访客地址时显示实际连接地址及“非公网”，不会用服务器IP替代。
        </p>
      </fieldset>
      <fieldset>
        <legend>命名模板</legend>
        <div class="connection-form">
          <label for="movie-naming-template">电影命名模板</label
          ><el-input
            id="movie-naming-template"
            v-model="record.value.movieTemplate"
          />
          <label for="series-naming-template">剧集命名模板</label
          ><el-input
            id="series-naming-template"
            v-model="record.value.seriesTemplate"
          />
          <p class="field-hint">
            支持相对文件夹，例如 {title} ({year})/{title} ({year}) -
            {videoFormat}。{videoFormat}表示已识别分辨率，{season_episode}表示S01E04等已确认季集标签。也支持{season}、{episode}、{resolution}、{source}、{codec}、{group}、{original}、{ext}。自动保留扩展名；绝对路径与../禁止。
          </p>
        </div>
      </fieldset>
      <el-button :loading="previewing" @click="previewNames"
        >预览命名模板</el-button
      >
      <div v-if="namingPreview" class="naming-preview">
        <p>
          电影示例：<code>{{ namingPreview.movie }}</code>
        </p>
        <p>
          剧集示例：<code>{{ namingPreview.series }}</code>
        </p>
        <p class="field-hint">
          纯模板示例，不操作真实文件；STRM实际输出保留版本标识。
        </p>
      </div>
      <ScrapingSettings v-model="record.value.scraping" />
      <fieldset>
        <legend>搜索缓存</legend>
        <div class="cache-fields">
          <div class="connection-form">
            <label for="pt-cache-seconds">站点搜索缓存有效期（秒）</label
            ><el-input-number
              id="pt-cache-seconds"
              v-model="record.value.searchCacheSeconds"
              :min="0"
              :max="86400"
            />
          </div>
          <div class="connection-form">
            <label for="tmdb-cache-seconds">TMDB 搜索缓存有效期（秒）</label
            ><el-input-number
              id="tmdb-cache-seconds"
              v-model="record.value.tmdbCacheSeconds"
              :min="0"
              :max="2592000"
            />
          </div>
        </div>
        <p class="field-hint">
          0
          禁用缓存。TMDB媒体搜索、订阅查询和下载确认共用结果缓存。站点最多1天，TMDB最多30天。历史保存在PostgreSQL，修改缓存时间不会删除历史。配置变更使旧缓存不再复用。
        </p>
      </fieldset>
      <fieldset>
        <legend>订阅调度</legend>
        <div class="connection-form">
          <label for="subscription-cron">订阅 Cron 表达式</label
          ><el-input
            id="subscription-cron"
            v-model="record.value.subscriptionCron"
            placeholder="*/30 * * * *"
          />
          <p class="field-hint">
            五字段：分 时 日 月 周，时区
            Asia/Shanghai，最少5分钟。所有订阅统一按日历时间调度；修改不会启用停用的订阅。
          </p>
        </div>
      </fieldset>
      <div class="settings-save">
        <el-button type="primary" native-type="submit" :loading="busy"
          >保存系统设置</el-button
        ><span class="muted">不会自动重命名现有媒体</span>
      </div>
    </form>
    <el-skeleton v-else-if="!error" :rows="5" animated />
  </section>
</template>
<style scoped>
.naming-preview {
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 8px;
  margin: 16px 0 24px;
}
.naming-preview code {
  white-space: normal;
  overflow-wrap: anywhere;
}
.naming-preview p {
  margin: 8px 0;
}
.settings-defaults {
  max-width: 880px;
}
fieldset {
  border: 0;
  padding: 0;
  margin: 0 0 24px;
  min-width: 0;
}
legend {
  font-size: 14px;
  font-weight: 650;
  border-bottom: 1px solid var(--border);
  padding: 0 0 12px;
  margin-bottom: 12px;
  width: 100%;
}
.cache-fields {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
}
.cache-fields .el-input-number {
  width: 100%;
  max-width: 250px;
}
.settings-save {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
}
@media (max-width: 680px) {
  .cache-fields {
    grid-template-columns: 1fr;
    gap: 16px;
  }
}
</style>

<style scoped>
:deep(.el-checkbox) {
  max-width: 100%;
  height: auto;
  min-height: 44px;
  align-items: flex-start;
}
:deep(.el-checkbox__label) {
  white-space: normal;
  line-height: 1.7;
  overflow-wrap: anywhere;
}
:deep(.el-checkbox__input) {
  margin-top: 5px;
}
.settings-defaults fieldset {
  min-width: 0;
}
</style>
