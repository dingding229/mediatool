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
  "d2578d872a94b921423b13270444bd8909e0fca91e6b48caf6810976035b928e"
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
  "742fd61125ec61d3e39fdce9411d947940b7cb5a3c280c61ec69895594340adf"
 ],
 [
  "playback/AccessPanel.vue",
  "939065ab747d4183fadd1356116d74d49a15db484ea672f5fc627bc452216739"
 ],
 [
  "media/ImageEditor.vue",
  "83634b7bb16ab870dd83b9660dfd5cf505b059f96e690ddd138f27b718710157"
 ],
 [
  "media/MetadataEditor.vue",
  "d812d3cac85ac4c532086082643d47097397abf86f4ee90a032805fb9cdc7e87"
 ],
 [
  "media/SubtitlePanel.vue",
  "c99245782cf904738ae19d411560293c5481bc601b9d6c9a6655c2932c855132"
 ],
 [
  "settings/AboutPanel.vue",
  "39915ebda1d0d22826f6e0fbfec3af5aea9499bb38c52d9c944f0177ae147728"
 ]
])('I26b leaves requests, payloads and interaction logic unchanged: %s',(file,hash)=>{
 expect(sha(read(file).match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!)).toBe(hash);
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
 const service=read('playback/ServicePanel.vue');expect(service).toContain(':total="libraries.data?.total??0"');expect(service).toContain('empty-title="暂无媒体库目录"');
});
