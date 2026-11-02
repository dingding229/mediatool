<script setup lang="ts">
import ExpandableSection from "../ui/ExpandableSection.vue";
import {joinMetadata} from '../presentation/metadata';

import { computed, ref } from 'vue'
import type { PluginMetadata } from '../api/plugins'
import inventory from '../../../internal/modules/plugins/v2/library.json'
const props=defineProps<{metadata:PluginMetadata}>()
const failed=ref(false)
const icon=computed(()=>{const m=props.metadata,r=inventory.resources.find(r=>r.id===m.id&&r.path===m.icon);return r&&inventory.resources.filter(x=>x.path.endsWith('.svg')&&x.sha256===r.sha256).length===1?`/plugins/${encodeURIComponent(m.id)}/${r.path.split('/').map(encodeURIComponent).join('/')}`:''})
const history=computed(()=>Object.entries(props.metadata.history??{}).sort(([a],[b])=>b.localeCompare(a,undefined,{numeric:true})))
</script>
<template>
 <div class="metadata-heading"><img v-if="icon&&!failed" :key="icon" :src="icon" alt="" width="40" height="40" loading="lazy" @error="failed=true"><span v-else class="icon-fallback" aria-hidden="true">{{Array.from(metadata.name||metadata.id)[0]}}</span><div><h3>{{metadata.name}}</h3><span class="plugin-meta-line" :title="`版本 ${metadata.version} · 作者 ${metadata.author||'未提供'}`">{{joinMetadata([joinMetadata(["版本 ",(metadata.version)], ''),joinMetadata(["作者 ",(metadata.author||'未提供')], '')])}}</span></div></div>
 <p>{{metadata.description}}</p>
 <ExpandableSection title="版本记录"><dl v-if="history.length"><template v-for="[version,note] in history" :key="version"><dt>{{version}}</dt><dd>{{note}}</dd></template></dl><p v-else>暂无版本记录</p></ExpandableSection>
</template>
<style scoped>
.metadata-heading{display:flex;align-items:center;gap:12px}.plugin-meta-line{display:block;font-size:12px;font-weight:400;color:var(--mt-text-muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.metadata-heading>div{min-width:0;flex:1}.metadata-heading h3{margin:0 0 6px}.metadata-heading img,.icon-fallback{flex:none;border-radius:8px}.icon-fallback{display:grid;place-items:center;width:40px;height:40px;background:var(--el-fill-color);color:var(--el-text-color-secondary)}summary{cursor:pointer;min-height:44px;align-content:center}p,dd{line-height:1.6}dt{font-weight:600}dd{margin:6px 0 16px}code,dd,span{overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:240px;overflow:auto}li{margin-block:10px}ul{padding-left:20px}
</style>
