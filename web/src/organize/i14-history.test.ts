import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {readFileSync} from 'node:fs';
import {setupComponent,flush} from '../test/setup-component';
import History from './HistoryPanel.vue';
import {request} from '../api/client';
import {kindLabels} from '../files/workbench';
const {route,replace}=vi.hoisted(()=>({route:{query:{tab:'history'} as Record<string,string>},replace:vi.fn()}));
vi.mock('vue-router',()=>({useRoute:()=>route,useRouter:()=>({replace})}));
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn(),post:vi.fn()}));
let view:ReturnType<typeof setupComponent>;
beforeEach(()=>{route.query={tab:'history'};replace.mockReset();vi.mocked(request).mockReset().mockResolvedValue({items:[],total:0});view=setupComponent(History)});
afterEach(()=>view.unmount());
it('resets all filters and page, preserving page size and unrelated URL keys',async()=>{
 await flush();const s=view.state;s.page=3;s.pageSize=50;
 for(const key of ['kind','state','query','batchId','origin','method','from','to'])s[key]='previous';
 route.query={tab:'history',kind:'output',state:'pending',q:'search',batchId:'batch',keep:'owned',historyPageSize:'50'};
 s.resetFilters();await flush();expect(s.page).toBe(1);expect(s.pageSize).toBe(50);
 for(const key of ['kind','state','query','batchId','origin','method','from','to'])expect(s[key]).toBe('');
 const url=new URL('http://fixture'+vi.mocked(request).mock.calls.at(-1)![0]);expect(url.searchParams.get('pageSize')).toBe('50');expect(url.searchParams.get('page')).toBe('1');expect(url.searchParams.get('kind')).toBe('');
 expect(replace).toHaveBeenCalledWith({query:{tab:'history',keep:'owned',historyPageSize:'50',historyPage:'1'}});
});
it('uses only live stage options while old/unknown records have an explicit fallback',()=>{
 const s=view.state;expect(s.stageOptions).not.toHaveProperty('emby');expect(s.stageOptions).not.toHaveProperty('cleanup');expect(kindLabels).not.toHaveProperty('emby');
 expect(s.stageLabel('emby')).toBe('其他/历史阶段');expect(s.stageLabel('future-kind')).toBe('其他/历史阶段');expect(s.stageLabel('file')).toBe('文件整理');
});
it('uses shared list geometry, no timeline or stage navigation, and preserves recovery/bulk actions',()=>{
 const read=(p:string)=>readFileSync(new URL(p,import.meta.url),'utf8'),source=read('./HistoryPanel.vue'),css=read('./workspaces.css');
 expect(source).not.toContain('历史阶段快捷筛选');expect(source).toContain('mt-list-row organize-history-row');expect(source).toContain('mt-list-actions history-row-actions');
 expect(source.indexOf('移除所选历史')).toBeLessThan(source.indexOf('<ol class="mt-list'));expect(source).toContain('control.canCancel');expect(source).toContain('control.canRetry');
 expect(source).toContain('待恢复输出');expect(source).toContain('冲突输出');expect(source).toContain('@confirm="retryOutput"');expect(css).not.toContain('organize-history-timeline');
 expect(source).toContain("usePageNumber('historyPage')");expect(source).toContain("usePageSize(25,'historyPage')");
});
