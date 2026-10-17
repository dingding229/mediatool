import { afterEach,beforeEach,expect,it,vi } from 'vitest'
import type { PluginInstance } from '../api/plugins'
import PluginsView from './PluginsView.vue'
import {setupComponent,flush} from '../test/setup-component'
import {catalog,instances,inspect,operate,dangerousScope,extensions,runHistory,runExtension} from '../api/plugins'
import extensionContract from '../../../internal/platform/httpapi/testdata/contracts/plugin-v2-extensions.json'
import fixture from '../../../internal/platform/httpapi/testdata/contracts/plugin-v2-instances.json'
vi.mock('../api/plugins',()=>({catalog:vi.fn(),instances:vi.fn(),inspect:vi.fn(),operate:vi.fn(),extensions:vi.fn(),runHistory:vi.fn(),runExtension:vi.fn(),dangerousScope:(s:string)=>s==='system.exec',pluginPin:'pinned'}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn().mockResolvedValue('confirm')}}))
let view:ReturnType<typeof setupComponent>
const d={manifest:{id:'hello-go',name:'Hello Go',version:'1.0.0',description:'fixture',permissions:fixture.request.scopes.map(scope=>({scope}))},fingerprint:fixture.request.fingerprint}
beforeEach(async()=>{vi.clearAllMocks();vi.stubGlobal('crypto',{randomUUID:()=>fixture.request.operationKey});vi.mocked(catalog).mockResolvedValue({items:[d],total:1});vi.mocked(instances).mockResolvedValue([]);view=setupComponent(PluginsView);await flush()})
afterEach(()=>{view.unmount();vi.unstubAllGlobals()})
it('requires consent and matches the exact shared operation contract',async()=>{view.state.install(d);await view.state.accept();expect(operate).not.toHaveBeenCalled();view.state.confirmed=true;await view.state.accept();expect(operate).toHaveBeenCalledWith({...fixture.request,instanceId:undefined})})
it('retains operationKey for an ambiguous retry, exposes errors, does not claim success',async()=>{vi.mocked(operate).mockRejectedValue(new Error('unavailable'));view.state.install(d);view.state.confirmed=true;await view.state.accept();expect(view.state.error).toBe('unavailable');await view.state.accept();expect(vi.mocked(operate).mock.calls[0]![0].operationKey).toBe(vi.mocked(operate).mock.calls[1]![0].operationKey)})
it('sends encrypted config through CAS and refuses malformed JSON',async()=>{const i=fixture.response as PluginInstance;vi.mocked(inspect).mockResolvedValue({instance:i,config:{value:{a:1},revision:1},entries:[],next:'',fingerprint:d.fingerprint});await view.state.open(i,'config');view.state.configText='{';await view.state.save();expect(operate).not.toHaveBeenCalled();view.state.configText='{"a":2}';await view.state.save();expect(operate).toHaveBeenCalledWith(expect.objectContaining({kind:'configure',config:{a:2},expectedRevision:i.revision,fingerprint:d.fingerprint}))})
it('missing plugin remains inspectable and page navigation is bounded',async()=>{vi.mocked(instances).mockResolvedValue([fixture.response as PluginInstance]);await view.state.changePage(-2);expect(instances).toHaveBeenLastCalledWith(1);expect(dangerousScope('system.exec')).toBe(true)})

it('shows queued status honestly and never auto-retries unknown results',async()=>{vi.mocked(extensions).mockResolvedValue(extensionContract.snapshot);vi.mocked(runHistory).mockResolvedValue(extensionContract.history);await view.state.openExtensions(fixture.response);expect(view.state.history[0].state).toBe('unknown');expect(runExtension).not.toHaveBeenCalled();await view.state.triggerExtension(view.state.runnableExtensions[0]);expect(runExtension).toHaveBeenCalledWith(fixture.response.id,extensionContract.request);expect(view.state.queued).toContain('不是执行成功')})
