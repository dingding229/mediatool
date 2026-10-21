<script setup lang="ts">
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText } from '../workflows/readiness';
defineProps<{
  model: any;
}>();
</script>
<template>
  <el-dialog v-model="model.visible" title="确认媒体身份与下载" width="min(700px,94vw)" :close-on-click-modal="false" :close-on-press-escape="!model.busy" :show-close="!model.busy" class="acquire-dialog">
    <div class="acquire-form">
      <h3>{{ model.resource?.title||'—' }}</h3>
      <p>下载后保留原文件做种，上传与入库另行核对；含 H&amp;R 的资源须满足站点做种要求。</p>
      <section v-if="model.usingIdentity&&model.chosen" class="acquire-identity">
        <h3>已使用搜索中的媒体身份</h3>
        <strong>{{ model.chosen.title }}</strong>
        <p>
          {{ model.chosen.mediaType==='series'?'剧集':'电影' }}
          ·
          {{ model.chosen.year||'—' }}
          · TMDB

        </p>
        <p>{{ model.chosen.originalTitle||'—' }}</p>
        <el-button :disabled="!!model.intent" @click="model.usingIdentity=false">更正媒体身份</el-button>
      </section>
      <template v-else-if="!model.intent">
        <div class="acquire-query-row">
          <el-select v-model="model.kind" aria-label="媒体类型">
            <el-option value="auto" label="自动识别"/>
            <el-option value="movie" label="电影"/>
            <el-option value="series" label="剧集"/>
          </el-select>
          <el-input v-model="model.query" aria-label="TMDB关键词"/>
          <el-button :loading="model.loading" @click="model.lookup">查询 TMDB</el-button>
        </div>
        <div class="acquire-choices" role="group" aria-label="下载媒体匹配">
          <button v-for="item in model.media" :key="item.mediaType+item.id" type="button" :aria-pressed="model.chosen?.id===item.id&&model.chosen?.mediaType===item.mediaType" @click="model.chosen=item">{{ item.title }} · {{ item.year||'—' }} · {{ item.mediaType==='series'?'剧集':'电影' }}</button>
        </div>
      </template>
      <p>下载使用打开弹窗时选中的搜索快照；过期或站点配置变化时请重新搜索。</p>
      <el-alert v-if="model.error" :title="errorText(model.error)" type="error" :closable="false"/>
      <p v-if="model.yearWarning" role="alert">{{ model.yearWarning }}</p>
      <el-checkbox v-if="model.yearWarning" v-model="model.yearConfirmed" :disabled="!!model.intent">我已核对资源年份/季年份与所选TMDB剧集身份，确认继续下载</el-checkbox>
      <AsyncResultBanner v-if="model.intent" :status="model.intent.phase" :message="model.intent.phase==='queued'?'下载已排队，尚未确认下载器接收。':'投递回执未知，先核对下载任务，不重复提交。'" @refresh="model.reconcile"/>
      <RouterLink v-if="model.intent" :to="{path:'/downloads',query:{tab:'tasks',taskId:model.intent.id}}">前往下载任务核对</RouterLink>
      <el-button v-else type="primary" :loading="model.busy" :disabled="!model.chosen||(!!model.yearWarning&&!model.yearConfirmed)" @click="model.submit">创建下载任务</el-button>
    </div>
  </el-dialog>
</template>
