import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import ManualDownloads from './ManualDownloads.vue'
import {post,request} from '../api/client'
import {ElMessageBox} from 'element-plus'
import {flush,setupComponent} from '../test/setup-component'
vi.mock('../api/client',()=>({request:vi.fn(),post:vi.fn()}))
vi.mock('element-plus',()=>({ElMessage:{success:vi.fn(),warning:vi.fn()},ElMessageBox:{confirm:vi.fn()}}))
let view:ReturnType<typeof setupComponent>
beforeEach(async()=>{vi.useFakeTimers();vi.mocked(post).mockReset();vi.mocked(request).mockResolvedValue([]);vi.mocked(ElMessageBox.confirm).mockResolvedValue(undefined as never);view=setupComponent(ManualDownloads);await flush()})
afterEach(()=>{view.unmount();vi.useRealTimers()})
it('never directly retries an uncertain task',async()=>{await view.state.retry({id:'id-1',state:'uncertain'});expect(post).not.toHaveBeenCalled()})
it('posts explicit confirmation and duplicate acknowledgement after an absent observation',async()=>{
 await view.state.retry({id:'id-1',state:'absent_confirmed'})
 expect(post).toHaveBeenCalledWith('/downloads/manual/id-1/retry',{confirm:true,acceptDuplicateRisk:true})
})
it('posts the read-only reconcile endpoint without silently requeueing',async()=>{
 vi.mocked(post).mockResolvedValue({state:'absent_confirmed'})
 await view.state.reconcile({id:'id-1',state:'uncertain'})
 expect(post).toHaveBeenCalledExactlyOnceWith('/downloads/manual/id-1/inspect',{})
})
it('retries a blocked task without falsely accepting unknown-outcome risk',async()=>{
 await view.state.retry({id:'id-1',state:'blocked'})
 expect(post).toHaveBeenCalledWith('/downloads/manual/id-1/retry',{confirm:true,acceptDuplicateRisk:false})
})
