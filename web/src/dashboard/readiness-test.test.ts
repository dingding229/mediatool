import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {defineComponent,reactive} from 'vue';
import {post,responseStatus} from '../api/client';
import {setupComponent,flush} from '../test/setup-component';
import {useReadinessTest,type ReadinessTestKind} from './useReadinessTest';
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),post:vi.fn(),responseStatus:vi.fn()}));
let rows:any[],reload:()=>Promise<unknown>,store:Map<string,string>,view:ReturnType<typeof setupComponent>;
function mount(kind?:ReadinessTestKind){rows=reactive(rows);return setupComponent(defineComponent({setup:()=>useReadinessTest(()=>rows,reload,kind)}))}
beforeEach(()=>{store=new Map();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>store.get(k)||null,setItem:(k:string,v:string)=>store.set(k,v),removeItem:(k:string)=>store.delete(k)});rows=[{id:'own',kind:'open115',enabled:true,revision:2,lastCheckedAt:'before'}];reload=vi.fn(async()=>{});vi.mocked(post).mockReset();vi.mocked(responseStatus).mockReturnValue(202);view=mount()});afterEach(()=>{view.unmount();vi.unstubAllGlobals()});
it('202 with even a passed body stays queued and cannot be posted twice',async()=>{vi.mocked(post).mockResolvedValue({passed:true});await view.state.start();expect(view.state.phase).toBe('queued');expect(view.state.message).toContain('尚未完成');await view.state.start();expect(post).toHaveBeenCalledExactlyOnceWith('/integrations/own/test',{});expect(reload).not.toHaveBeenCalled();expect([...store.values()].join()).not.toMatch(/token|secret/) });
it('unknown result survives remount and readback never repeats POST',async()=>{vi.mocked(post).mockRejectedValue(Error('network'));await view.state.start();view.unmount();view=mount();await view.state.start();await view.state.reconcile();expect(post).toHaveBeenCalledTimes(1);expect(view.state.phase).toBe('reconciling');rows[0].lastCheckedAt='after';await view.state.reconcile();expect(view.state.phase).toBeNull()});
it('a changed revision or stale check is not treated as proof of this test finishing',async()=>{vi.mocked(post).mockResolvedValue({id:'job'});await view.state.start();rows[0].revision=3;rows[0].lastCheckedAt='after';await view.state.reconcile();expect(view.state.phase).toBe('queued')});
it('ambiguous enabled accounts never pick an arbitrary connection',async()=>{rows.push({...rows[0],id:'other'});expect(view.state.available).toBe(false);await view.state.start();expect(post).not.toHaveBeenCalled()});
it('a synchronous confirmed result reloads readiness without implying player playback',async()=>{vi.mocked(responseStatus).mockReturnValue(200);vi.mocked(post).mockResolvedValue({passed:true});await view.state.start();await flush();expect(view.state.phase).toBeNull();expect(reload).toHaveBeenCalledTimes(1);expect(view.state.message).toContain('不代表播放器')});

for(const kind of ['open115','clouddrive2','qbittorrent'] as const){
 it(`${kind}: busy is locked, confirmed failure reloads, disabled does not test`,async()=>{
  view.unmount();rows=[{...rows[0],id:kind,kind}];view=mount(kind);
  let finish!:(value:unknown)=>void;
  vi.mocked(responseStatus).mockReturnValue(200);
  vi.mocked(post).mockImplementation(()=>new Promise(resolve=>{finish=resolve}));
  const running=view.state.start();expect(view.state.busy).toBe(true);
  await view.state.start();expect(post).toHaveBeenCalledTimes(1);
  finish({passed:false});await running;
  expect(post).toHaveBeenCalledWith(`/integrations/${kind}/test`,{});
  expect(reload).toHaveBeenCalledTimes(1);expect(view.state.phase).toBeNull();
  expect(view.state.message).toContain('未通过');rows[0].enabled=false;
  expect(view.state.available).toBe(false);await view.state.start();expect(post).toHaveBeenCalledTimes(1);
 });
 it(`${kind}: unknown receipt survives remount and reconciles without replay`,async()=>{
  view.unmount();rows=[{...rows[0],id:kind,kind}];view=mount(kind);
  vi.mocked(post).mockRejectedValue(Error('lost response'));await view.state.start();
  view.unmount();view=mount(kind);await view.state.start();await view.state.reconcile();
  expect(post).toHaveBeenCalledTimes(1);expect(view.state.phase).toBe('reconciling');
  rows[0].lastCheckedAt='after';await view.state.reconcile();expect(view.state.phase).toBeNull();
 });
}
it('a pending 115 receipt does not lock CD2/qB or share their stored receipts',async()=>{
 vi.mocked(post).mockRejectedValue(Error('lost'));await view.state.start();
 rows.push({id:'cd2',kind:'clouddrive2',enabled:true,revision:1},{id:'qb',kind:'qbittorrent',enabled:true,revision:1});
 const cd=mount('clouddrive2'),qb=mount('qbittorrent');
 try{
  expect(cd.state.phase).toBeNull();expect(qb.state.phase).toBeNull();
  await cd.state.start();await qb.state.start();expect(post).toHaveBeenCalledTimes(3);
  expect(store.size).toBe(3);expect([...store.keys()]).toContain('media-tool:readiness-test');
 }finally{cd.unmount();qb.unmount()}
});
