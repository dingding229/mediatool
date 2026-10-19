import {beforeEach,expect,it,vi} from 'vitest'
import {reactive} from 'vue'
import {setupComponent,deferred,flush} from '../test/setup-component'
import LogsView from './LogsView.vue'
import {request} from '../api/client'
const route=reactive({query:{module:'plugins',instance:'first'}})
vi.mock('vue-router',()=>({useRoute:()=>route}))
vi.mock('../api/client',()=>({request:vi.fn(),post:vi.fn()}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn()},ElMessage:{error:vi.fn(),success:vi.fn()},ElDatePicker:{}}))
const result={items:[],total:0,modules:['plugins'],before:'',truncated:false}
beforeEach(()=>{vi.resetAllMocks();route.query={module:'plugins',instance:'first'}})
it('log count remains unknown until a successful read; failure and retry preserve the instance filter',async()=>{
 const d=deferred<any>();let reads=0
 vi.mocked(request).mockImplementation(async(path)=>{if(path==='/logs/summary')return {} as any;if(++reads===1)return d.promise;return result})
 const v=setupComponent(LogsView)
 try{expect(v.state.busy).toBe(true);expect(v.state.loaded).toBe(false);d.reject(new Error('failed'));await flush();expect(v.state.loaded).toBe(false);expect(v.state.error).toBe('failed');v.state.refresh();await flush();expect(v.state.loaded).toBe(true);expect(v.state.error).toBe('');expect(vi.mocked(request).mock.calls.filter(([p])=>p.startsWith('/logs/query')).every(([p])=>p.includes('module=plugins')&&p.includes('instance=first'))).toBe(true)}finally{v.unmount()}
})
it('changing route filters starts a new query and fences the old instance response',async()=>{
 const d=deferred<any>();let reads=0
 vi.mocked(request).mockImplementation(async(path)=>path==='/logs/summary'?{} as any:++reads===1?d.promise:result)
 const v=setupComponent(LogsView)
 try{route.query={module:'plugins',instance:'second'};await flush();expect(v.state.instance).toBe('second');expect(vi.mocked(request).mock.calls.some(([p])=>p.includes('instance=second'))).toBe(true);d.resolve({...result,items:[{msg:'old-instance'}],total:1});await flush();expect(v.state.rows).toEqual([]);expect(v.state.total).toBe(0)}finally{v.unmount()}
})
