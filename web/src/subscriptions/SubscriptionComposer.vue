<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { ElIcon } from 'element-plus'
import { Search, Check, ArrowLeft, ArrowRight } from '@element-plus/icons-vue'
import { request } from '../api/client'
import { resourceSources } from '../utils/resource-source'
import RunReview from './RunReview.vue'
import StateView from '../ui/StateView.vue'
import { asError, discovery, payloadFor, quality, stateFor } from './model'
import type { Account, Draft, Media, PendingRun, Run } from './model'
const visible = defineModel<boolean>('visible', { default: false })
const draft = defineModel<Draft>('draft', { required: true })
const props = defineProps<{ accounts: Account[]; accountError: Error | null; saving: boolean; error: string; previewReady?: boolean; previewRun?: Run | null; previewError?: Error | null; pending?: PendingRun | null }>()
const emit = defineEmits<{ save: [draft: Draft]; reload: []; retryAccounts: []; preview: [draft: Draft]; reconcile: [] }>()
const steps = ['选影视', '站点和筛选', '预演', '保存'], editingExisting = ref(false), step = ref(0), query = ref(''), mediaType = ref('movie')
const matches = ref<Media[]>([]), finding = ref(false), findError = ref<Error | null>(null), searched = ref(false)
const body = ref<HTMLElement | null>(null)
const cache = ref<{ cached: boolean; seconds: number } | null>(null), validation = ref('')
const siteNames = computed(() => draft.value.accountIds.map(id => props.accounts.find(a => a.id === id)?.name || '站点暂不可用').join('、') || '—')
watch(step, () => body.value?.scrollTo({ top: 0 }), { flush: 'post' })
let sequence = 0, disposed = false
watch(visible, open => { if (!open) { ++sequence; finding.value = false }; if (open) { editingExisting.value = !!draft.value.id; step.value = draft.value.media ? 1 : 0; query.value = ''; matches.value = []; searched.value = false; cache.value = null; validation.value = ''; findError.value = null } })
watch(mediaType, () => { ++sequence; matches.value = []; searched.value = false; cache.value = null; findError.value = null; draft.value.media = null })
async function findMedia() {
  if (!query.value.trim() || finding.value) return
  const seq = ++sequence; finding.value = true; searched.value = false; findError.value = null; cache.value = null; matches.value = []; draft.value.media = null
  try {
    const out = await request<{ items: Media[]; cached: boolean; cacheSeconds: number }>(`/search/media?mediaType=${mediaType.value}&q=${encodeURIComponent(query.value)}`)
    if (!disposed && seq === sequence) { matches.value = out.items; searched.value = true; cache.value = { cached: out.cached, seconds: out.cacheSeconds } }
  } catch (e) { if (!disposed && seq === sequence) findError.value = asError(e) }
  finally { if (!disposed && seq === sequence) finding.value = false }
}
function choose(media: Media) { draft.value.media = media; if (!draft.value.keyword) draft.value.keyword = media.title }
function advance() {
  if (step.value === 0 && draft.value.media) step.value = 1
  else { try { payloadFor(draft.value); validation.value = ''; if (step.value === 1) step.value = 2; else if (props.previewReady) step.value = 3 } catch (e) { validation.value = asError(e).message } }
}
onUnmounted(() => { disposed = true; ++sequence })
</script>
<template>
  <el-dialog v-model="visible" :title="editingExisting ? '编辑订阅' : '新建订阅'" width="min(1000px, 96vw)" class="subscription-composer" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving">
    <div class="composer-grid">
      <nav class="composer-progress" aria-label="创建订阅步骤">
        <ol>
          <li v-for="(label, index) in steps" :key="label" :class="{ current: step === index, completed: step > index }" :aria-current="step === index ? 'step' : undefined">
            <span>{{ index + 1 }}</span>
            <div><strong>{{ label }}</strong><small>{{ ['明确要跟进哪部影视', '决定去哪找、下载什么', '核对真实资源决策', '确认状态与下载保护'][index] }}</small></div>
          </li>
        </ol>
        <p>预演会先保存为停用草稿，退出向导后仍会保留。最后确认是否启用。</p>
      </nav>
      <form ref="body" class="composer-body" @submit.prevent="step < 3 ? advance() : emit('save', draft)">
        <div class="composer-heading"><span>{{ step + 1 }} / 4</span><h2>{{ ['先找到你想跟进的影视', '把资源筛选交给规则', '先看规则会选中什么', '确认保存与启用状态'][step] }}</h2><p>{{ ['选定准确的媒体身份，避免同名作品混淆。', '选择站点和质量要求；无法识别的受限属性不会被当作匹配。', '服务端只预演已保存的规则；先保存为停用草稿，不会自动下载。', '预演草稿已经保留。确认保存，可保持停用或显式启用。'][step] }}</p></div>
        <template v-if="step === 0">
          <div class="composer-media-search">
            <el-select v-model="mediaType" :disabled="finding" aria-label="订阅媒体类型"><el-option label="电影" value="movie" /><el-option label="剧集" value="series" /></el-select>
            <el-input v-model="query" :prefix-icon="Search" aria-label="订阅TMDB关键词" placeholder="输入准确的影视名称" @keydown.enter.prevent="findMedia" />
            <el-button :disabled="!query.trim()" :loading="finding" @click="findMedia">查询TMDB</el-button>
          </div>
          <p v-if="cache" class="composer-cache">{{ cache.cached ? '来自TMDB缓存' : '已查询TMDB' }} · 缓存有效期 {{ cache.seconds }} 秒（0 表示禁用缓存）</p>
          <StateView v-if="finding" state="loading" title="正在匹配影视…" />
          <StateView v-else-if="findError" :state="stateFor(findError)" title="影视查询未完成" :description="findError.message" @retry="findMedia" />
          <StateView v-else-if="searched && !matches.length" state="empty" title="没有找到匹配的影视" description="试试完整片名或切换电影／剧集后重新搜索。" />
          <div v-else-if="matches.length" class="composer-matches" role="group" aria-label="匹配影视">
            <button v-for="media in matches" :key="media.id" type="button" :aria-pressed="draft.media?.id === media.id" @click="choose(media)">
              <span><strong>{{ media.title }}</strong><small>{{ media.mediaType === 'movie' ? '电影' : '剧集' }} · {{ media.year || '—' }} · TMDB {{ media.id }}</small></span>
              <el-icon v-if="draft.media?.id === media.id"><Check /></el-icon>
            </button>
          </div>
          <p v-else class="composer-search-hint">从一部想看的影视开始，搜索后即可选择。</p>
        </template>
        <template v-else-if="step === 1">
          <div class="composer-target"><strong>{{ draft.media?.title || '—' }}</strong><span>{{ draft.media?.mediaType === 'movie' ? '电影' : '剧集' }} · {{ draft.media?.year || '—' }}</span><small v-if="draft.id">媒体身份已绑定，编辑时不可更换。</small></div>
          <fieldset>
            <legend>从哪些站点寻找</legend>
            <StateView v-if="accountError" :state="stateFor(accountError)" title="站点账号暂不可用" :description="accountError.message" @retry="emit('retryAccounts')" />
            <p v-else-if="!accounts.length">暂无可用账号，请先在站点页面添加并启用账号。</p>
            <el-checkbox-group v-model="draft.accountIds" aria-label="PT账号"><el-checkbox v-for="a in accounts" :key="a.id" :value="a.id">{{ a.name }}{{ a.secretFields?.includes('rssUrl') ? ' · 已配RSS' : '' }}</el-checkbox></el-checkbox-group>
          </fieldset>
          <div class="composer-fields">
            <label>发现方式<el-select v-model="draft.source" aria-label="发现方式"><el-option label="周期搜索" value="search" /><el-option label="RSS新条目" value="rss" /><el-option label="搜索 + RSS" value="both" /></el-select></label>
            <label>站点搜索词<el-input v-model="draft.keyword" aria-label="站点搜索词" /></label>
          </div>
          <p class="composer-help">搜索词会发给站点，下面的正则仅在本地筛选。RSS 地址在站点账号中加密保存；首次默认只建立基线，预演不会改变基线。执行周期由系统统一设置。</p>
          <div class="composer-fields quality-fields">
            <label>
              分辨率
              <el-select v-model="draft.resolution" aria-label="订阅分辨率" placeholder="不限"><el-option label="不限" value="" /><el-option v-for="x in ['480p','576p','720p','1080i','1080p','1440p','2160p','4320p']" :key="x" :label="x" :value="x" /></el-select>
            </label>
            <label>资源来源<el-select v-model="draft.releaseSource" aria-label="订阅资源来源" placeholder="不限"><el-option label="不限" value="" /><el-option v-for="s in resourceSources" :key="s.value" :label="s.label" :value="s.value" /></el-select></label>
            <label>编码<el-select v-model="draft.codec" aria-label="订阅编码" placeholder="不限"><el-option label="不限" value="" /><el-option v-for="x in ['h264','h265','av1','mpeg2']" :key="x" :label="x" :value="x" /></el-select></label>
          </div>
          <el-checkbox v-model="draft.regexKeywords">使用正则匹配包含／排除关键词</el-checkbox>
          <div class="composer-fields">
            <label>包含关键词（每行一条）<el-input v-model="draft.includeText" type="textarea" :rows="3" aria-label="订阅包含关键词" /></label>
            <label>排除关键词（每行一条）<el-input v-model="draft.excludeText" type="textarea" :rows="3" aria-label="订阅排除关键词" /></label>
          </div>
          <div v-if="draft.media?.mediaType === 'series'" class="composer-fields">
            <label>目标季（0 为特别篇）<el-input-number v-model="draft.season" :min="0" :max="999" aria-label="目标季" /></label>
            <label>目标集（留空跟进已播集）<el-input v-model="draft.episodeText" aria-label="目标集" placeholder="使用逗号分隔集数" /></label>
          </div>
          <fieldset class="composer-safeguards">
            <legend>下载保护</legend>
            <el-checkbox v-model="draft.freeOnly">只下载已确认免费资源</el-checkbox>
            <el-checkbox v-model="draft.rejectHR">拒绝下载H&amp;R资源</el-checkbox>
            <p>H&amp;R 信息未知时始终拒绝自动下载。关闭此项只允许已明确标记的 H&amp;R 资源，请承担相应做种义务。</p>
            <el-checkbox v-model="draft.includeHistory">允许首次RSS历史补抓</el-checkbox>
            <p>RSS 缺少促销数据时无法满足免费限制；已入库或正在下载的季集不会重复申请，原做种文件不受影响。</p>
          </fieldset>
        </template>
        <template v-else-if="step === 2">
          <el-alert title="此操作会保存停用草稿；编辑已启用订阅时也会先停用。已有下载和文件不受影响，退出向导后草稿仍保留。" type="info" :closable="false" />
          <el-button :disabled="!!pending || !!accountError || error.includes('刷新列表')" :loading="saving" @click="emit('preview', draft)">{{ previewRun ? '重新保存停用草稿并预演' : '保存停用草稿并预演' }}</el-button>
          <p v-if="pending" role="status">{{ pending.status === 'locked' ? '正在提交，尚未确认排队。' : pending.status === 'queued' ? '预演已排队，尚未完成。' : '预演回执未知，请核对后再继续，不会重复提交。' }}</p>
          <RunReview v-if="previewRun || previewError" compact :run="previewRun || null" :loading="false" :error="previewError || null" @refresh="emit('reconcile')" />
          <el-button v-if="pending?.status === 'reconciling'" @click="emit('reconcile')">核对预演状态</el-button>
        </template>
        <template v-else>
          <dl class="composer-review">
            <div><dt>跟进影视</dt><dd>{{ draft.media?.title || '—' }}</dd></div>
            <div><dt>监控站点</dt><dd>{{ siteNames }}</dd></div>
            <div><dt>发现方式</dt><dd>{{ discovery[draft.source] || '—' }}</dd></div>
            <div><dt>质量要求</dt><dd>{{ quality(draft) }}</dd></div>
            <div><dt>站点搜索词</dt><dd>{{ draft.keyword || '—' }}</dd></div>
            <div><dt>{{ draft.regexKeywords ? '正则筛选' : '关键词筛选' }}</dt><dd>包含：{{ draft.includeText || '—' }}<br />排除：{{ draft.excludeText || '—' }}</dd></div>
            <div v-if="draft.media?.mediaType === 'series'"><dt>跟进范围</dt><dd>第 {{ draft.season }} 季 · {{ draft.episodeText || '已播集' }}</dd></div>
          </dl>
          <div class="composer-enable"><el-checkbox v-model="draft.enabled" :disabled="previewRun?.state !== 'succeeded'">保存后启用自动下载</el-checkbox><p>{{ draft.enabled ? '你已选择直接启用。请确认以上匹配规则与下载保护符合预期。' : '预演已完成。可以保持停用；确认匹配结果后，也可显式启用。' }}</p></div>
        </template>
        <el-alert v-if="validation || error" :title="error || validation" type="error" :closable="false" />
        <el-button v-if="error.includes('刷新列表')" @click="emit('reload')">刷新列表并核对</el-button>
        <div class="composer-footer">
          <el-button v-if="step > (draft.id ? 1 : 0)" :icon="ArrowLeft" :disabled="saving || !!pending" @click="step--">上一步</el-button>
          <span v-else />
          <el-button v-if="step < 3" type="primary" :disabled="finding || (step === 0 ? !draft.media : step === 1 ? !draft.accountIds.length || !!accountError : !previewReady)" @click="advance">{{ step === 0 ? '选择站点与规则' : step === 1 ? '进入预演' : '确认并继续' }}<el-icon><ArrowRight /></el-icon></el-button>
          <el-button v-else type="primary" native-type="submit" :loading="saving" :disabled="!previewReady || !!pending || !!accountError || error.includes('刷新列表')">保存订阅</el-button>
        </div>
      </form>
    </div>
  </el-dialog>
</template>
