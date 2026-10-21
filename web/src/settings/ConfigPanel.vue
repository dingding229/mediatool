<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { computed, onMounted, onUnmounted, ref, watch, reactive } from 'vue';
import { request } from '../api/client';
import ConfirmAction from '../ui/ConfirmAction.vue';
import RevisionFeedback from './RevisionFeedback.vue';
import { useRevision, messageFor } from './revision';
const props = defineProps<{
  section: 'general' | 'naming' | 'automation';
}>();
const app = useRevision<Record<string, any>>('/settings/application'), access = useRevision<Record<string, string>>('/settings/access');
const a = app.draft, domains = access.draft, confirmCleanup = ref(false), preview = ref<{
  movie: string;
  series: string;
}>(), previewError = ref(''), previewing = ref(false);
const touched=reactive(new Set<string>());
function originValue(key:string){return touched.has(key)?domains.value?.[key]||'':domains.value?.[key]||effective.value?.[key]||''}
function setOrigin(key:string,value:string){touched.add(key);if(domains.value)domains.value[key]=value}
watch(()=>access.baseline.value?.revision,()=>{if(!access.changed.value)touched.clear()});
const effective = computed(() => access.latest.value?.effective ?? access.baseline.value?.effective);
const fields = [['managementUrl', '后台访问域名'], ['playbackUrl', '播放链接域名'], ['memberUrl', '内置媒体库域名'], ['embyUrl', 'Emby 网关域名']] as const;
const scraping = [['movie', '电影', ['nfo', 'poster', 'backdrop', 'logo']], ['series', '剧集', ['nfo', 'poster', 'backdrop', 'logo']], ['season', '季', ['nfo', 'poster']], ['episode', '集', ['nfo', 'thumb']]] as const;
const labels: Record<string, string> = { nfo: '媒体资料', poster: '海报', backdrop: '背景', logo: '标志', thumb: '缩略图' };
const proxyText = computed({ get: () => a.value?.trustedProxyCidrs?.join('\n') || '', set: (s: string) => {
    if (a.value)
      a.value.trustedProxyCidrs = s.split(/[\n,]/).map(x => x.trim()).filter(Boolean);
  } });
let timer: ReturnType<typeof setTimeout> | undefined, version = 0, alive = true;
async function previewNames() {
  const id = ++version;
  preview.value = undefined;
  if (!a.value)
    return;
  previewing.value = true;
  previewError.value = '';
  try {
    const out = await request<{
      movie: string;
      series: string;
    }>('/settings/naming-preview', { method: 'POST', body: JSON.stringify({ movieTemplate: a.value.movieTemplate, seriesTemplate: a.value.seriesTemplate }) });
    if (alive && id === version)
      preview.value = out;
  }
  catch (e) {
    if (alive && id === version)
      previewError.value = messageFor(e);
  }
  finally {
    if (alive && id === version)
      previewing.value = false;
  }
}
watch(() => [a.value?.movieTemplate, a.value?.seriesTemplate], () => {
  if (props.section !== 'naming')
    return;
  version++;
  preview.value = undefined;
  clearTimeout(timer);
  timer = setTimeout(() => void previewNames(), 400);
});
onUnmounted(() => {
  alive = false;
  version++;
  clearTimeout(timer);
});
async function saveNaming() {
  if (a.value?.cleanupAfterImport && !app.baseline.value?.value.cleanupAfterImport) {
    confirmCleanup.value = true;
    return;
  }
  await app.save(['movieTemplate', 'seriesTemplate', 'scraping', 'cleanupAfterImport']);
}
async function approveCleanup() {
  confirmCleanup.value = false;
  await app.save(['movieTemplate', 'seriesTemplate', 'scraping', 'cleanupAfterImport']);
}
onMounted(() => {
  void app.load();
  if (props.section === 'general')
    void access.load();
});
usePageRefresh(()=>Promise.all([app.load(),...(props.section==='general'?[access.load()]:[])]), props.section);
</script>
<template>
  <div class="settings-stack">
    <section v-if="section==='general'" class="settings-card" aria-label="访问与域名">
      <header>
        <div>
          <h2>访问与域名</h2>
          <p>为后台、播放和媒体库分别设置入口。</p>
        </div>

      </header>
      <RevisionFeedback :form="access"/>
      <form v-if="domains" @submit.prevent="access.save(fields.map(f=>f[0]))">
        <div class="settings-grid">
          <div v-for="[key,label] in fields" :key="key" class="settings-field">
            <label :for="`access-${key}`">{{label}}</label>
            <el-input :id="`access-${key}`" :model-value="originValue(key)" @update:model-value="(value:string)=>setOrigin(key,value)" autocomplete="url"/>
<small v-if="originValue(key)!==(effective?.[key]||'')">尚未保存，当前使用 {{effective?.[key]||'部署地址'}}。</small>
          </div>
        </div>
        <p>请填写完整域名，可带端口；留空使用部署地址。需要自行配置域名解析、证书和反向代理。</p>
        <p>
          修改不覆盖已有播放文件。
          <RouterLink to="/files?tab=tools">前往 STRM 工具预览修复</RouterLink>
        </p>

        <footer>
          <el-button type="primary" native-type="submit" :loading="access.busy.value" :disabled="access.conflict.value||!access.changed.value" class="mt-save-button">保存域名设置</el-button>
        </footer>
      </form>
    </section>
    <section class="settings-card" :aria-label="section==='general'?'访客地址识别':section==='naming'?'命名与入库':'搜索与订阅'">
      <header>
        <div>
          <h2>{{section==='general'?'访客地址识别':section==='naming'?'命名与入库':'搜索与订阅'}}</h2>
          <p>{{section==='naming'?'调整新入库媒体的命名与资料，不会自动修改已有文件。':'只保存本分区的改动，不影响其他设置。'}}</p>
        </div>

      </header>
      <RevisionFeedback :form="app"/>
      <form v-if="a" @submit.prevent="section==='naming'?saveNaming():app.save(section==='general'?['trustedProxyCidrs']:['searchCacheSeconds','tmdbCacheSeconds','subscriptionCron'])">
        <template v-if="section==='general'">
          <label for="trusted-proxies">可信反代 IP / CIDR（每行一个）</label>
          <el-input id="trusted-proxies" v-model="proxyText" type="textarea" :rows="4"/>
          <p>只填写实际代理来源。错误信任会导致访客地址被伪造；留空仅记录直接连接地址。</p>

        </template>
        <template v-else-if="section==='naming'">
          <div class="settings-field">
            <label for="movie-naming-template">电影命名模板</label>
            <el-input id="movie-naming-template" v-model="a.movieTemplate"/>
          </div>
          <div class="settings-field">
            <label for="series-naming-template">剧集命名模板</label>
            <el-input id="series-naming-template" v-model="a.seriesTemplate"/>
          </div>
          <details>
            <summary>可用命名字段</summary>
            <p v-pre>{title}、{year}、{videoFormat}、{season_episode}、{season}、{episode}、{resolution}、{source}、{codec}、{group}、{original}、{ext}</p>
            <p>支持相对文件夹，自动保留扩展名；不支持绝对路径和上级目录。实际输出会保留版本标识。</p>
          </details>
          <section class="settings-preview" aria-label="命名实时预览" aria-live="polite">
            <strong>命名预览</strong>
            <p v-if="previewing">正在预览…</p>
            <p v-if="previewError" role="alert">{{previewError}}</p>
            <p>
              电影：
              <code>{{preview?.movie||'—'}}</code>
            </p>
            <p>
              剧集：
              <code>{{preview?.series||'—'}}</code>
            </p>
            <small>预览由当前模板生成，不操作真实文件。</small>
            <el-button @click="previewNames">预览命名模板</el-button>
          </section>
          <fieldset v-if="a.scraping">
            <legend>入库时生成的资料</legend>
            <p>仅影响下次确认入库。关闭不删除旧资料；图片获取失败会阻止本次写入，缺图时显示原因。</p>
            <div class="settings-grid">
              <div v-for="[key,label,options] in scraping" :key="key">
                <h3>{{label}}</h3>
                <label v-for="option in options" :key="option" class="settings-choice">
                  <input v-model="a.scraping[key][option]" type="checkbox"/>
                  <span>{{labels[option]}}</span>
                </label>
              </div>
            </div>
          </fieldset>
          <label class="settings-choice">
            <input v-model="a.cleanupAfterImport" type="checkbox"/>
            <span>新任务上传校验并入库后，清理本地文件和下载种子</span>
          </label>
          <p>有做种要求或要求未知时仍需人工确认。共用文件、缺少校验或归属信息时不自动删除；不追溯历史任务。</p>
        </template>
        <template v-else>
          <div class="settings-grid">
            <div class="settings-field">
              <label for="pt-cache-seconds">站点搜索缓存有效期（秒）</label>
              <el-input-number id="pt-cache-seconds" v-model="a.searchCacheSeconds" :min="0" :max="86400"/>
            </div>
            <div class="settings-field">
              <label for="tmdb-cache-seconds">TMDB 搜索缓存有效期（秒）</label>
              <el-input-number id="tmdb-cache-seconds" v-model="a.tmdbCacheSeconds" :min="0" :max="2592000"/>
            </div>
          </div>
          <p>
            0 表示禁用缓存。修改有效期不会删除搜索历史。
            <RouterLink to="/ops?tab=cache">查看与清理缓存</RouterLink>
          </p>
          <div class="settings-field">
            <label for="subscription-cron">订阅调度时间</label>
            <el-input id="subscription-cron" v-model="a.subscriptionCron"/>
            <small>五字段：分、时、日、月、周；上海时区，间隔至少 5 分钟。不自动启用已停用的订阅。</small>
          </div>
        </template>
        <footer>
          <el-button native-type="submit" type="primary" :loading="app.busy.value" :disabled="app.conflict.value||!app.changed.value" class="mt-save-button">{{section==='general'?'保存访客地址设置':section==='naming'?'保存命名与入库设置':'保存搜索订阅设置'}}</el-button>
        </footer>
      </form>
    </section>
    <ConfirmAction class="settings-dialog" v-model:visible="confirmCleanup" title="启用上传后清理" description="仅新任务全部校验并入库后生效。有做种要求或要求未知时，仍须逐项确认完成做种。" impact-scope="允许删除符合条件的本地文件及下载种子，不自动清理历史任务。" confirm-text="确认启用并保存" @confirm="approveCleanup"/>
  </div>
</template>
