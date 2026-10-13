<script setup lang="ts">
import { ref, onMounted } from "vue";
import { request } from "../api/client";
interface Settings {
  movieTemplate: string;
  seriesTemplate: string;
  searchCacheSeconds: number;
  tmdbCacheSeconds: number;
  subscriptionIntervalMinutes: number;
  embyUserId: string;
}
const record = ref<{ value: Settings; revision: number } | null>(null),
  busy = ref(false),
  error = ref(""),
  message = ref("");
async function load() {
  try {
    record.value = await request("/settings/application");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "设置读取失败";
  }
}
async function save() {
  if (!record.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    record.value = await request("/settings/application", {
      method: "PUT",
      body: JSON.stringify(record.value),
    });
    message.value =
      "模板、订阅间隔与缓存有效期已保存，新的整理预览默认使用模板。不会自动重命名现有文件。";
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
            支持
            {title}、{year}、{season}、{episode}、{resolution}、{source}、{codec}、{group}、{ext}、{original}。缺失年份不猜造，未知类型需确认；保留扩展名请包含
            {ext}。
          </p>
        </div>
      </fieldset>
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
          <label for="subscription-interval-minutes">订阅刷新间隔（分钟）</label
          ><el-input-number
            id="subscription-interval-minutes"
            v-model="record.value.subscriptionIntervalMinutes"
            :min="5"
            :max="1440"
            :precision="0"
          />
          <p class="field-hint">
            所有订阅统一使用此间隔，5分钟～1天。修改会更新已有订阅的下次调度时间，不会启用停用的订阅或重复已有下载。
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
