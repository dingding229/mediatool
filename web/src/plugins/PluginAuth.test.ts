import{expect,it,vi}from'vitest'
import{setupComponent,flush}from'../test/setup-component'
import PluginAuth from'./PluginAuth.vue'
import{authProviders,bindProvider,inspect}from'../api/plugins'
vi.mock('../api/plugins',()=>({authProviders:vi.fn(),bindProvider:vi.fn(),inspect:vi.fn()}));vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn().mockResolvedValue(true)}}))
it('only submits explicit binding, never a login or role assertion, and releases busy',async()=>{vi.mocked(authProviders).mockResolvedValue([{id:'fake',instanceId:'owned',bound:0,state:'unavailable',error:'session_adapter_unsupported'}]);vi.mocked(inspect).mockResolvedValue({fingerprint:'reviewed'}as any);vi.mocked(bindProvider).mockRejectedValueOnce(new Error('denied'));const v=setupComponent(PluginAuth,{instanceId:'owned'});await flush();v.state.subject='external';v.state.account='local-id';await v.state.bind();expect(bindProvider).toHaveBeenCalledWith({instanceId:'owned',providerId:'fake',subject:'external',accountId:'local-id',fingerprint:'reviewed',confirm:true});expect(v.state.busy).toBe(false);expect(v.state.notice).toBe('');v.unmount()})
