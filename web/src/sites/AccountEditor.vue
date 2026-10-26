<script setup lang="ts">
import { onUnmounted, reactive, ref, watch, computed } from 'vue';
import { request } from '../api/client';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import StateView from '../ui/StateView.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText } from '../workflows/readiness';
import { accountPayload, safeAccount } from './accountModel';
const emit = defineEmits<{
  saved: [
    message: string
  ];
}>(), visible = ref(false), loading = ref(false), loadError = ref<unknown>(null), templates = ref<any[]>([]), accounts = ref<any[]>([]), current = ref<any>(null), latest = ref<any>(null), notice = ref(''), form = reactive<any>({ id: '', definitionId: '', cookie: '', userAgent: '', userId: '', rssUrl: '', passkey: '', rediscover: false, revision: 0 }), op = reactive(useOperation('site-editor'));
const missingTemplate = computed(() => !!form.id && !loading.value && !loadError.value && !!form.definitionId && !templates.value.some(t => t.id === form.definitionId));
let generation = 0, savedRSS = '';
async function open(id = '') {
  savedRSS = '';
  form.id = id;
  form.definitionId = '';
  form.cookie = '';
  form.rssUrl = '';
  form.passkey = '';
  form.userId = '';
  form.userAgent = '';
  form.rediscover = false;
  form.revision = 0;
  current.value = null;
  latest.value = null;
  notice.value = '';
  op.error = null;
  visible.value = true;
  await load(false);
}
async function load(preserve = true) {
  const version = ++generation;
  loading.value = true;
  loadError.value = null;
  try {
    const [catalog, list, ua]: any = await Promise.all([request('/pt/templates'), request('/pt/accounts'), request('/pt/ua')]);
    if (version !== generation || !visible.value)
      return;
    accounts.value = list.map(safeAccount);
    const raw = list.find((a: any) => a.id === form.id);
    templates.value = catalog.templates.filter((t: any) => t.loaded || t.id === raw?.definitionId);
    if (raw) {
      const safe = safeAccount(raw);
      savedRSS = raw.values?.rssUrl || '';
      current.value = safe;
      if (!preserve) {
        Object.assign(form, { definitionId: safe.definitionId, revision: safe.revision, userAgent: safe.settings.userAgent || ua.userAgent, userId: safe.settings.userId || '', cookie: raw.values?.cookie || '', rssUrl: raw.values?.rssUrl || '', passkey: raw.values?.passkey || '' });
      }
      else if (form.revision !== safe.revision)
        latest.value = safe;
    }
    else if (!preserve)
      form.userAgent = ua.userAgent;
    if (op.pending) {
      const p = JSON.parse(op.pending.key), proof = list.find((r: any) => p.id ? r.id === p.id : r.definitionId === p.definitionId);
      if (proof && proof.revision !== p.revision) {
        op.clear();
        if (!p.id) {
          form.id = proof.id;
          current.value = safeAccount(proof);
          latest.value = current.value;
          savedRSS = proof.values?.rssUrl || '';
        }
        notice.value = '已重新读取站点记录，请核对后继续。';
      }
    }
  }
  catch (e) {
    if (version === generation)
      loadError.value = e;
  }
  finally {
    if (version === generation)
      loading.value = false;
  }
}
async function captureUA() {
  try {
    form.userAgent = (await request<any>('/pt/ua')).userAgent;
  }
  catch (e) {
    op.error = e;
  }
}
async function save() {
  if (op.pending || loading.value || loadError.value || latest.value || missingTemplate.value)
    return;
  if (!form.definitionId || !form.id && !form.cookie.trim()) {
    op.error = Error('请选择已加载的模板并填写登录 Cookie');
    return;
  }
  const body = accountPayload(form, savedRSS), out = await op.run(JSON.stringify({ id: form.id, definitionId: form.definitionId, revision: form.revision }), () => form.id ? request(`/pt/accounts/${form.id}`, { method: 'PUT', body: JSON.stringify(body) }) : request('/pt/accounts', { method: 'POST', body: JSON.stringify(body) }));
  await load(true);
  if (out !== undefined && !loadError.value && !op.pending) {
    visible.value = false;
    emit('saved', out.rssState === 'ready' ? '站点账号已保存并启用，订阅地址已获取。' : out.rssMessage || '站点账号已保存并启用，请在账号中核对订阅地址。');
  }
  else if (op.error) {
    if (!latest.value && form.id && loadError.value)
      latest.value = { revision: null };
    notice.value = '保存未完成或配置已修改，输入已保留。请重新读取后核对当前修订。';
  }
}
watch(visible, v => {
  if (!v) {
    generation++;
    form.cookie = '';
    form.rssUrl = '';
    form.passkey = '';
    savedRSS = '';
  }
});
onUnmounted(() => {
  generation++;
  savedRSS = '';
  form.cookie = '';
  form.rssUrl = '';
  form.passkey = '';
});
defineExpose({ open, load, save, form, current, latest, op });
</script>
<template>
  <el-dialog v-model="visible" :title="form.id?'编辑站点账号':'添加站点账号'" width="min(740px,94vw)" :close-on-click-modal="false" :show-close="!op.busy" class="media-dialog u5-dialog">
    <ResourceState :loading="loading" :error="loadError" @reload="load(true)"/>
    <StateView v-if="missingTemplate" state="unsupported" title="此账号的模板当前不可用" description="账号和登录信息仍保留，可查看和编辑保存值；不会自动替换模板。"/>
    <form class="u5-form" @submit.prevent="save">
      <label>
        站点模板
        <select v-model="form.definitionId" aria-label="站点模板" :disabled="!!form.id||loading||op.busy||!!op.pending">
          <option value="">请选择已加载模板</option>
          <option v-for="site in templates" :key="site.id" :value="site.id" :disabled="!form.id&&accounts.some(a=>a.definitionId===site.id)">{{ site.name }}{{ !form.id&&accounts.some(a=>a.definitionId===site.id)?' · 已添加':'' }}</option>
        </select>
      </label>
      <p v-if="form.definitionId">{{ templates.find(t=>t.id===form.definitionId)?.name||'—' }} · {{ templates.find(t=>t.id===form.definitionId)?.domain||'—' }}</p>
      <p>同一站点只添加一个账号；请先在站点模板标签加载所需模板。</p>
      <label>
        登录 Cookie
        <input v-model="form.cookie" type="text" aria-label="站点 Cookie" autocomplete="off" spellcheck="false" placeholder="填写此站点的登录 Cookie" :disabled="loading||op.busy||!!op.pending"/>
      </label>
      <label>
        浏览器标识（UA）
        <textarea v-model="form.userAgent" rows="3" aria-label="站点 User-Agent" :disabled="loading||op.busy||!!op.pending"/>
      </label>
      <el-button :disabled="op.busy||!!op.pending" @click="captureUA">使用当前浏览器 UA</el-button>
      <label>
        站内用户 ID（可选）
        <input v-model="form.userId" aria-label="站点用户ID" :disabled="loading||op.busy||!!op.pending"/>
      </label>
      <label>
        RSS 地址
        <input v-model="form.rssUrl" type="text" aria-label="站点 RSS 地址" autocomplete="off" spellcheck="false" placeholder="留空尝试自动获取" :disabled="loading||op.busy||!!op.pending"/>
      </label>
      <label>
        订阅密钥
        <input v-model="form.passkey" type="text" aria-label="站点订阅密钥" autocomplete="off" spellcheck="false" :disabled="loading||op.busy||!!op.pending"/>
      </label>
      <label class="u5-check">
        <input v-model="form.rediscover" type="checkbox" :disabled="op.busy||!!op.pending"/>
        重新获取订阅地址
      </label>
      <p>保存前会验证站点登录；保存后账号会启用。未取得订阅地址时保留已有内容，并在账号中提示。</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <section v-if="latest">
        <el-button v-if="latest.revision==null&&!op.phase" @click="load(true)">核对账号配置</el-button>
        <p>当前账号已重新读取；你的 Cookie、RSS 地址和订阅密钥输入仍保留。请核对后采用最新修订。</p>

        <el-button :disabled="latest.revision==null" @click="form.revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
      </section>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'账号变更已排队，尚未核验完成。':'正在核对上次保存，不重复提交账号。'" @refresh="load(true)"/>
    </form>
    <template #footer>
      <el-button :disabled="op.busy" @click="visible=false">取消</el-button>
      <el-button :loading="op.busy" :disabled="!!op.pending||!!loadError||!!latest||loading||missingTemplate" @click="save" type="primary" class="mt-save-button">验证并保存</el-button>
    </template>
  </el-dialog>
</template>
