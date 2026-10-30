import {expect,it} from 'vitest';
import {readdirSync,readFileSync} from 'node:fs';
import {resolve} from 'node:path';
function files(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?files(resolve(dir,e.name)):[resolve(dir,e.name)])}
it('every pagination instance consumes page size updates (including local-only slices)',()=>{let count=0;for(const path of files(resolve(import.meta.dirname,'..')).filter(p=>p.endsWith('.vue'))){for(const match of readFileSync(path,'utf8').matchAll(/<PagePagination\b[\s\S]*?\/>/g)){count++;expect(match[0],path).toMatch(/v-model:page-size=|@update:page-size=/)}}expect(count).toBe(42);expect(readFileSync(resolve(import.meta.dirname,'../media/MetadataEditor.vue'),'utf8')).not.toContain('PagePagination');expect(readFileSync(resolve(import.meta.dirname,'../playback/UserActivityPanel.vue'),'utf8')).toContain("usePageNumber('userActivityPage')")});

it('workflow dialog does not share the installed instances pagination key',()=>{expect(readFileSync(resolve(import.meta.dirname,'../plugins/PluginAutomation.vue'),'utf8')).toContain("usePageNumber('workflowsPage')");});
it('workflow pagers own distinct regions; an unselected empty run history has no pager',()=>{const source=readFileSync(resolve(import.meta.dirname,'../plugins/PluginAutomation.vue'),'utf8');expect(source).toContain('<section aria-label="工作流列表">');expect(source).toContain('<section v-if="selected" aria-label="工作流运行记录">');for(const label of ['工作流列表分页','工作流运行记录分页'])expect(source.match(new RegExp('label="'+label+'"','g'))).toHaveLength(1)});
