<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PluginMetadata } from '../api/plugins'
import { dangerousScope } from '../api/plugins'
import inventory from '../../../internal/modules/plugins/v2/library.json'
const props=defineProps<{metadata:PluginMetadata}>()
const failed=ref(false)
const icon=computed(()=>{const m=props.metadata;return m.icon&&inventory.resources.some(r=>r.id===m.id&&r.path===m.icon)?`/plugins/${encodeURIComponent(m.id)}/${m.icon.split('/').map(encodeURIComponent).join('/')}`:''})
const history=computed(()=>Object.entries(props.metadata.history??{}).sort(([a],[b])=>b.localeCompare(a,undefined,{numeric:true})))
</script>
<template>
 <div class="metadata-heading"><img v-if="icon&&!failed" :key="icon" :src="icon" alt="" width="40" height="40" loading="lazy" @error="failed=true"><span v-else class="icon-fallback" aria-hidden="true">P</span><div><h3>{{metadata.name}}</h3><span>版本 {{metadata.version}} · 作者 {{metadata.author||'未提供'}} · 等级 {{metadata.level??'未提供'}}</span></div></div>
 <p>{{metadata.description}}</p>
 <details><summary>版本记录与权限（{{metadata.permissions.length}}）</summary>
  <h4>版本记录</h4><dl v-if="history.length"><template v-for="[version,note] in history" :key="version"><dt>{{version}}</dt><dd>{{note}}</dd></template></dl><p v-else>暂无版本记录</p>
  <h4>声明权限</h4><ul><li v-for="p in metadata.permissions" :key="p.scope"><el-tag :type="dangerousScope(p.scope)?'danger':'info'">{{dangerousScope(p.scope)?'危险':'权限'}}</el-tag> <code>{{p.scope}}</code><pre v-if="p.resources">{{JSON.stringify(p.resources,null,2)}}</pre></li></ul>
 </details>
</template>
<style scoped>
.metadata-heading{display:flex;align-items:center;gap:12px}.metadata-heading h3{margin:0 0 6px}.metadata-heading img,.icon-fallback{flex:none;border-radius:8px}.icon-fallback{display:grid;place-items:center;width:40px;height:40px;background:var(--el-fill-color);color:var(--el-text-color-secondary)}summary{cursor:pointer;min-height:44px;align-content:center}p,dd{line-height:1.6}dt{font-weight:600}dd{margin:6px 0 16px}code,dd,span{overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:240px;overflow:auto}li{margin-block:10px}ul{padding-left:20px}
</style>
