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

it.each([{total:0,label:'第 1 页 / 共 1 页'},{total:1,label:'第 1 页 / 共 1 页'},{total:26,label:'第 1 页 / 共 2 页'},{total:undefined,label:'第 1 页'}])('pagination copy for $total',({total,label})=>{const view=setupComponent(Pagination,{total});expect(view.state.pageLabel).toBe(label);view.unmount()});

it('resize returns to first page exactly once after both model updates',async()=>{
 const calls:string[]=[],size=vi.fn(),change=vi.fn(),v=setupComponent(Pagination,{currentPage:3,pageSize:25,total:100,'onUpdate:pageSize':()=>calls.push('size'),'onUpdate:currentPage':()=>calls.push('page'),onSizeChange:size,onCurrentChange:change});
 expect(v.state.sizeOptions).toEqual([10,25,50,100]);v.state.resize(50);expect(calls).toEqual(['size','page']);expect(size).toHaveBeenCalledExactlyOnceWith(50);expect(change).toHaveBeenCalledExactlyOnceWith(1);v.unmount();
});
it('disabled and unchanged sizes do not trigger reads and custom options win',()=>{
 const change=vi.fn();for(const props of [{disabled:true},{pageSizes:[25,50,200]}]){const v=setupComponent(Pagination,{pageSize:25,...props,onSizeChange:change});v.state.resize(25);v.state.resize(10);if(props.disabled)v.state.resize(50);else expect(v.state.sizeOptions).toEqual([25,50,200]);v.unmount()}expect(change).not.toHaveBeenCalled();
});

it('pagination count copy has no whitespace before the comma',async()=>{const {createSSRApp,h}=await import('vue');const {renderToString}=await import('vue/server-renderer');const app=createSSRApp(Pagination,{total:1});app.component('el-select',{setup:(_:unknown,ctx:any)=>()=>h('select',ctx.slots.default?.())});app.component('el-option',{setup:()=>()=>h('option')});const html=await renderToString(app);expect(html).toContain('条，共 1 条');expect(html).not.toContain('条 ，共 1 条')});

it('form checkbox alignment keeps native control size and label hit area',async()=>{
 const {readFileSync}=await import('node:fs');
 const css=readFileSync(new URL('./consistency.css',import.meta.url),'utf8');
 expect(css).toContain('.mt-form-checkbox.mt-form-checkbox{display:flex;align-items:flex-start');
 expect(css).toContain('.mt-form-checkbox.mt-form-checkbox .el-checkbox__input{flex:0 0 20px');
 expect(css).toContain('margin:0 0 8px var(--mt-checkbox-text-offset,28px)');
 expect(css).not.toMatch(/\.mt-form-checkbox[^\{]*input[^\{]*width:\s*44px/);
 const proxy=readFileSync(new URL('../connections/ProxySettings.vue',import.meta.url),'utf8');
 expect(proxy.match(/mt-form-checkbox/g)?.length).toBeGreaterThanOrEqual(4);
 const connection=readFileSync(new URL('../connections/ConnectionManager.vue',import.meta.url),'utf8');
 expect(connection).toContain('class="mt-form-checkbox"');
});

it('single-value native time fields use the shared cross-workspace contract without touching Cron or range pickers',async()=>{
 const {readFileSync}=await import('node:fs');
 const read=(path:string)=>readFileSync(new URL(path,import.meta.url),'utf8');
 const css=read('./consistency.css');
 expect(css).toContain(':root input.mt-native-time-input:is([type=time],[type=datetime-local]),:root input.mt-native-form-input{');
 expect(css).toContain('height:44px;min-height:44px;box-sizing:border-box;padding:var(--mt-field-padding)');
 expect(css).toContain(':root.dark input.mt-native-time-input,:root.dark input.mt-native-form-input{color-scheme:dark}');
 expect(read('../settings/IdleUpdatePanel.vue').match(/class="mt-native-form-input/g)).toHaveLength(1);
 expect(read('../settings/AboutPanel.vue')).toContain('class="mt-native-form-input" aria-label="检查间隔（小时）"');
 for(const [file,count,type] of [['../settings/IdleUpdatePanel.vue',2,'time'],['../organize/HistoryPanel.vue',2,'datetime-local'],['../playback/UserEditor.vue',1,'datetime-local']] as const){
  expect(read(file).match(new RegExp('class="mt-native-time-input" type="'+type+'"','g'))).toHaveLength(count);
 }
 for(const file of ['../media/media.css','../organize/workspaces.css'])expect(read(file)).not.toContain('input[type=datetime-local]');
 expect(read('./CronInput.vue')).not.toContain('mt-native-time-input');
 expect(read('../operations/LogsPanel.vue')).toContain('type="datetimerange"');
});
