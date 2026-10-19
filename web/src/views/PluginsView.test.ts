import { afterEach,beforeEach,expect,it,vi } from 'vitest'
import {APIError} from '../api/client'
import type { PluginInstance } from '../api/plugins'
import PluginsView from './PluginsView.vue'
import { requestUUID } from '../utils/request-uuid'
import {setupComponent,flush} from '../test/setup-component'
import {catalog,instances,inspect,operate,dangerousScope,reviewPlan,extensions,runHistory,runExtension} from '../api/plugins'
import extensionContract from '../../../internal/platform/httpapi/testdata/contracts/plugin-v2-extensions.json'
import fixture from '../../../internal/platform/httpapi/testdata/contracts/plugin-v2-instances.json'
vi.mock('../api/plugins',()=>({catalog:vi.fn(),instances:vi.fn(),inspect:vi.fn(),operate:vi.fn(),reviewPlan:vi.fn(),extensions:vi.fn(),runHistory:vi.fn(),runExtension:vi.fn(),dangerousScope:(s:string)=>s==='system.exec',pluginPin:'pinned'}))
vi.mock('../utils/request-uuid',()=>({requestUUID:vi.fn()}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn().mockResolvedValue('confirm')}}))
let view:ReturnType<typeof setupComponent>
const d={manifest:{id:'hello-go',name:'Hello Go',version:'1.0.0',description:'fixture',permissions:fixture.request.scopes.map(scope=>({scope}))},fingerprint:fixture.request.fingerprint}
beforeEach(async()=>{try{sessionStorage.removeItem('media-tool:plugin-install-intents:v1')}catch{}vi.clearAllMocks();vi.mocked(operate).mockReset().mockResolvedValue(fixture.response as PluginInstance);vi.mocked(runExtension).mockReset().mockResolvedValue({runId:'fixture-run'});vi.mocked(requestUUID).mockReset().mockReturnValue(fixture.request.operationKey);vi.mocked(catalog).mockResolvedValue({items:[d],total:1});vi.mocked(instances).mockResolvedValue([]);view=setupComponent(PluginsView);await flush()})
afterEach(()=>{view.unmount();vi.unstubAllGlobals()})
it('requires consent and matches the exact shared operation contract',async()=>{view.state.install(d);await view.state.accept();expect(operate).not.toHaveBeenCalled();view.state.confirmed=true;await view.state.accept();expect(operate).toHaveBeenCalledWith({...fixture.request,instanceId:undefined})})
it('retains operationKey for an ambiguous retry, exposes errors, does not claim success',async()=>{vi.mocked(operate).mockRejectedValue(new Error('unavailable'));view.state.install(d);view.state.confirmed=true;await view.state.accept();expect(view.state.error).toContain('结果未确认');await view.state.accept();expect(vi.mocked(operate).mock.calls[0]![0].operationKey).toBe(vi.mocked(operate).mock.calls[1]![0].operationKey)})
it('sends encrypted config through CAS and refuses malformed JSON',async()=>{const i=fixture.response as PluginInstance;vi.mocked(inspect).mockResolvedValue({instance:i,config:{value:{a:1},revision:1},entries:[],next:'',fingerprint:d.fingerprint});await view.state.open(i,'config');view.state.configText='{';await view.state.save();expect(operate).not.toHaveBeenCalled();view.state.configText='{"a":2}';await view.state.save();expect(operate).toHaveBeenCalledWith(expect.objectContaining({kind:'configure',config:{a:2},expectedRevision:i.revision,fingerprint:d.fingerprint}))})
it('missing plugin remains inspectable and page navigation is bounded',async()=>{vi.mocked(instances).mockResolvedValue([fixture.response as PluginInstance]);await view.state.changePage(-2);expect(instances).toHaveBeenLastCalledWith(1);expect(dangerousScope('system.exec')).toBe(true)})

it('shows queued status honestly and never auto-retries unknown results',async()=>{vi.mocked(extensions).mockResolvedValue(extensionContract.snapshot);vi.mocked(runHistory).mockResolvedValue(extensionContract.history);await view.state.openExtensions(fixture.response);expect(view.state.history[0].state).toBe('unknown');expect(runExtension).not.toHaveBeenCalled();await view.state.triggerExtension(view.state.runnableExtensions[0]);expect(runExtension).toHaveBeenCalledWith(fixture.response.id,extensionContract.request);expect(view.state.queued).toContain('不是执行成功')})

it('releases install busy after UUID failure and allows retry without a false success',async()=>{
 vi.mocked(requestUUID).mockImplementationOnce(()=>{throw new Error('entropy unavailable')})
 view.state.install(d);view.state.confirmed=true;await view.state.accept()
 expect(view.state.busy).toBe(false);expect(view.state.error).toContain('操作未完成');expect(view.state.error).not.toContain('entropy unavailable');expect(operate).not.toHaveBeenCalled();expect(view.state.consent).not.toBeNull()
 await view.state.accept();expect(operate).toHaveBeenCalledTimes(1);expect(view.state.busy).toBe(false);expect(view.state.consent).toBeNull();expect(view.state.error).toBe('')
})
it('releases busy even when serializing input fails',async()=>{
 const input:any={kind:'configure'};input.config=input
 await view.state.apply(input);expect(view.state.busy).toBe(false);expect(view.state.error).not.toBe('');expect(operate).not.toHaveBeenCalled()
 await view.state.apply({kind:'disable'});expect(operate).toHaveBeenCalledTimes(1)
})
it('releases busy after HTTP failure and retries the same operation key',async()=>{
 vi.mocked(operate).mockRejectedValueOnce(new Error('timeout'));view.state.install(d);view.state.confirmed=true
 await view.state.accept();expect(view.state.busy).toBe(false)
 await view.state.accept();expect(view.state.busy).toBe(false);expect(requestUUID).toHaveBeenCalledTimes(1)
})
it('releases extension busy after UUID and HTTP failures, preserves retry identity',async()=>{
 vi.mocked(extensions).mockResolvedValue(extensionContract.snapshot);vi.mocked(runHistory).mockResolvedValue([])
 await view.state.openExtensions(fixture.response);const command=view.state.runnableExtensions[0]
 vi.mocked(requestUUID).mockImplementationOnce(()=>{throw new Error('entropy unavailable')})
 await view.state.triggerExtension(command);expect(view.state.extensionBusy).toBe(false);expect(runExtension).not.toHaveBeenCalled();expect(view.state.queued).toBe('')
 vi.mocked(runExtension).mockRejectedValueOnce(new Error('timeout'))
 await view.state.triggerExtension(command);expect(view.state.extensionBusy).toBe(false);expect(view.state.error).toBe('timeout');expect(view.state.queued).toBe('')
 await view.state.triggerExtension(command);expect(view.state.extensionBusy).toBe(false);expect(runExtension).toHaveBeenCalledTimes(2)
 expect(vi.mocked(runExtension).mock.calls[0]![1]).toEqual(vi.mocked(runExtension).mock.calls[1]![1]);expect(view.state.queued).toContain('不是执行成功')
})

it('loads the host review snapshot, displays scope diff and still requires explicit enable',async()=>{
 const plan={instance:{...fixture.response,revision:7} as PluginInstance,current:d.manifest,availability:'available',fingerprint:d.fingerprint,approvedVersion:'0.9.0',baselineKnown:true,approvedScopes:[],added:['system.exec'],removed:[],changed:[],needsReview:true}
 vi.mocked(reviewPlan).mockResolvedValue(plan)
 await view.state.review(fixture.response);expect(view.state.reviewDetails).toEqual(plan);expect(view.state.selected.revision).toBe(7);expect(view.state.confirmed).toBe(false)
 view.state.confirmed=true;await view.state.accept();expect(operate).toHaveBeenCalledWith(expect.objectContaining({kind:'review',expectedRevision:7}));expect(view.state.notice).toContain('仍为停用');expect(vi.mocked(operate).mock.calls).toHaveLength(1)
})
it('missing definitions cannot be confirmed and failed preview always releases busy',async()=>{
 vi.mocked(reviewPlan).mockRejectedValueOnce(new Error('stale'));await view.state.review(fixture.response);expect(view.state.busy).toBe(false);expect(view.state.error).toBe('stale')
 vi.mocked(reviewPlan).mockResolvedValue({instance:fixture.response as PluginInstance,current:d.manifest,availability:'missing',fingerprint:d.fingerprint,approvedVersion:'',baselineKnown:false,approvedScopes:[],added:[],removed:[],changed:[],needsReview:true})
 await view.state.review(fixture.response);view.state.confirmed=true;await view.state.accept();expect(operate).not.toHaveBeenCalled();expect(view.state.available({...fixture.response,availability:'missing'})).toBe(false)
})
it('reloading a review resets consent and uses the latest host revision',async()=>{
 vi.mocked(reviewPlan).mockResolvedValue({instance:fixture.response as PluginInstance,current:d.manifest,availability:'available',fingerprint:d.fingerprint,approvedVersion:'',baselineKnown:false,approvedScopes:[],added:[],removed:[],changed:[],needsReview:true})
 await view.state.review(fixture.response);view.state.confirmed=true;await view.state.review(fixture.response);expect(view.state.confirmed).toBe(false)
})

it('R regression: reports installed receipt even when list synchronization fails',async()=>{
 view.state.install(d);view.state.confirmed=true
 vi.mocked(catalog).mockRejectedValueOnce(new Error('list unavailable'))
 await view.state.accept()
 expect(view.state.notice).toContain('已安装')
 expect(view.state.notice).toContain(fixture.response.id)
 expect(view.state.operationPhase).toBe('sync_failed')
 expect(view.state.consent).toBeNull()
 expect(view.state.error).toContain('列表同步失败')
 expect(view.state.busy).toBe(false)
})
it('R regression: labels ambiguous results without leaking raw exception contents',async()=>{
 view.state.install(d);view.state.confirmed=true
 vi.mocked(operate).mockRejectedValueOnce(new Error('secret-input-sentinel network disconnected'))
 await view.state.accept()
 expect(view.state.operationPhase).toBe('uncertain')
 expect(view.state.error).toContain('结果未确认')
 expect(view.state.error).not.toContain('secret-input-sentinel')
 await view.state.accept()
 expect(vi.mocked(operate).mock.calls[0]![0].operationKey).toBe(vi.mocked(operate).mock.calls[1]![0].operationKey)
})

it('keeps a committed receipt and retries only list synchronization, not installation',async()=>{
 view.state.install(d);view.state.confirmed=true;vi.mocked(catalog).mockRejectedValueOnce(new Error('private list error'))
 await view.state.accept();expect(view.state.operationPhase).toBe('sync_failed');expect(view.state.installed[0].id).toBe(fixture.response.id)
 expect(view.state.installLabel({...d,installedCount:1})).toBe('新增实例')
 await view.state.load();expect(operate).toHaveBeenCalledTimes(1);expect(view.state.operationPhase).toBe('committed');expect(view.state.notice).toContain('已安装')
})
it('shows a rejected safe result code and requestId, never exception contents',async()=>{
 view.state.install(d);view.state.confirmed=true
 vi.mocked(operate).mockRejectedValueOnce(new APIError(409,'plugin_conflict','config-secret-sentinel','abcdef0123456789abcdef01'))
 await view.state.accept();expect(view.state.operationPhase).toBe('rejected');expect(view.state.error).toContain('plugin_conflict');expect(view.state.error).toContain('abcdef0123456789abcdef01');expect(view.state.error).not.toContain('config-secret-sentinel');expect(view.state.notice).toBe('')
})
it('blocks a duplicate submit while the first operation is in flight',async()=>{
 let finish!:(i:PluginInstance)=>void;vi.mocked(operate).mockImplementationOnce(()=>new Promise(r=>{finish=r}))
 view.state.install(d);view.state.confirmed=true;const first=view.state.accept();await flush();expect(view.state.operationPhase).toBe('submitting');await view.state.accept();expect(operate).toHaveBeenCalledTimes(1)
 finish(fixture.response as PluginInstance);await first;expect(view.state.busy).toBe(false)
})
it('does not let an older list response replace a newer authoritative snapshot',async()=>{
 let release!:(v:any)=>void
 vi.mocked(catalog).mockImplementationOnce(()=>new Promise(r=>{release=r}))
 const old=view.state.load();await flush()
 vi.mocked(catalog).mockResolvedValueOnce({items:[{...d,installedCount:1}],total:1});vi.mocked(instances).mockResolvedValueOnce([fixture.response as PluginInstance])
 await view.state.load();release({items:[],total:0});await old
 expect(view.state.definitions).toHaveLength(1);expect(view.state.installed[0].id).toBe(fixture.response.id)
})
