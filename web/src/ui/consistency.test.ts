import {describe,it,expect,vi} from 'vitest';
import {reactive,defineComponent,h} from 'vue';
import {createRefreshScope,providePageRefresh,usePageRefresh} from './page-refresh';
import Pagination from './PagePagination.vue';
import {setupComponent,deferred,flush} from '../test/setup-component';
import {scheduleLabel} from '../presentation/labels';
const route=reactive({query:{} as Record<string,string>});
vi.mock('vue-router',()=>({useRoute:()=>route}));
describe('unified page refresh',()=>{
 it('only runs active readers, isolates failures, and prevents concurrent reads',async()=>{
  const scope=createRefreshScope(),wait=deferred(),active=vi.fn(()=>wait.promise),hidden=vi.fn(),failure=vi.fn(()=>{throw Error('offline')});
  scope.register(active,()=>true);scope.register(hidden,()=>false);scope.register(failure,()=>true);
  const pending=scope.refresh();await flush();await scope.refresh();expect(active).toHaveBeenCalledTimes(1);expect(hidden).not.toHaveBeenCalled();expect(scope.busy.value).toBe(true);
  wait.resolve(undefined);await pending;expect(scope.error.value).toContain('部分内容');expect(scope.busy.value).toBe(false);
 });
 it('removes disposed registrations and clears stale errors on another refresh',async()=>{const scope=createRefreshScope(),read=vi.fn();const remove=scope.register(read,()=>true);remove();await scope.refresh();expect(read).not.toHaveBeenCalled();expect(scope.active.value).toEqual([]);expect(scope.error.value).toBe('')});
 it('handles the default tab and unregisters readers when the page is left',async()=>{
  route.query={};const read=vi.fn(),scope=createRefreshScope('query');
  // Use real parent/child lifecycle: the parent provides before the child registers.
  let supplied:ReturnType<typeof providePageRefresh>;
  const child=defineComponent({setup(){usePageRefresh(read,'query');return()=>null}});
  const {createRenderer}=await import('vue');
  const renderer=createRenderer<object,object>({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},patchProp(){},parentNode:()=>null,nextSibling:()=>null});
  const app=renderer.createApp(defineComponent({setup(){supplied=providePageRefresh(scope.defaultTab);return()=>h(child)}}));app.mount({});
  await supplied!.refresh();expect(read).toHaveBeenCalledTimes(1);route.query.tab='history';await supplied!.refresh();expect(read).toHaveBeenCalledTimes(1);app.unmount();expect(supplied!.active.value).toHaveLength(0);
 });
});
describe('unified pagination',()=>{
 it('emits one update and one change, and prevents crossing list boundaries',()=>{const update=vi.fn(),change=vi.fn(),v=setupComponent(Pagination,{currentPage:1,pageSize:10,total:11,'onUpdate:currentPage':update,onCurrentChange:change});v.state.go(0);v.state.go(1);v.state.go(2);expect(update).toHaveBeenCalledExactlyOnceWith(2);expect(change).toHaveBeenCalledExactlyOnceWith(2);v.unmount()});
 it('unknown totals use hasNext, disabled controls never navigate',()=>{for(const props of [{currentPage:2,hasNext:false},{currentPage:2,hasNext:true,disabled:true}]){const change=vi.fn(),v=setupComponent(Pagination,{...props,onCurrentChange:change});v.state.go(3);expect(change).not.toHaveBeenCalled();v.unmount()}});
});
it.each([['0 */2 * * *','每 2 小时'],['*/15 * * * *','每 15 分钟'],['0 * * * *','每小时'],['30 2 * * *','每天 02:30'],['0 0 * * 1','按指定日期执行'],['','—']])('human schedule %s', (cron,label)=>expect(scheduleLabel(cron)).toBe(label));
it.each([['*/15 * * * * *','每 15 秒'],['* * * * *','每分钟']])('human scheduler interval %s',(cron,label)=>expect(scheduleLabel(cron)).toBe(label));
