import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {createSSRApp} from 'vue';
import {renderToString} from 'vue/server-renderer';
import Sessions from './SessionsPanel.vue';
import PagePagination from '../ui/PagePagination.vue';
import {listNativeSessions} from '../api/mediaServer';
import {setupComponent,deferred,flush} from '../test/setup-component';
vi.mock('vue-router',()=>({useRoute:()=>({query:{tab:'sessions'}})}));
vi.mock('../api/mediaServer',()=>({listNativeSessions:vi.fn()}));
let mounted:ReturnType<typeof setupComponent>|undefined;
beforeEach(()=>{vi.mocked(listNativeSessions).mockReset();});
afterEach(()=>{mounted?.unmount();mounted=undefined;});
it('T16B renders the real session template before any request completes',async()=>{
 const app=createSSRApp(Sessions);app.component('PagePagination',PagePagination);app.config.warnHandler=()=>{};
 const html=await renderToString(app);
 expect(html).toContain('在线会话');expect(html).toContain('第 1 页 / 共 1 页');
 expect(listNativeSessions).not.toHaveBeenCalled();
});
it('T16B starts with an empty page while loading and preserves it on failure',async()=>{
 const pending=deferred<Awaited<ReturnType<typeof listNativeSessions>>>();
 vi.mocked(listNativeSessions).mockReturnValueOnce(pending.promise);
 mounted=setupComponent(Sessions);
 expect(mounted.state.data.data).toEqual({items:[],total:0,startIndex:0,limit:50});
 expect(mounted.state.data.loading).toBe(true);expect(mounted.state.open).toBe(false);
 pending.reject(Error('读取失败'));await flush();
 expect(mounted.state.data.error).toBeTruthy();expect(mounted.state.data.data.items).toEqual([]);expect(mounted.state.data.data.total).toBe(0);
 expect(mounted.state.open).toBe(false);
});
