import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {request,post} from '../api/client';
import {setupComponent,flush} from '../test/setup-component';
import TaskControl from './TaskControl.vue';
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn(),post:vi.fn()}));
let view:ReturnType<typeof setupComponent>;
beforeEach(()=>{vi.clearAllMocks();vi.mocked(request).mockResolvedValue({id:'job',revision:3,state:'unknown',queued:false,stopPending:false,reason:'待核对'});view=setupComponent(TaskControl,{kind:'jobs',id:'job',state:'unknown'})});
afterEach(()=>view.unmount());
it('confirmation first reads a current revision and never performs a write',async()=>{await view.state.ask('retry');expect(request).toHaveBeenCalledWith('/tasks/jobs/job');expect(post).not.toHaveBeenCalled();expect(view.state.action).toBe('retry')});
it('202 queues without claiming completion and duplicate clicks cannot retry',async()=>{await view.state.ask('retry');vi.mocked(post).mockResolvedValue({id:'job',revision:4,state:'pending',queued:true,reason:'已排队，尚未完成'});await view.state.apply();await view.state.ask('retry');await view.state.apply();expect(post).toHaveBeenCalledOnce();expect(view.state.phase).toBe('queued')});
it('unknown response requires GET reconciliation before another explicit action',async()=>{await view.state.ask('retry');vi.mocked(post).mockRejectedValue(Error('connection lost'));await view.state.apply();await view.state.ask('retry');expect(post).toHaveBeenCalledOnce();expect(view.state.phase).toBe('reconciling');vi.mocked(request).mockResolvedValue({id:'job',revision:4,state:'checking',queued:false,stopPending:false,reason:'核对中'});await view.state.read();expect(view.state.phase).toBe('reconciling')});
it('failed read cannot open the destructive confirmation',async()=>{vi.mocked(request).mockRejectedValue(Error('offline'));await view.state.ask('cancel');expect(view.state.action).toBe('');expect(view.state.error).toBeTruthy()});

it('task queue copy describes reconciliation instead of claiming the queue is read-only',async()=>{const {readFileSync}=await import('node:fs');const text=readFileSync(new URL('../operations/QueuePanel.vue',import.meta.url),'utf8');expect(text).toContain('重试先核对原业务状态');expect(text).not.toContain('这里查看队列状态，不会重放业务')});
