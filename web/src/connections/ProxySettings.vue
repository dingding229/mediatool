<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { onMounted, ref, onUnmounted } from 'vue';
import { request } from '../api/client';
import { isConflict } from './model';
interface Proxy {
  url: string;
  tmdb: boolean;
  sites: boolean;
  revision: number;
}
const saved = ref<Proxy | null>(null), input = ref(''), tmdb = ref(false), sites = ref(false), clear = ref(false), busy = ref(false), error = ref(''), notice = ref(''), conflict = ref<Proxy | null>(null), blocked = ref(false);
const showSaved = ref(false);
async function copySaved() {
  try { if (!navigator.clipboard?.writeText) throw Error('unavailable'); await navigator.clipboard.writeText(saved.value?.url || ''); notice.value = '已复制，请妥善保管。'; }
  catch { notice.value = '复制失败，请在已显示的地址内手动选择并复制。'; }
}
let disposed = false;
async function load(preserve = false) {
  showSaved.value = false;
  try {
    const out = await request<Proxy>('/settings/network-proxy');
    if (!disposed) {
      if (preserve && saved.value && (input.value || clear.value || tmdb.value!==saved.value.tmdb || sites.value!==saved.value.sites)) {
        conflict.value=out;
        notice.value='已刷新配置，你的输入已保留，请对照后确认。';
        return;
      }
      saved.value = out;
      conflict.value = null;
      tmdb.value = out.tmdb;
      sites.value = out.sites;
      input.value = '';
      clear.value = false;
      blocked.value = false;
      error.value = '';
    }
  }
  catch {
    error.value = '代理设置读取失败，请重新读取。';
  }
}
async function save() {
  if (!saved.value || busy.value || conflict.value || blocked.value)
    return;
  showSaved.value = false;
  busy.value = true;
  error.value = '';
  notice.value = '';
  try {
    const out = await request<Proxy>('/settings/network-proxy', { method: 'PUT', body: JSON.stringify({ url: clear.value ? '' : input.value || saved.value.url, tmdb: tmdb.value, sites: sites.value, revision: saved.value.revision }) });
    saved.value = out;
    input.value = '';
    clear.value = false;
    notice.value = '代理设置已保存，新的请求使用所选路由。';
  }
  catch (e: any) {
    if (isConflict(e)) {
      try {
        conflict.value = await request<Proxy>('/settings/network-proxy');
        error.value = '配置已被修改，已重新读取。你的代理地址与路由选择已保留，请对照后确认。';
      }
      catch {
        error.value = '配置冲突后读取失败，输入已保留。';
        blocked.value = true;
      }
    }
    else if (e?.status >= 400 && e.status < 500) {
      error.value = e.message;
    }
    else {
      error.value = '保存未确认，请重新读取核对；输入已保留。';
      blocked.value = true;
    }
  }
  finally {
    busy.value = false;
  }
}
function adopt() {
  if (conflict.value) {
    saved.value = conflict.value;
    conflict.value = null;
    error.value = '已采用最新修订，你的输入未变。';
  }
}
onMounted(()=>load());
onUnmounted(() => {
  disposed = true;
  input.value = '';
  saved.value = null;
  conflict.value = null;
});
usePageRefresh(()=>load(true));
</script>
<template>
<section class="connection-manager" aria-label="网络代理">
  <header class="connection-section-heading">
    <div>
      <h2>网络代理</h2>
      <p>代理失效时明确报错，不静默回退直连。</p>
    </div>

  </header>
  <form v-if="saved" class="connection-editor" @submit.prevent="save">
    <p>{{ saved.url?'代理地址已设置（默认隐藏）':'尚未设置代理地址' }}</p>
    <label>
      代理地址
      <el-input v-model="input" type="password" aria-label="代理地址" autocomplete="new-password" placeholder="留空保留现有地址，填写新地址替换" />
    </label>
    <div v-if="saved.url" class="saved-secret-controls">
      <el-button @click="showSaved=!showSaved">{{ showSaved?'隐藏代理地址':'显示代理地址' }}</el-button>
      <template v-if="showSaved"><el-input :model-value="saved.url" aria-label="已保存代理地址" readonly /><el-button @click="copySaved">复制代理地址</el-button></template>
    </div>
    <el-checkbox v-model="clear">明确清除已保存的代理地址</el-checkbox>
    <el-checkbox v-model="tmdb">TMDB查询与海报使用代理</el-checkbox>
    <el-checkbox v-model="sites">PT站点搜索、RSS与站点请求使用代理</el-checkbox>
    <p>不会影响 115、Emby、CloudDrive2 或下载器。关闭两个路由即可停用代理。</p>
    <el-alert v-if="conflict" :title="`服务端最新路由：TMDB ${conflict.tmdb?'代理':'直连'}；PT ${conflict.sites?'代理':'直连'}。地址仅显示是否已设置：${conflict.url?'已设置':'未设置'}`" type="info" :closable="false" />
    <el-button v-if="conflict" @click="adopt">保留输入并采用最新修订</el-button>
    <el-button type="primary" native-type="submit" :loading="busy" :disabled="!!conflict||blocked">保存代理设置</el-button>
    <p v-if="notice" role="status">{{ notice }}</p>
  </form>
  <el-alert v-if="error" :title="error" type="warning" :closable="false" />
</section>
</template>
