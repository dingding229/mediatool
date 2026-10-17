<script setup lang="ts">
import { onMounted, ref } from 'vue'
import NativeMediaSettings from '../components/NativeMediaSettings.vue'
import NativeActiveSessions from '../components/NativeActiveSessions.vue'
import NativeProcessing from '../components/NativeProcessing.vue'
import { request } from '../api/client'
import type { MediaProvider } from '../api/mediaServer'
const data = ref<{ contract: string; providers: MediaProvider[] }>()
const settingsVersion=ref(0), processingVersion=ref(0)
const loading = ref(false), error = ref('')
const nativeCapabilities={catalog:true,memberAuth:true,libraryPermissions:true,playbackTickets:true,progress:true}
const labels: Record<string, string> = { catalog: '媒体目录', memberAuth: '媒体用户模型', libraryPermissions: '媒体库权限', playbackTickets: '播放授权票据', progress: '播放进度' }
async function load() { loading.value = true; error.value = ''; try { data.value = await request('/media-server/providers') } catch (e) { error.value = e instanceof Error ? e.message : '媒体服务读取失败' } finally { loading.value = false } }
onMounted(load)
</script>
<template>
 <section class="page-heading"><div><span class="page-kicker">MEDIA SERVER / 扩展边界</span><h1>自研媒体库</h1><p>内置服务的接入入口；现有 Emby 继续运行，不会自动切换或改写 STRM。</p></div><el-button :loading="loading" @click="load">刷新状态</el-button></section>
 <el-alert v-if="error" :title="error" type="error" :closable="false"/>
 <NativeMediaSettings :key="settingsVersion" @saved="processingVersion++"/><NativeActiveSessions/><NativeProcessing :key="processingVersion" @settings-saved="settingsVersion++"/>
 <section class="native-links panel"><div><h2>媒体用户管理</h2><p>媒体账号由管理员创建，可通过本端口 Emby 认证使用播放器；账号与库权限由管理员管理，不提供媒体用户网页登录入口。</p></div><div class="actions"><RouterLink class="el-button el-button--primary" to="/media-users">管理媒体用户</RouterLink></div></section>
 <section v-loading="loading" class="provider-grid" aria-label="媒体服务能力"><article v-for="provider in data?.providers" :key="provider.id" class="panel"><div class="provider-title"><h2>{{provider.name}}</h2><el-tag :type="provider.id==='builtin'?'warning':'info'">{{provider.id==='builtin'?'原生播放已接入':'外部服务'}}</el-tag></div><p>{{provider.id==='builtin'?'浏览、认证、权限、302 播放与观看进度已接入；媒体预处理和在线会话由上方管理。':provider.note}}</p><dl><div><dt>运行方式</dt><dd>{{provider.runtime==='go-embedded'?'Go · 主程序内置':'外部 Emby'}}</dd></div><div><dt>接口版本</dt><dd>{{provider.contract}}</dd></div></dl><ul><li v-for="(available,key) in (provider.id==='builtin'?nativeCapabilities:provider.capabilities)" :key="key"><span>{{labels[key]||key}}</span><el-tag size="small" :type="available?'success':'info'">{{available?'已接入':'未实现'}}</el-tag></li></ul></article></section>
 <section class="panel architecture"><h2>后续扩展方式</h2><p>推荐 Go 继续实现服务端，Vue / TypeScript 承载媒体用户端。未来 TypeScript 服务可通过 Provider v1 适配，不需要改变用户及权限模型。</p><p>媒体库已按目录分类同步，仅展示已确认、文件存在且有有效 STRM 的资产；浏览、预处理、302 播放、内嵌字幕和观看进度已接入。</p><p>播放保留原始流 302 到 115 CDN 的方向，不引入服务器流量中转、HLS 或转码。</p><RouterLink to="/mediaLibrary">继续使用现有 Emby 媒体库</RouterLink></section>
</template>
<style scoped>
.native-links{display:flex;justify-content:space-between;align-items:center;gap:24px;margin-bottom:24px}.native-links p,.provider-grid p,.architecture p{color:var(--muted);line-height:1.8}.actions{display:flex;flex-wrap:wrap;gap:12px}.actions .el-button{margin:0;text-decoration:none}.provider-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:24px;margin-bottom:24px;min-height:180px}.provider-title{display:flex;align-items:center;justify-content:space-between;gap:12px}.provider-title h2{margin:0}.provider-grid dl div,.provider-grid li{display:flex;justify-content:space-between;gap:16px;padding:12px 0;border-bottom:1px solid var(--border)}.provider-grid dt{color:var(--muted)}.provider-grid dd{margin:0}.provider-grid ul{list-style:none;padding:0;margin:12px 0 0}.architecture a{color:var(--el-color-primary)}@media(max-width:700px){.provider-grid{grid-template-columns:1fr}.native-links{align-items:flex-start;flex-direction:column}.actions{width:100%}.actions .el-button{min-height:44px}}
</style>
