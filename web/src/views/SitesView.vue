<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { post, request } from '../api/client'
import { isSourceCommit, snapshotRequest } from '../sites/snapshot'

interface Definition {
  id: string
  name: string
  schema: string
  sourceCommit: string
  compatibility: { state: string; notes: string[]; accountVerified: boolean }
}
const definitions = ref<Definition[]>([])
const error = ref(''), loading = ref(false), importing = ref(false), accepted = ref(false)
const ids = ref('hdsky,ourbits,mteam'), sourceCommit = ref('')
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
async function sync(importIds = selectedIds.value, commit = sourceCommit.value) {
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
onMounted(refresh)
</script>

<template>
  <section class="page-heading">
    <div><h1>PT 站点配置</h1><p>兼容上游配置并记录快照。解析支持不等于账号实测通过。</p></div>
    <el-button :loading="loading" @click="refresh">刷新</el-button>
  </section>
  <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
  <section class="panel section-gap">
    <div class="panel-title"><h2>导入 pre-dessert-sites</h2><span class="muted">配置保存在数据库，不打包进镜像</span></div>
    <div class="connection-form">
      <label for="site-ids">站点 ID（逗号分隔）</label>
      <el-input id="site-ids" v-model="ids" placeholder="hdsky,ourbits,mteam" />
      <label for="site-source-commit">来源提交（可选）</label>
      <el-input id="site-source-commit" v-model="sourceCommit" placeholder="完整的 40 位提交 SHA" aria-describedby="site-source-hint" />
      <p id="site-source-hint" class="field-hint">留空时解析上游当前提交后固定快照；填写原提交可恢复同一版本，不会悄悄升级。整批验证成功后才写入数据库。</p>
      <p v-if="formError" role="alert" class="snapshot-error">{{ formError }}</p>
      <p class="field-hint">首批支持 NexusPHP 和 M-Team 核心规则。浏览器挑战、特殊框架及未验证的时间偏移会明确报告限制。</p>
      <el-checkbox v-model="accepted">我已阅读上游许可证，确认具备本次配置使用授权</el-checkbox>
      <div class="heading-actions snapshot-actions">
        <el-button type="primary" :loading="importing" :disabled="!accepted || !!formError" @click="sync()">同步固定快照</el-button>
        <RouterLink to="/integrations" class="inline-link">配置站点账号</RouterLink>
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
      <el-table-column label="解析兼容" min-width="200">
        <template #default="scope">
          <span>{{ scope.row.compatibility.state === 'parser_supported' ? '核心规则支持，待实测' : '部分支持' }}</span>
          <p class="field-hint">{{ scope.row.compatibility.notes?.join('；') }}</p>
        </template>
      </el-table-column>
      <el-table-column label="来源快照" min-width="150">
        <template #default="scope"><code :title="scope.row.sourceCommit">{{ scope.row.sourceCommit.slice(0, 12) }}</code></template>
      </el-table-column>
      <el-table-column label="快照恢复" min-width="190">
        <template #default="scope">
          <el-button class="snapshot-reimport" :disabled="!accepted || importing || !isSourceCommit(scope.row.sourceCommit)" @click="sync([scope.row.id], scope.row.sourceCommit)">按此快照重新导入</el-button>
          <p v-if="!isSourceCommit(scope.row.sourceCommit)" class="field-hint">此记录没有可复现的上游提交。</p>
        </template>
      </el-table-column>
    </el-table>
    <div class="snapshot-mobile">
      <p v-if="!definitions.length" class="muted">尚未导入站点配置</p>
      <article v-for="definition in definitions" :key="definition.id" class="snapshot-record">
        <div><strong>{{ definition.name }}</strong><small class="table-id">{{ definition.id }} · {{ definition.schema }}</small></div>
        <p>{{ definition.compatibility.state === 'parser_supported' ? '核心规则支持，待实测' : '部分支持' }}</p>
        <p v-if="definition.compatibility.notes?.length" class="field-hint">{{ definition.compatibility.notes.join('；') }}</p>
        <div><span class="muted">来源快照</span><code class="snapshot-commit">{{ definition.sourceCommit }}</code></div>
        <el-button class="snapshot-reimport" :disabled="!accepted || importing || !isSourceCommit(definition.sourceCommit)" @click="sync([definition.id], definition.sourceCommit)">按此快照重新导入</el-button>
        <p v-if="!isSourceCommit(definition.sourceCommit)" class="field-hint">此记录没有可复现的上游提交。</p>
      </article>
    </div>
  </section>
</template>

<style scoped>
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
