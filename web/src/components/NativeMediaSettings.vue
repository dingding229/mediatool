<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getNativeServerSettings, saveNativeServerSettings, listNativeLibraryMappings, type NativeServerSettings, type NativeLibrary, type ManagementPage } from '../api/mediaServer'
const emit = defineEmits<{ saved: [] }>()
const settings = ref<NativeServerSettings>(), form = reactive({ serverName: '', enabled: false })
const loading = ref(false), saving = ref(false), error = ref(''), page = ref(1)
const libraries = ref<ManagementPage<NativeLibrary>>({ items: [], total: 0, startIndex: 0, limit: 50 })
async function loadLibraries() { libraries.value = await listNativeLibraryMappings((page.value - 1) * 50) }
async function load() {
 loading.value = true; error.value = ''
 try { settings.value = await getNativeServerSettings(); Object.assign(form, settings.value); await loadLibraries() }
 catch (e) { error.value = e instanceof Error ? e.message : '媒体库设置读取失败' }
 finally { loading.value = false }
}
async function changePage() {
 loading.value = true; error.value = ''
 try { await loadLibraries() } catch (e) { error.value = e instanceof Error ? e.message : '映射读取失败' } finally { loading.value = false }
}
async function save() {
 if (!settings.value) return
 if (!form.serverName.trim() || [...form.serverName.trim()].length > 100) { error.value = '服务名称需为1–100个字符'; return }
 saving.value = true; error.value = ''
 try {
  await saveNativeServerSettings({ serverName: form.serverName.trim(), enabled: form.enabled, revision: settings.value.revision })
  ElMessage.success('内置媒体库设置已保存'); await load(); emit('saved')
 } catch (e) { error.value = e instanceof Error ? e.message : '保存失败，请刷新后重试' }
 finally { saving.value = false }
}
onMounted(load)
</script>
<template>
 <section class="panel media-settings" v-loading="loading" aria-label="内置媒体库设置">
  <div class="heading"><div><h2>内置媒体库连接</h2><p>只管理本程序的 Emby 兼容服务，不更改外部 Emby 或已有 STRM。</p></div><el-button :disabled="saving" @click="load">刷新连接设置</el-button></div>
  <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  <el-form label-position="top" @submit.prevent="save">
   <el-form-item label="服务名称（ServerName）" for="native-server-name"><el-input id="native-server-name" v-model="form.serverName" maxlength="100" :disabled="!settings || saving"/></el-form-item>
   <el-form-item label="内置媒体库" for="native-library-enabled"><el-switch id="native-library-enabled" v-model="form.enabled" aria-label="启用内置媒体库" :disabled="!settings || saving"/><span class="switch-label">{{form.enabled?'启用':'停用'}}</span></el-form-item>
   <p class="hint">停用时除公共发现信息与 Ping 外均返回 503，探测和截图均不生效；配置、授权及媒体文件保留。</p>
   <el-button type="primary" :loading="saving" :disabled="!settings || loading" @click="save">保存媒体库设置</el-button>
  </el-form>
  <dl v-if="settings" class="connection"><div><dt>播放器服务地址</dt><dd>{{settings.publicUrl || '未配置；请在访问设置中填写媒体端口外部地址'}}</dd></div><div><dt>监听端口</dt><dd>{{settings.listenPort || '未启用监听'}}（MEMBER_ADDR；修改需重启）</dd></div><div><dt>地址来源</dt><dd>{{settings.addressSource}}</dd></div></dl>
  <div class="client-guides"><article><h3>Infuse 添加</h3><ol><li>打开「设置 → 添加文件 → 添加共享」，选择 Emby。</li><li>输入上方播放器服务地址；若分开填写地址与端口，使用该地址的协议、域名和外部端口。</li><li>输入管理员创建的媒体账号与最终密码，选择已授权的媒体库。</li></ol></article><article><h3>SenPlayer 添加</h3><ol><li>在媒体源 / 服务器管理中添加 Emby 类型服务器（名称随版本可能不同）。</li><li>填写同一播放器服务地址、媒体账号及密码。</li><li>刷新媒体库。无法连接时检查反向代理和播放器 UA 白名单。</li></ol></article></div>
  <p class="hint">不要填管理后台地址或外部 Emby 地址；无需全局 API Key。这里只做原始流 302 直连，不转码。</p>
  <h3>媒体库与目录分类</h3><p class="hint">库名来自目录分类策略；只列分类名称，不展示云盘路径或输出物理目录。隐藏库不向播放器展示，新库默认未授权。</p>
  <div class="mapping-table"><el-table :data="libraries.items" empty-text="尚无同步的分类库"><el-table-column prop="name" label="媒体库" min-width="150"/><el-table-column label="目录分类" min-width="180"><template #default="{row}">{{row.groupName || '兜底分类'}}{{row.ruleName ? ' / ' + row.ruleName : ''}}</template></el-table-column><el-table-column label="类型" width="90"><template #default="{row}">{{row.collectionType==='movies'?'电影':'剧集'}}</template></el-table-column><el-table-column prop="assetCount" label="可暴露资产" width="110"/><el-table-column label="状态" width="100"><template #default="{row}">{{row.hidden?'隐藏':'可授权'}}</template></el-table-column></el-table></div>
  <el-pagination v-model:current-page="page" :total="libraries.total" :page-size="50" layout="prev, pager, next, total" :disabled="loading || saving" @current-change="changePage"/>
 </section>
</template>
<style scoped>
.media-settings{margin-bottom:24px}.heading{display:flex;justify-content:space-between;gap:16px;align-items:center}.media-settings h2{margin:0}.media-settings p,.hint,.connection dt,.client-guides li{color:var(--muted);line-height:1.8}.media-settings .el-form{max-width:620px;margin:24px 0}.switch-label{margin-left:12px}.connection{border-block:1px solid var(--border);padding:12px 0}.connection>div{display:grid;grid-template-columns:150px 1fr;gap:16px;padding:10px 0}.connection dd{margin:0;overflow-wrap:anywhere}.client-guides{display:grid;grid-template-columns:1fr 1fr;gap:24px}.client-guides ol{padding-left:24px}.mapping-table{overflow:auto;margin:16px 0}.mapping-table .el-table{min-width:640px}.el-pagination{max-width:100%;overflow:auto}.media-settings .el-button{min-height:40px}@media(max-width:700px){.heading{align-items:flex-start;flex-direction:column}.client-guides{grid-template-columns:1fr;gap:8px}.connection>div{grid-template-columns:1fr;gap:4px}.media-settings .el-button{min-height:44px}}
</style>
