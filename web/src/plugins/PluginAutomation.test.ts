import{beforeEach,expect,it,vi}from'vitest'
import{setupComponent,flush}from'../test/setup-component'
import PluginAutomation from'./PluginAutomation.vue'
import{workflows,workflowDefinition,workflowRuns,workflowOperation}from'../api/plugins'
vi.mock('../api/plugins',()=>({workflows:vi.fn(),workflowDefinition:vi.fn(),workflowRuns:vi.fn(),workflowOperation:vi.fn()}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn().mockResolvedValue(true)}}));vi.mock('../utils/request-uuid',()=>({requestUUID:()=> '11111111-1111-4111-8111-111111111111'}))
beforeEach(()=>{vi.clearAllMocks();vi.mocked(workflows).mockResolvedValue([]);vi.mocked(workflowDefinition).mockResolvedValue({dag:{id:'owned',steps:[]},revision:2,fingerprint:'current',approved:'old'});vi.mocked(workflowRuns).mockResolvedValue([]);vi.mocked(workflowOperation).mockResolvedValue({}as any)})
it('approval binds current fingerprint, then requires a separate manual run',async()=>{const v=setupComponent(PluginAutomation);await flush();await v.state.open('owned');await v.state.perform('approve');expect(workflowOperation).toHaveBeenCalledWith(expect.objectContaining({operation:'approve',fingerprint:'current',revision:2,confirm:true}));expect(workflowOperation).toHaveBeenCalledTimes(1);v.unmount()})
it('invalid DAG JSON and HTTP failure cannot leave busy stuck or fake success',async()=>{const v=setupComponent(PluginAutomation);await flush();v.state.dag='{';await v.state.perform('save');expect(workflowOperation).not.toHaveBeenCalled();v.state.dag='{}';vi.mocked(workflowOperation).mockRejectedValueOnce(new Error('fenced'));await v.state.perform('save');expect(v.state.busy).toBe(false);expect(v.state.notice).toBe('');v.unmount()})
