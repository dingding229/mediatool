import {it,expect,beforeEach,afterEach,vi} from 'vitest';
import {reactive,nextTick} from 'vue';
import {setupComponent,flush} from '../test/setup-component';
import About from './AboutPanel.vue';
import Online from './OnlineUpdatePanel.vue';
import {setUpdateChecks,getOnlineUpdate,executeOnlineUpdate,confirmOnlineUpdateRecovery} from '../api/updates';
let store:any,views:ReturnType<typeof setupComponent>[]=[];
vi.mock('../stores/updates',()=>({useUpdateStore:()=>store}));
vi.mock('../api/updates',()=>({setUpdateChecks:vi.fn(),getOnlineUpdate:vi.fn(),executeOnlineUpdate:vi.fn(),confirmOnlineUpdateRecovery:vi.fn(),checkUpdates:vi.fn()}));
const state=()=>({enabled:true,intervalHours:12,hasUpdate:true,result:{status:'verified',checkedAt:'2026-10-26T00:00:00Z',latest:{version:'0.40.0',tag:'v0.40.0',digest:'sha256:fixture'}}});
const available=()=>({available:true,reason:'',socketMounted:true,policyVerified:true,recoveryRequired:[],job:null});
const mount=(c:any)=>{const v=setupComponent(c);views.push(v);return v.state};
beforeEach(()=>{vi.clearAllMocks();store=reactive({state:state(),build:{version:'0.39.4'},error:'',notice:null,dismiss:vi.fn(),refresh:vi.fn(),accept:vi.fn(v=>store.state=v)});vi.mocked(getOnlineUpdate).mockResolvedValue(available())});
afterEach(()=>{views.forEach(v=>v.unmount());views=[]});
it('checks interval bounds and prevents invalid settings submissions',async()=>{const s=mount(About);for(const value of ['0','169','1.5','abc','']){s.hours=value;expect(s.intervalValid).toBe(false);await s.saveSettings()}expect(setUpdateChecks).not.toHaveBeenCalled();for(const value of ['1','12','168']){s.hours=value;s.enabled=false;vi.mocked(setUpdateChecks).mockResolvedValue({...state(),intervalHours:Number(value),enabled:false} as any);await s.saveSettings();expect(setUpdateChecks).toHaveBeenLastCalledWith(false,Number(value))}});
it('does not overwrite an edited interval during a background refresh',async()=>{const s=mount(About);s.hours='168';s.dirty=true;store.state={...state(),intervalHours:1};await nextTick();expect(s.hours).toBe('168')});
it('requires both password and explicit risk confirmation, then clears the password',async()=>{const s=mount(Online);await flush();expect(s.ready).toBeTruthy();s.prepare();s.password='synthetic-secret';await s.execute();expect(executeOnlineUpdate).not.toHaveBeenCalled();s.confirmed=true;s.password='';await s.execute();expect(executeOnlineUpdate).not.toHaveBeenCalled();s.password='synthetic-secret';vi.mocked(executeOnlineUpdate).mockRejectedValue(Error('当前密码不正确'));await s.execute();expect(executeOnlineUpdate).toHaveBeenCalledWith('0.40.0','sha256:fixture','synthetic-secret',true);expect(s.password).toBe('');expect(s.error).toContain('当前密码不正确')});
it('is always on with valid prerequisites and keeps manual recovery locked',async()=>{vi.mocked(getOnlineUpdate).mockResolvedValue({...available(),executionEnabled:false} as any);const s=mount(Online);await flush();expect(s.ready).toBeTruthy();vi.mocked(getOnlineUpdate).mockResolvedValue({...available(),available:false,recoveryRequired:['pending']} as any);await s.load();expect(s.ready).toBeFalsy();s.prepare('pending');s.password='synthetic-password';s.confirmed=true;await s.execute();expect(confirmOnlineUpdateRecovery).toHaveBeenCalledWith('pending','synthetic-password',true);expect(executeOnlineUpdate).not.toHaveBeenCalled()});
it('shows an explicit environment reason and never enables the update button',async()=>{vi.mocked(getOnlineUpdate).mockResolvedValue({...available(),available:false,reason:'docker.sock 权限不足，请核对 GID'} as any);const s=mount(Online);await flush();expect(s.ready).toBeFalsy();expect(s.result).toContain('GID')});
it('never presents a persisted older release or its notes after a manual release',()=>{
 store.build.version='0.51.0';store.state.result.latest={version:'0.49.0',tag:'v0.49.0',notes:'old release'};store.state.hasUpdate=false;
 const s=mount(About);expect(s.currentAtLeastLatest).toBe(true);expect(s.latest).toBeNull();expect(s.status).toBe('已是最新版本');
 store.state.result.latest={version:'0.51.0',tag:'v0.51.0'};expect(s.currentAtLeastLatest).toBe(true);
 store.state.result.latest={version:'0.52.0',tag:'v0.52.0'};store.state.hasUpdate=true;expect(s.currentAtLeastLatest).toBe(false);expect(s.latest.version).toBe('0.52.0');expect(s.status).toBe('发现新稳定版本');
});
it('uses fresh state provenance while build metadata is loading, never flashes an older cache',()=>{
 store.state.current={version:'0.51.0'};store.build=undefined;store.state.result.latest={version:'0.49.0',tag:'v0.49.0'};
 const s=mount(About);expect(s.version).toBe('v0.51.0');expect(s.latest).toBeNull();expect(s.currentAtLeastLatest).toBe(true);
 store.state.current=undefined;expect(s.latest).toBeNull();expect(s.version).toBe('读取中');
});

it('uses the server ignored-cache flag when no older release is returned, without showing update execution',async()=>{store.state={...state(),hasUpdate:false,currentAtLeastLatest:true,result:{status:'verified',checkedAt:'2026-11-02T00:00:00Z',latest:null}};store.build={version:'0.51.0'};const s=mount(About);await flush();expect(s.currentAtLeastLatest).toBe(true);expect(s.latest).toBeNull();expect(s.status).toBe('已是最新版本');const online=mount(Online);await flush();expect(online.target).toBeFalsy();expect(executeOnlineUpdate).not.toHaveBeenCalled()});
