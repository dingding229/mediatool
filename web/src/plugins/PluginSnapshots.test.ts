import{beforeEach,expect,it,vi}from'vitest'
import{setupComponent,flush}from'../test/setup-component'
import PluginSnapshots from'./PluginSnapshots.vue'
import{inspect,stateOperation,stateSnapshots}from'../api/plugins'
vi.mock('../api/plugins',()=>({inspect:vi.fn(),stateOperation:vi.fn(),stateSnapshots:vi.fn()}))
vi.mock('../utils/request-uuid',()=>({requestUUID:()=> '11111111-1111-4111-8111-111111111111'}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn().mockResolvedValue(true)}}))
beforeEach(()=>{vi.clearAllMocks();vi.mocked(stateSnapshots).mockResolvedValue([]);vi.mocked(inspect).mockResolvedValue({instance:{id:'owned',pluginId:'hello-go',desiredState:'disabled',state:'stopped',revision:7,reason:''},config:{value:{},revision:1},entries:[],next:'',fingerprint:'approved'});vi.mocked(stateOperation).mockResolvedValue({}as any)})
it('uses host CAS and remains disabled after restore',async()=>{const v=setupComponent(PluginSnapshots,{instanceId:'owned'});await flush();await v.state.operate('restore','snapshot');expect(stateOperation).toHaveBeenCalledWith('owned',expect.objectContaining({kind:'restore',snapshotId:'snapshot',expectedRevision:7,fingerprint:'approved',confirm:true}));expect(v.state.notice).toContain('保持停用');expect(v.state.busy).toBe(false);v.unmount()})
it('refuses a live instance and releases busy on every failure',async()=>{vi.mocked(inspect).mockResolvedValueOnce({instance:{desiredState:'enabled',state:'running'}}as any);const v=setupComponent(PluginSnapshots,{instanceId:'owned'});await flush();await v.state.operate('create');expect(stateOperation).not.toHaveBeenCalled();expect(v.state.busy).toBe(false);v.unmount()})
it('ambiguous retry preserves the entire original CAS request, not just its key',async()=>{vi.mocked(stateOperation).mockRejectedValueOnce(new Error('unknown'));const v=setupComponent(PluginSnapshots,{instanceId:'owned'});await flush();await v.state.operate('restore','snapshot');await v.state.operate('restore','snapshot');expect(vi.mocked(stateOperation).mock.calls[0]).toEqual(vi.mocked(stateOperation).mock.calls[1]);expect(inspect).toHaveBeenCalledTimes(1);expect(v.state.busy).toBe(false);v.unmount()})
