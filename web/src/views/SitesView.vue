<script setup lang="ts">
import SiteMetrics from '../components/SiteMetrics.vue'
import ConnectionPanel from '../components/ConnectionPanel.vue'
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { post, request } from '../api/client'
import { isSourceCommit, snapshotRequest } from '../sites/snapshot'

interface Definition {
  id: string
  name: string
  schema: string
  sourceRepository: string
  sourceCommit: string
  compatibility: { state: string; notes: string[]; accountVerified: boolean }
}
const definitions = ref<Definition[]>([])
const error = ref(''), loading = ref(false), importing = ref(false), accepted = ref(false)
interface Catalog {repository:string;sourceCommit:string;templates:{id:string;name:string;templateVersion:number;sha256:string}[]}
const catalog=ref<Catalog|null>(null),catalogLoading=ref(false),catalogError=ref('')
const ids = ref('hdsky,ptchdbits,ssd,ourbits,ubits,ttg'), sourceCommit = ref('')
function selectTemplates(value:(string|number|boolean)[]){ids.value=value.map(String).join(',')}
async function loadCatalog(){catalogLoading.value=true;catalogError.value='';try{catalog.value=await request<Catalog>('/sites/catalog'+(sourceCommit.value.trim()?'?sourceCommit='+encodeURIComponent(sourceCommit.value.trim()):''));ids.value=catalog.value.templates.map(x=>x.id).join(',')}catch(e){catalogError.value=e instanceof Error?e.message:'目录获取失败'}finally{catalogLoading.value=false}}
const selectedIds = computed(() => ids.value.split(',').map(value => value.trim()).filter(Boolean))
const formError = computed(() => {
  try { snapshotRequest(selectedIds.value, sourceCommit.value); return '' }
  catch (failure) { return failure instanceof Error ? failure.message : '请检查站点 ID 和提交引用。' }
})

async function refresh() {
  loading.value = true
  error.value = ''
  try { definitions.value = await request<Definition[]>('/sites/definitions') }
  catch (failure) { error.value = failure instanceof Error ? failure.message : '站点配置读取失败' }
  finally { loading.value = false }
}
async function sync(importIds = selectedIds.value, commit = sourceCommit.value.trim() || catalog.value?.sourceCommit || '') {
  if (!accepted.value || importing.value) return
  importing.value = true
  error.value = ''
  try {
    const payload = snapshotRequest(importIds, commit)
    await post('/sites/definitions/import', { licenseAccepted: true, ...payload })
    ElMessage.success('已原子导入固定快照，账号实测仍需单独验证')
    await refresh()
  } catch (failure) {
    const message = failure instanceof Error ? failure.message : '导入失败'
    // A lost commit response may still have saved the batch. Read back first;
    // never retry automatically or silently choose a newer commit.
    await refresh()
    error.value = message
  }
  finally { importing.value = false }
}
onMounted(()=>{refresh();loadCatalog()})
</script>

<template>
  <section class="page-heading">
    <div><span class="page-kicker">ACQUIRE / PT 站点</span><h1>PT 站点配置</h1><p>只使用内置 mediatool-site 原生模板仓库。旧模板不再支持，解析支持不等于账号实测通过。</p></div>
    <el-button :loading="loading" @click="refresh">刷新</el-button>
  </section>
  <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
  <SiteMetrics/>
  <section class="panel section-gap">
    <div class="panel-title"><h2>内置站点模板仓库</h2><span class="muted">原生格式 v1 · 固定提交与摘要校验</span></div>
    <p class="field-hint">唯一来源：<a href="https://github.com/dingding229/mediatool-site" target="_blank" rel="noopener noreferrer" class="inline-link">dingding229/mediatool-site</a>。无需填写仓库地址；不回退到旧配置库。</p>
<el-alert v-if="catalogError" :title="catalogError" type="warning" :closable="false"/>
<div class="heading-actions"><el-button :loading="catalogLoading" @click="loadCatalog">读取内置仓库目录</el-button><span v-if="catalog" class="muted">{{catalog.templates.length}} 个模板 · 目录提交 {{catalog.sourceCommit.slice(0,12)}}</span></div>
<el-checkbox-group v-if="catalog" :model-value="selectedIds" @update:model-value="selectTemplates" class="template-selection" aria-label="选择原生站点模板"><el-checkbox v-for="entry in catalog.templates" :key="entry.id" :value="entry.id">{{entry.name}} · {{entry.id}}</el-checkbox></el-checkbox-group>
<div class="connection-form">
      <label for="site-ids">站点 ID（逗号分隔）</label>
      <el-input id="site-ids" v-model="ids" placeholder="hdsky,ptchdbits,ssd,ourbits,ubits,ttg" />
      <label for="site-source-commit">来源提交（可选）</label>
      <el-input id="site-source-commit" v-model="sourceCommit" placeholder="完整的 40 位提交 SHA" aria-describedby="site-source-hint" />
      <p id="site-source-hint" class="field-hint">留空时使用已读取目录的固定提交；尚未读取目录时才解析内置仓库当前提交。填写原提交可恢复同一版本，不会悄悄升级。整批验证成功后才写入数据库。</p>
      <p v-if="formError" role="alert" class="snapshot-error">{{ formError }}</p>
      <p class="field-hint">仅接受 templateVersion=1 的独立模板与 catalogVersion=1 目录，校验 SHA256。旧模板、公共配置继承和原仓库提交不受支持。</p>
      <el-checkbox v-model="accepted">我已阅读模板仓库许可，确认具备本次使用授权</el-checkbox>
      <div class="heading-actions snapshot-actions">
        <el-button type="primary" :loading="importing" :disabled="!accepted || !!formError" @click="sync()">同步固定快照</el-button>
        <a href="#pt-connections" class="inline-link">配置站点账号</a>
      </div>
    </div>
  </section>
  <section class="panel">
    <div class="panel-title"><h2>已导入配置</h2><span class="muted">{{ definitions.length }} 个站点</span></div>
    <el-table class="snapshot-desktop" :data="definitions" empty-text="尚未导入站点配置">
      <el-table-column label="站点" min-width="160">
        <template #default="scope"><strong>{{ scope.row.name }}</strong><small class="table-id">{{ scope.row.id }}</small></template>
      </el-table-column>
      <el-table-column prop="schema" label="框架" min-width="120" />
      <el-table-column label="原生模板状态" min-width="200">
        <template #default="scope">
          <span>{{ scope.row.compatibility.state === 'parser_supported' ? '核心规则支持，待实测' : scope.row.compatibility.state==='unsupported'?'旧模板不支持':'部分支持' }}</span>
          <p class="field-hint">{{ scope.row.compatibility.notes?.join('；') }}</p>
        </template>
      </el-table-column>
      <el-table-column label="来源快照" min-width="150">
        <template #default="scope"><code :title="scope.row.sourceCommit">{{ scope.row.sourceCommit.slice(0, 12) }}</code></template>
      </el-table-column>
      <el-table-column label="快照恢复" min-width="190">
        <template #default="scope">
          <el-button class="snapshot-reimport" :disabled="!accepted || importing || !isSourceCommit(scope.row.sourceCommit) || scope.row.sourceRepository!=='dingding229/mediatool-site'" @click="sync([scope.row.id], scope.row.sourceCommit)">按此快照重新导入</el-button>
          <p v-if="!isSourceCommit(scope.row.sourceCommit) || scope.row.sourceRepository!=='dingding229/mediatool-site'" class="field-hint">此记录不是原生仓库快照，请从内置目录重新导入。</p>
        </template>
      </el-table-column>
    </el-table>
    <div class="snapshot-mobile">
      <p v-if="!definitions.length" class="muted">尚未导入站点配置</p>
      <article v-for="definition in definitions" :key="definition.id" class="snapshot-record">
        <div><strong>{{ definition.name }}</strong><small class="table-id">{{ definition.id }} · {{ definition.schema }}</small></div>
        <p>{{ definition.compatibility.state === 'parser_supported' ? '核心规则支持，待实测' : definition.compatibility.state==='unsupported'?'旧模板不支持':'部分支持' }}</p>
        <p v-if="definition.compatibility.notes?.length" class="field-hint">{{ definition.compatibility.notes.join('；') }}</p>
        <div><span class="muted">来源快照</span><code class="snapshot-commit">{{ definition.sourceCommit }}</code></div>
        <el-button class="snapshot-reimport" :disabled="!accepted || importing || !isSourceCommit(definition.sourceCommit) || definition.sourceRepository!=='dingding229/mediatool-site'" @click="sync([definition.id], definition.sourceCommit)">按此快照重新导入</el-button>
        <p v-if="!isSourceCommit(definition.sourceCommit) || definition.sourceRepository!=='dingding229/mediatool-site'" class="field-hint">此记录不是原生仓库快照，请从内置目录重新导入。</p>
      </article>
    </div>
  </section>
<div id="pt-connections" class="anchor-section"><ConnectionPanel :allowed-kinds="['ptsite']" title="PT 站点账号"/></div></template>

<style scoped>
.template-selection{display:flex;flex-wrap:wrap;gap:8px 16px;margin-block:16px}.template-selection :deep(.el-checkbox){min-height:44px;margin-right:0}
.snapshot-error { color: var(--el-color-danger); font-size: 13px; line-height: 1.6; }
.snapshot-actions :deep(.el-button), .snapshot-reimport { min-height: 44px; }
.connection-form :deep(.el-checkbox) { height: auto; min-height: 44px; white-space: normal; align-items: flex-start; padding-block: 10px; }
.connection-form :deep(.el-checkbox__label) { white-space: normal; line-height: 1.6; }
.connection-form :deep(.el-checkbox__input) { margin-top: 4px; }
.snapshot-mobile { display: none; }
.snapshot-record { display: grid; gap: 12px; padding-block: 16px; }
.snapshot-record + .snapshot-record { border-top: 1px solid var(--el-border-color-lighter); }
.snapshot-record p { font-size: 13px; line-height: 1.6; }
.snapshot-commit { display: block; margin-top: 6px; }
@media (max-width: 560px) { .snapshot-desktop { display: none; } .snapshot-mobile { display: block; } }
</style>
