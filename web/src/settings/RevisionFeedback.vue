<script setup lang="ts">
import { computed } from 'vue';
import {scheduleLabel} from '../presentation/labels';
import StateView from '../ui/StateView.vue';
import { stateFor, messageFor, type RevisionForm } from './revision';
const props = defineProps<{
  form: RevisionForm;
}>();
const labels: Record<string, string> = { managementUrl: '后台访问域名', playbackUrl: '播放链接域名', memberUrl: '内置媒体库域名', trustedProxyCidrs: '可信反代地址', movieTemplate: '电影命名模板', seriesTemplate: '剧集命名模板', scraping: '入库资料', cleanupAfterImport: '上传后清理', searchCacheSeconds: '站点搜索缓存（秒）', tmdbCacheSeconds: 'TMDB 缓存（秒）', subscriptionCron: '订阅调度', groups: '目录分类' };
const differences = computed(() => Object.keys(props.form.draft.value || {}).filter(k => JSON.stringify(props.form.draft.value[k]) !== JSON.stringify(props.form.latest.value?.value[k])));
function display(value: any, field?:string): string {
  if(field==='subscriptionCron')return scheduleLabel(value);
  if (value == null || value === '')
    return '—';
  if (typeof value === 'boolean')
    return value ? '启用' : '关闭';
  if (Array.isArray(value))
    return value.length?value.map(v=>display(v)).join('；'):'无';
  if (typeof value === 'object')
    return Object.entries(value).filter(([k])=>!['id','revision'].includes(k)).map(([k,v])=>(labels[k]||({name:'名称',directory:'目录',enabled:'启用',rules:'规则',countries:'地区',languages:'语言',genreIds:'题材',movie:'电影',series:'剧集',season:'季',episode:'集',nfo:'资料',poster:'海报',backdrop:'背景',logo:'标志',thumb:'缩略图'} as Record<string,string>)[k]||'其他设置')+'：'+display(v)).join('；');
  return String(value);
}
</script>
<template>
  <StateView v-if="form.loading.value&&!form.draft.value" state="loading"/>
  <StateView v-if="form.error.value" :state="stateFor(form.error.value)" :description="messageFor(form.error.value)" retry-text="重试" @retry="form.load"/>
  <p v-if="form.notice.value" role="status" class="settings-notice">{{form.notice.value}}</p>
  <section v-if="form.conflict.value" class="settings-compare" aria-label="配置冲突对照">
    <h3>草稿与服务器配置</h3>
    <p>输入尚未丢失。请对照后选择要保留的内容，选择后不会自动保存。</p>
    <template v-if="form.latest.value">
      <article v-for="key in differences" :key="key" class="settings-conflict-field">
        <strong>{{labels[key]||'其他设置'}}</strong>
        <div class="settings-comparison">
          <p>草稿：{{display(form.draft.value[key],key)}}</p>
          <p>服务器：{{display(form.latest.value.value[key],key)}}</p>
        </div>
      </article>
      <p v-if="!differences.length">当前草稿与服务器配置一致，可以使用服务器配置完成本次核对。</p>

    </template>
    <div class="settings-actions">
      <el-button :disabled="!form.latest.value" @click="form.useServer">使用服务器配置</el-button>
      <el-button :disabled="!form.latest.value" @click="form.keepDraft">保留草稿并继续编辑</el-button>
      <el-button v-if="!form.latest.value" @click="form.load">核对结果</el-button>
    </div>
  </section>
</template>
