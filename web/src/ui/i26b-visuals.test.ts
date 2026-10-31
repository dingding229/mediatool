// I41 approves grouped directory reads, segmented access UI and debounced filtering; mutating functions remain pinned separately.
// I40 ImageEditor/SubtitlePanel intentionally gain deferred saves and language recognition; behavioral tests live in media/i40-edit.test.ts and subtitles.test.ts.
// I38: approved global pagination/defaults, FilePicker adapters, display-only imports/layout and version projection. Payload/action guards remain.
// I36: refresh display-only snapshots after removing developer details; action functions are pinned in i36-business-functions.test.ts.
import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it.each([
 [
  "views/DashboardWorkspaceView.vue",
  "5861923cdb7ed844ed4d9fab14c542b0bce785d0010149e933f58a92d2326814"
 ],
 [
  "operations/LogsPanel.vue",
  "38fe7895e48a8bf4a6c3946e0322682fbf7f6d00824af3bf4dba2db800b9b1f4"
 ],
 [
  "connections/ProxySettings.vue",
  "1802e9f79698ab1e0c5f87cd98185d11bb771e03e7a3e47270c902bfe4a18d3f"
 ],
 [
  "playback/UserEditor.vue",
  "8ec0a81af9fbdd61c4ab19a76227fd7264a122897cb9534ae7b7a772e7cba81f"
 ],
 [
  "playback/IPBlockEditor.vue",
  "eaec83a7ddefc45f49265d4cea547acfe12e64f5dadb060ea0b12fe977d5b5bd"
 ],
 [
  "playback/ServicePanel.vue",
  "a6595fb65d5cec6cde4ac652aeceea6660970c015bf10a17737d440690127f04"
 ],
 [
  "playback/AccessPanel.vue",
  "510fb15e710fb56199ba541da798b66936468d261bc10b2ca7863d9f8a19002c"
 ],
 [
  "settings/AboutPanel.vue",
  "e1ba9e19fe8cd5230d7984df13e070e3d24236d706430df20b1e457d397582fc"
 ]
])('I26b leaves requests, payloads and interaction logic unchanged: %s',(file,hash)=>{
 const script=read(file).match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!.replace("import {label} from '../acquisition/model';\n",'');
 // I34 adds only the shared state-chip CSS import; the entire image operation script remains pinned.
 expect(sha(file==='media/ImageEditor.vue'?script.replace("import './editor.css';\n",''):script)).toBe(hash);
});
it('IP confirmation changes only its unique CSS class; original words, acknowledgement and actions remain exact',()=>{
 const source=read('playback/IPBlockEditor.vue');
 const template=source.slice(source.indexOf('<template>'),source.indexOf('<style')).replace('class="media-dialog ip-block-confirm"','class="media-dialog"');
 expect(sha(template)).toBe('ca2effcd28613ad4071d971f5e4c43304614137ed6e37355f3d57e8342b226a4');
});
it('I26b field tokens no longer style Element inner inputs as nested native frames',()=>{
 const css=read('workflows/u5.css');
 expect(css).toContain('input:not([type=checkbox]):not(.el-input__inner):not(.el-select__input)');
 expect(css).toContain('border-radius:var(--el-border-radius-base)');expect(css).toContain('padding:var(--mt-field-padding)');
 expect(read('style.css')).toContain('--mt-field-padding: 8px 12px');
 const presentation=read('operations/presentation.css');
 expect(presentation).not.toMatch(/\.el-input__inner\{[^}]*padding:/);
});
it('I26b friendly preference switches its original boolean, not the saved policy',()=>{
 const logs=read('operations/LogsPanel.vue');expect(logs).toContain('<ElSwitch v-model="friendly" aria-label="友好说明"/>');
 expect(logs).toContain("await policy.save(['logging','playbackRetentionDays'])");
 const service=read('playback/ServicePanel.vue');expect(service).not.toContain('PagePagination');expect(service).toContain('listAllNativeLibraryMappings');expect(service).toContain('empty-title="暂无媒体库目录"');
});

// I33 intentionally replaces the paged read and adds a retained-draft close guard.
// Keep the original save envelope, lock/restore patch and unknown-receipt blocking.
it('I33 metadata layout keeps the save API and revision/conflict protections',()=>{
 const source=read('media/MetadataEditor.vue');
 expect(source).toContain('saveMediaMetadata(activeID.value,patch.value)');
 expect(source).toContain('metadataPatch(record.value,draft,restore.value)');
 expect(source).toContain('e.status>=500||e.status===409');
 expect(source).toContain('busy||blocked||!dirty');
 expect(source).toContain('readMediaMetadata(activeID.value,1,unmatchedLimit)');
});
