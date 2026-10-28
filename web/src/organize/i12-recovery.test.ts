import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {readFileSync,existsSync} from 'node:fs';
import {setupComponent,flush} from '../test/setup-component';
import History from './HistoryPanel.vue';
import {post,request} from '../api/client';
vi.mock('vue-router',()=>({useRoute:()=>({query:{tab:'history'}})}));
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),post:vi.fn(),request:vi.fn()}));
let view:ReturnType<typeof setupComponent>;
const row={id:'12121212-0000-4000-8000-000000000004',kind:'output',state:'pending',source:'Movie.strm',details:{operation:'write'}};
beforeEach(()=>{vi.mocked(request).mockReset().mockResolvedValue({items:[row],total:1});vi.mocked(post).mockReset().mockResolvedValue({repaired:true});view=setupComponent(History)});
afterEach(()=>view?.unmount());
it('finds pending and conflict outputs through the existing history filter',async()=>{
 await flush();const s=view.state;s.page=3;s.query='previous search';s.filterOutputs('pending');await flush();expect(s.kind).toBe('output');expect(s.state).toBe('pending');expect(s.page).toBe(1);expect(s.query).toBe('');expect(request).toHaveBeenLastCalledWith(expect.stringContaining('kind=output&state=pending'));
 s.filterOutputs('conflict');await flush();expect(request).toHaveBeenLastCalledWith(expect.stringContaining('kind=output&state=conflict'));
});
it('only retries the original pending/conflict journal and reads back history',async()=>{
 await flush();const s=view.state;s.detail=row;s.open=true;s.retryOpen=true;await s.retryOutput();expect(post).toHaveBeenCalledExactlyOnceWith(`/library/artifact-repairs/${row.id}/retry`,{});expect(s.open).toBe(false);expect(s.retryPending).toBe('');expect(request).toHaveBeenCalledWith(expect.stringContaining('/library/activity?'));
 s.detail={...row,state:'applied'};await s.retryOutput();expect(post).toHaveBeenCalledTimes(1);
});
it('blocks a repeated uncertain retry until successful history readback',async()=>{
 await flush();const s=view.state;s.detail=row;s.open=true;vi.mocked(post).mockRejectedValueOnce(Error('回执丢失'));await s.retryOutput();expect(s.retryPending).toBe(row.id);expect(s.open).toBe(true);await s.retryOutput();expect(post).toHaveBeenCalledTimes(1);await s.load();expect(s.retryPending).toBe('');
});
it('removes the independent tab and archive UI while downloads reuse the existing recovery dialog',()=>{
 const read=(p:string)=>readFileSync(new URL(p,import.meta.url),'utf8');expect(existsSync(new URL('./ArchivePanel.vue',import.meta.url))).toBe(false);
 expect(read('../views/OrganizeWorkspaceView.vue')).not.toMatch(/tab===\x27recovery|ArchivePanel|artifact-repairs/);
 const downloads=read('../views/DownloadsWorkspaceView.vue');expect(downloads).toContain('output?.open(transfer.id)');expect(downloads).toContain('<OutputRecoveryDialog ref="output"');expect(downloads).not.toContain("tab:'recovery'");
 const history=read('./HistoryPanel.vue');expect(history).toContain('待恢复输出');expect(history).toContain('冲突输出');expect(history).toContain('重试恢复');
});
