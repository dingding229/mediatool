import {expect,it} from 'vitest';
import {readFileSync,readdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {parse} from '@vue/compiler-sfc';
const root=resolve(__dirname,'..');
function files(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?files(resolve(dir,e.name)):e.name.endsWith('.vue')?[resolve(dir,e.name)]:[])}
it('all page and runtime pagination uses the same component and vocabulary',()=>{for(const file of files(root)){const template=parse(readFileSync(file,'utf8')).descriptor.template?.content||'';expect(template,file).not.toContain('<el-pagination');if(!file.endsWith('/PagePagination.vue'))expect(template,file).not.toMatch(/>(上一页|下一页)[^<]*</)}});
it('main action slots never contain a text refresh action',()=>{for(const file of files(resolve(root,'views'))){const template=parse(readFileSync(file,'utf8')).descriptor.template?.content||'';const action=template.match(/<template #actions>([\s\S]*?)<\/template>/)?.[1]||'';expect(action,file).not.toMatch(/>\s*(刷新|重新读取|读取当前)[^<]*</)}});
it('settings forms compare user-facing fields without embedding raw technical blocks',()=>{for(const name of ['ConfigPanel.vue','RevisionFeedback.vue','ClassificationPanel.vue'])expect(parse(readFileSync(resolve(root,'settings',name),'utf8')).descriptor.template!.content).not.toContain('<summary>技术详情')});
it('empty error unsupported and unauthorized remain distinct states',()=>{const template=parse(readFileSync(resolve(root,'ui/StateView.vue'),'utf8')).descriptor.template!.content;for(const kind of ['empty','error','unsupported','unauthorized'])expect(template).toContain(`state === '${kind}'`)});
it('read-only refresh remains distinct from explicit downloader synchronization',()=>{const page=readFileSync(resolve(root,'views/DownloadsWorkspaceView.vue'),'utf8');expect(page).toContain('同步下载任务');expect(page).toContain('@click="refresh(true)"');expect(page).toContain('usePageRefresh(()=>refresh()');expect(readFileSync(resolve(root,'ui/AsyncResultBanner.vue'),'utf8')).toContain("emit('refresh')")});
