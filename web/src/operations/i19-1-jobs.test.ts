import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it('I19-1 leaves request, paging, SSE and task state logic byte-identical except the icon import',()=>{
 const script=read('operations/QueuePanel.vue').match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!.replace("import { View, MoreFilled } from '@element-plus/icons-vue';\n",'');
 expect(sha(script)).toBe('889a3b5f550e1ff4fbf3275dc0995c3f25e9e7c440616207065521097e0df80f');
});
it.each([
 ['workflows/TaskControl.vue','1754c6a35624517dccf545cb2e34ccc6daa8fa30d74f296e3c5ebe73663bad5b'],
 ['ui/ConfirmAction.vue','abb54b05b33a408a385aef0ad5d32b311be6d7b77ab7125c481cd10f8773278b'],
 ['ui/DetailDrawer.vue','89d028ea7bb8716a17edf4af45a9095b5bae5378d3bdf34dfee310d2224a2d76'],
 ['workflows/useOperation.ts','fdd5b04fc7e4c02d537ca4eb727f64bd7c1ffb096569dca4aa4ddbfd8a5f9747'],
])('I19-1 does not alter safety or global dialog behavior in %s',(file,hash)=>expect(sha(read(file))).toBe(hash));
it('I19-1 reuses lists/pagination, never truncates execution metadata and keeps the existing risk text',()=>{
 const source=read('operations/QueuePanel.vue');for(const token of ['mt-list-row f3-job-row','mt-list-columns ops-job-columns','mt-list-actions ops-job-actions','<PagePagination','重试先核对原业务状态','不能证明安全时保留未知状态','<TaskControl'])expect(source).toContain(token);
 expect(source).toContain('!jobs.loaded&&!jobs.error');expect(source).toContain('aria-label="任务详情"');expect(source).toContain('aria-label="更多"');
});
it('I19-1 confines every selector to operations, without rewriting global stability/minimum sizes',()=>{
 const source=read('operations/presentation.css');postcss.parse(source).walkRules(rule=>{for(const s of rule.selector.split(/,(?![^()]*\))/))expect(s.trim(),s).toMatch(/^\.t8-ops(?:\s|>|[.:]|$)|^\.t8-ops-menu(?:\s|$)/)});
 expect(source).toContain('.ops-job-list.ops-content-stage{min-height:0}');expect(source).toContain('.ops-job-meta-queue{display:inline}');
});
