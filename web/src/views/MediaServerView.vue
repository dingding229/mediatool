<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { request } from '../api/client'
import type { MediaProvider } from '../api/mediaServer'
const data = ref<{ contract: string; providers: MediaProvider[] }>()
const loading = ref(false), error = ref('')
const labels: Record<string, string> = { catalog: '媒体目录', memberAuth: '独立媒体账号', libraryPermissions: '媒体库权限', playbackTickets: '播放授权票据', progress: '播放进度' }
async function load() { loading.value = true; error.value = ''; try { data.value = await request('/media-server/providers') } catch (e) { error.value = e instanceof Error ? e.message : '媒体服务读取失败' } finally { loading.value = false } }
onMounted(load)
</script>
<template>
 <section class="page-heading"><div><span class="page-kicker">MEDIA SERVER / 扩展边界</span><h1>自研媒体库</h1><p>内置服务的接入入口；现有 Emby 继续运行，不会自动切换或改写 STRM。</p></div><el-button :loading="loading" @click="load">刷新状态</el-button></section>
 <el-alert v-if="error" :title="error" type="error" :closable="false"/>
 <section class="native-links panel"><div><h2>独立媒体用户</h2><p>媒体账号由管理员创建，只能访问明确授权的目录，不能访问管理后台。</p></div><div class="actions"><RouterLink class="el-button el-button--primary" to="/media-users">管理媒体用户</RouterLink><a class="el-button" href="/watch" target="_blank" rel="noopener">打开媒体用户入口</a></div></section>
 <section v-loading="loading" class="provider-grid" aria-label="媒体服务能力"><article v-for="provider in data?.providers" :key="provider.id" class="panel"><div class="provider-title"><h2>{{provider.name}}</h2><el-tag :type="provider.id==='builtin'?'warning':'info'">{{provider.id==='builtin'?'目录预览阶段':'外部服务'}}</el-tag></div><p>{{provider.note}}</p><dl><div><dt>运行方式</dt><dd>{{provider.runtime==='go-embedded'?'Go · 主程序内置':'外部 Emby'}}</dd></div><div><dt>接口版本</dt><dd>{{provider.contract}}</dd></div></dl><ul><li v-for="(available,key) in provider.capabilities" :key="key"><span>{{labels[key]||key}}</span><el-tag size="small" :type="available?'success':'info'">{{available?'已接入':'未实现'}}</el-tag></li></ul></article></section>
 <section class="panel architecture"><h2>后续扩展方式</h2><p>推荐 Go 继续实现服务端，Vue / TypeScript 承载媒体用户端。未来 TypeScript 服务可通过 Provider v1 适配，不需要改变用户及权限模型。</p><p>本轮目录按“电影 / 剧集”提供逻辑媒体库，仅展示已确认且文件在库的资产。物理目录规则、完整播放器、短期可撤销播放票据和观看进度仍待开发。</p><p>播放保留原始流 302 到 115 CDN 的方向，不引入服务器流量中转、HLS 或转码。</p><RouterLink to="/mediaLibrary">继续使用现有 Emby 媒体库</RouterLink></section>
</template>
<style scoped>
.native-links{display:flex;justify-content:space-between;align-items:center;gap:24px;margin-bottom:24px}.native-links p,.provider-grid p,.architecture p{color:var(--muted);line-height:1.8}.actions{display:flex;flex-wrap:wrap;gap:12px}.actions .el-button{margin:0;text-decoration:none}.provider-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:24px;margin-bottom:24px;min-height:180px}.provider-title{display:flex;align-items:center;justify-content:space-between;gap:12px}.provider-title h2{margin:0}.provider-grid dl div,.provider-grid li{display:flex;justify-content:space-between;gap:16px;padding:12px 0;border-bottom:1px solid var(--border)}.provider-grid dt{color:var(--muted)}.provider-grid dd{margin:0}.provider-grid ul{list-style:none;padding:0;margin:12px 0 0}.architecture a{color:var(--el-color-primary)}@media(max-width:700px){.provider-grid{grid-template-columns:1fr}.native-links{align-items:flex-start;flex-direction:column}.actions{width:100%}.actions .el-button{min-height:44px}}
</style>
