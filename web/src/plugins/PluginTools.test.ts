import{beforeEach,expect,it,vi}from'vitest'
import{setupComponent,flush}from'../test/setup-component'
import PluginTools from'./PluginTools.vue'
import{tools,callTool}from'../api/plugins'
import{ElMessageBox}from'element-plus'
vi.mock('../api/plugins',()=>({tools:vi.fn(),callTool:vi.fn(),dangerousScope:()=>false}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn().mockResolvedValue(true)}}))
vi.mock('../utils/request-uuid',()=>({requestUUID:()=> '11111111-1111-4111-8111-111111111111'}))
beforeEach(()=>{vi.clearAllMocks();vi.mocked(tools).mockResolvedValue([{kind:'tool',id:'echo',name:'echo',description:'fixture',inputSchema:{type:'object'},effectKind:'read',requiredScopes:['agent.tools']}]);vi.mocked(callTool).mockResolvedValue({ok:true})})
it('read tool is manual and errors release busy without claiming success',async()=>{const v=setupComponent(PluginTools,{instanceId:'owned'});await flush();vi.mocked(callTool).mockRejectedValueOnce(new Error('unknown'));await v.state.run();expect(v.state.busy).toBe(false);expect(v.state.output).toBe('');await v.state.run();expect(vi.mocked(callTool).mock.calls[0]).toEqual(vi.mocked(callTool).mock.calls[1]);expect(ElMessageBox.confirm).not.toHaveBeenCalled();v.unmount()})
it('mutating tools require a second confirmation; invalid JSON never invokes',async()=>{vi.mocked(tools).mockResolvedValue([{kind:'tool',id:'write',name:'write',description:'fixture',inputSchema:{type:'object'},effectKind:'dangerous',requiredScopes:['agent.tools']}]);const v=setupComponent(PluginTools,{instanceId:'owned'});await flush();v.state.input='{';await v.state.run();expect(callTool).not.toHaveBeenCalled();v.state.input='{}';await v.state.run();expect(ElMessageBox.confirm).toHaveBeenCalled();expect(callTool).toHaveBeenCalledWith('owned',expect.objectContaining({confirm:true}));v.unmount()})
