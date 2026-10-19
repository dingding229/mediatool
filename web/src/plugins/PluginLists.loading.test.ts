import {beforeEach,expect,it,vi} from 'vitest'
import {setupComponent,deferred,flush} from '../test/setup-component'
import PluginAutomation from './PluginAutomation.vue'
import PluginSnapshots from './PluginSnapshots.vue'
import PluginAuth from './PluginAuth.vue'
import PluginTools from './PluginTools.vue'
import {workflows,stateSnapshots,authProviders,tools,workflowDefinition,workflowRuns} from '../api/plugins'
vi.mock('../api/plugins',()=>({workflows:vi.fn(),workflowDefinition:vi.fn(),workflowRuns:vi.fn(),workflowOperation:vi.fn(),inspect:vi.fn(),stateSnapshots:vi.fn(),stateOperation:vi.fn(),authProviders:vi.fn(),bindProvider:vi.fn(),tools:vi.fn(),callTool:vi.fn(),dangerousScope:()=>false}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn()}}))
beforeEach(()=>vi.resetAllMocks())
for(const [name,component,api] of [['workflows',PluginAutomation,workflows],['snapshots',PluginSnapshots,stateSnapshots],['auth',PluginAuth,authProviders],['tools',PluginTools,tools]] as const){
 it(`${name}: pending/failure are not empty and explicit retry is a read only`,async()=>{
  const d=deferred<any>();vi.mocked(api).mockReturnValueOnce(d.promise).mockResolvedValueOnce([])
  const v=setupComponent(component,{instanceId:'owned'})
  try{expect(v.state.listRead).toMatchObject({loading:true,loaded:false});d.reject(new Error('private-error'));await flush();expect(v.state.listRead).toMatchObject({loading:false,loaded:false});expect(v.state.listRead.error).not.toBe('');await v.state.load();expect(v.state.listRead).toMatchObject({loading:false,loaded:true,error:''});expect(api).toHaveBeenCalledTimes(2)}finally{v.unmount()}
 })
}
it('workflow details and run history have separate pending/error states; new draft cancels late details',async()=>{
 vi.mocked(workflows).mockResolvedValue([]);vi.mocked(workflowDefinition).mockResolvedValueOnce({dag:{id:'owned',steps:[]},revision:1,fingerprint:'fp',approved:'fp'})
 const runs=deferred<any>();vi.mocked(workflowRuns).mockReturnValueOnce(runs.promise);const v=setupComponent(PluginAutomation)
 try{await flush();const opening=v.state.open('owned');await flush();expect(v.state.detailRead.loaded).toBe(true);expect(v.state.runsRead).toMatchObject({loading:true,loaded:false});runs.reject(new Error());await opening;expect(v.state.runsRead).toMatchObject({loading:false,loaded:false});const stale=deferred<any>();vi.mocked(workflowDefinition).mockReturnValueOnce(stale.promise);const pending=v.state.open('other');v.state.newWorkflow();stale.resolve({dag:{id:'other'}});await pending;expect(v.state.selected).toBeNull();expect(v.state.detailRead.loaded).toBe(false)}finally{v.unmount()}
})
